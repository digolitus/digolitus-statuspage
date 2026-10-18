package maintenance

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/audit"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/component"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/clock"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
)

// Service implements maintenance use cases.
type Service struct {
	pool *pgxpool.Pool
	now  clock.Clock
}

// NewService creates a Service.
func NewService(pool *pgxpool.Pool, now clock.Clock) *Service {
	return &Service{pool: pool, now: now}
}

type auditView struct {
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	ExpectedImpact string   `json:"expected_impact"`
	Status         string   `json:"status"`
	Timezone       string   `json:"timezone"`
	ScheduledStart string   `json:"scheduled_start"`
	ScheduledEnd   string   `json:"scheduled_end"`
	InternalNotes  string   `json:"internal_notes"`
	ComponentIDs   []string `json:"component_ids"`
}

const rfc3339Micro = "2006-01-02T15:04:05.999999Z07:00"

func viewOf(m Maintenance, componentIDs []string) auditView {
	if componentIDs == nil {
		componentIDs = []string{}
	}
	return auditView{Title: m.Title, Description: m.Description, ExpectedImpact: m.ExpectedImpact, Status: m.Status,
		Timezone: m.Timezone, ScheduledStart: m.ScheduledStart.Format(rfc3339Micro), ScheduledEnd: m.ScheduledEnd.Format(rfc3339Micro),
		InternalNotes: m.InternalNotes, ComponentIDs: componentIDs}
}

// Create schedules a maintenance and publishes its first timeline entry.
func (s *Service) Create(ctx context.Context, in CreateInput) (Maintenance, error) {
	now := s.now()
	plan, err := in.normalize(now)
	if err != nil {
		return Maintenance{}, err
	}
	var id string
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		refs, err := component.ResolveRefs(ctx, tx, plan.componentIDs, nil, "component_ids")
		if err != nil {
			return err
		}
		m := plan.m
		m.CreatedAt = now
		created, err := insert(ctx, tx, m)
		if err != nil {
			return err
		}
		id = created.ID
		ids := component.RefIDs(refs)
		if err := replaceComponents(ctx, tx, id, ids); err != nil {
			return err
		}
		if err := insertUpdate(ctx, tx, id, "created", StatusScheduled, plan.message, now); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.ResourceMaintenance, id, "created", audit.Changes{After: viewOf(created, ids)}, now)
	})
	if err != nil {
		return Maintenance{}, err
	}
	return Load(ctx, s.pool, id, s.now())
}

// Patch edits an open maintenance. Rescheduling is recorded in the audit
// log; publishing a public explanation is a separate, explicit message.
func (s *Service) Patch(ctx context.Context, id string, in PatchInput) (Maintenance, error) {
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := get(ctx, tx, id, true)
		if err != nil {
			return err
		}
		now := s.now()
		next, rescheduled, err := in.apply(current, now)
		if err != nil {
			return err
		}
		existing, err := component.RefsByOwner(ctx, tx, component.OwnerMaintenance, []string{id})
		if err != nil {
			return err
		}
		beforeIDs := component.RefIDs(existing[id])
		afterIDs := beforeIDs
		if in.ComponentIDs != nil {
			keep := map[string]bool{}
			for _, cid := range beforeIDs {
				keep[cid] = true
			}
			refs, err := component.ResolveRefs(ctx, tx, *in.ComponentIDs, keep, "component_ids")
			if err != nil {
				return err
			}
			afterIDs = component.RefIDs(refs)
			if err := replaceComponents(ctx, tx, id, afterIDs); err != nil {
				return err
			}
		}
		next.UpdatedAt = now
		saved, err := save(ctx, tx, next)
		if err != nil {
			return err
		}
		action := "updated"
		if rescheduled {
			action = "rescheduled"
		}
		return audit.Record(ctx, tx, audit.ResourceMaintenance, id, action, audit.Diff(viewOf(current, beforeIDs), viewOf(saved, afterIDs)), now)
	})
	if err != nil {
		return Maintenance{}, err
	}
	return Load(ctx, s.pool, id, s.now())
}

// PostMessage publishes a message without changing the status.
func (s *Service) PostMessage(ctx context.Context, id string, in MessageInput) (Maintenance, error) {
	var v apperror.Validator
	msg := optionalMessage(&v, in.Message)
	if msg == nil && !v.Has("message") {
		v.Add("message", "Informe uma mensagem com 1 a 5000 caracteres.")
	}
	if err := v.Err(); err != nil {
		return Maintenance{}, err
	}
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := get(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if !IsOpen(current.Status) {
			return apperror.Conflict("maintenance_closed", "Manutenções concluídas ou canceladas não aceitam novas mensagens.")
		}
		now := s.now()
		if err := insertUpdate(ctx, tx, id, "message", current.Status, msg, now); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE maintenances SET updated_at = $2 WHERE id = $1", id, now); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.ResourceMaintenance, id, "update_published", audit.Changes{After: map[string]any{"message": *msg}}, now)
	})
	if err != nil {
		return Maintenance{}, err
	}
	return Load(ctx, s.pool, id, s.now())
}

var transitionKinds = map[Transition]string{Start: "started", Complete: "completed", Cancel: "cancelled"}

// Transition applies a lifecycle action with an optional message. The actual
// instants (started_at, completed_at, cancelled_at) are the server time of
// the action, independent of the planned window.
func (s *Service) Transition(ctx context.Context, id string, t Transition, in MessageInput) (Maintenance, error) {
	var v apperror.Validator
	msg := optionalMessage(&v, in.Message)
	if err := v.Err(); err != nil {
		return Maintenance{}, err
	}
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := get(ctx, tx, id, true)
		if err != nil {
			return err
		}
		to, ok := Target(t, current.Status)
		if !ok {
			return apperror.Conflict("maintenance_invalid_transition",
				"A ação '"+string(t)+"' não é permitida para uma manutenção com status '"+current.Status+"'.")
		}
		now := s.now()
		next := current
		next.Status = to
		next.UpdatedAt = now
		switch t {
		case Start:
			next.StartedAt = &now
		case Complete:
			next.CompletedAt = &now
		case Cancel:
			next.CancelledAt = &now
		}
		if _, err := save(ctx, tx, next); err != nil {
			return err
		}
		if err := insertUpdate(ctx, tx, id, transitionKinds[t], to, msg, now); err != nil {
			return err
		}
		changes := audit.Changes{Before: map[string]any{"status": current.Status}, After: map[string]any{"status": to}}
		if msg != nil {
			changes.After.(map[string]any)["message"] = *msg
		}
		return audit.Record(ctx, tx, audit.ResourceMaintenance, id, transitionKinds[t], changes, now)
	})
	if err != nil {
		return Maintenance{}, err
	}
	return Load(ctx, s.pool, id, s.now())
}
