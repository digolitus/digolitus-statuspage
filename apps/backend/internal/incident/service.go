package incident

import (
	"time"

	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/audit"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/auth"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/component"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/notify"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/clock"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
)

// Service implements incident use cases. Each mutation runs in a single
// transaction together with its timeline entry and audit record.
type Service struct {
	pool *pgxpool.Pool
	now  clock.Clock
}

// NewService creates a Service.
func NewService(pool *pgxpool.Pool, now clock.Clock) *Service {
	return &Service{pool: pool, now: now}
}

type auditView struct {
	Title         string   `json:"title"`
	Impact        string   `json:"impact"`
	Status        string   `json:"status"`
	InternalNotes string   `json:"internal_notes"`
	StartedAt     string   `json:"started_at"`
	ResolvedAt    *string  `json:"resolved_at,omitempty"`
	TemplateID    *string  `json:"template_id,omitempty"`
	ComponentIDs  []string `json:"component_ids,omitempty"`
}

func viewOf(i Incident) auditView {
	v := auditView{Title: i.Title, Impact: i.Impact, Status: i.Status, InternalNotes: i.InternalNotes,
		StartedAt: i.StartedAt.Format("2006-01-02T15:04:05.999999Z07:00"), TemplateID: i.TemplateID}
	if i.ResolvedAt != nil {
		s := i.ResolvedAt.Format("2006-01-02T15:04:05.999999Z07:00")
		v.ResolvedAt = &s
	}
	if i.Components != nil {
		v.ComponentIDs = component.RefIDs(i.Components)
	}
	return v
}

// Create opens an incident and publishes its first update atomically.
func (s *Service) Create(ctx context.Context, in CreateInput) (Incident, error) {
	now := s.now()
	plan, err := in.normalize(now)
	if err != nil {
		return Incident{}, err
	}
	var id string
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		refs, err := component.ResolveRefs(ctx, tx, plan.componentIDs, nil, "component_ids")
		if err != nil {
			return err
		}
		if err := auth.RequireComponents(ctx, tx, auth.PermIncidentsManage, component.RefIDs(refs)); err != nil {
			return err
		}
		inc := plan.incident
		inc.CreatedAt = now
		created, err := insert(ctx, tx, inc)
		if err != nil {
			return err
		}
		id = created.ID
		ids := component.RefIDs(refs)
		if err := replaceComponents(ctx, tx, id, ids); err != nil {
			return err
		}
		if _, err := insertUpdate(ctx, tx, id, created.Status, plan.message, ids, now); err != nil {
			return err
		}
		if err := notify.Record(ctx, tx, event(notify.IncidentCreated, created, plan.message, refs, now)); err != nil {
			return err
		}
		created.Components = refs
		return audit.Record(ctx, tx, audit.ResourceIncident, id, "created", audit.Changes{After: map[string]any{
			"incident": viewOf(created), "message": plan.message}}, now)
	})
	if err != nil {
		return Incident{}, err
	}
	return Load(ctx, s.pool, id)
}

// Patch edits metadata without publishing a timeline entry.
func (s *Service) Patch(ctx context.Context, id string, in PatchInput) (Incident, error) {
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := get(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if err := requireScope(ctx, tx, id); err != nil {
			return err
		}
		now := s.now()
		next, err := in.apply(current, now)
		if err != nil {
			return err
		}
		next.UpdatedAt = now
		saved, err := save(ctx, tx, next)
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.ResourceIncident, id, "updated", audit.Diff(viewOf(current), viewOf(saved)), now)
	})
	if err != nil {
		return Incident{}, err
	}
	return Load(ctx, s.pool, id)
}

// PostUpdate publishes a timeline entry and applies its status and
// component changes. Moving to resolved also sets resolved_at.
func (s *Service) PostUpdate(ctx context.Context, id string, in UpdateInput) (Incident, error) {
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := get(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if err := requireScope(ctx, tx, id); err != nil {
			return err
		}
		msg, err := in.validate(current.Status)
		if err != nil {
			return err
		}
		now := s.now()
		existing, err := component.RefsByOwner(ctx, tx, component.OwnerIncident, []string{id})
		if err != nil {
			return err
		}
		current.Components = existing[id]

		var snapshot []string
		affected := current.Components
		if in.ComponentIDs != nil {
			keep := map[string]bool{}
			for _, r := range current.Components {
				keep[r.ID] = true
			}
			refs, err := component.ResolveRefs(ctx, tx, *in.ComponentIDs, keep, "component_ids")
			if err != nil {
				return err
			}
			if err := auth.RequireComponents(ctx, tx, auth.PermIncidentsManage, component.RefIDs(refs)); err != nil {
				return err
			}
			snapshot = component.RefIDs(refs)
			affected = refs
			if err := replaceComponents(ctx, tx, id, snapshot); err != nil {
				return err
			}
		}

		next := current
		next.Status = in.Status
		next.UpdatedAt = now
		if in.Status == StatusResolved {
			if now.Before(next.StartedAt) {
				return apperror.Conflict("incident_not_started", "O incidente não pode ser resolvido antes do horário de início informado.")
			}
			next.ResolvedAt = &now
		}
		saved, err := save(ctx, tx, next)
		if err != nil {
			return err
		}
		if _, err := insertUpdate(ctx, tx, id, in.Status, msg, snapshot, now); err != nil {
			return err
		}
		kind := notify.IncidentUpdated
		if in.Status == StatusResolved {
			kind = notify.IncidentResolved
		}
		if err := notify.Record(ctx, tx, event(kind, saved, msg, affected, now)); err != nil {
			return err
		}
		action := "update_published"
		if in.Status == StatusResolved {
			action = "resolved"
		} else if in.Status != current.Status {
			action = "status_changed"
		}
		before := viewOf(current)
		after := viewOf(saved)
		if snapshot != nil {
			after.ComponentIDs = snapshot
		} else {
			after.ComponentIDs = before.ComponentIDs
		}
		changes := audit.Diff(before, after)
		if m, ok := changes.After.(map[string]any); ok {
			m["message"] = msg
		}
		return audit.Record(ctx, tx, audit.ResourceIncident, id, action, changes, now)
	})
	if err != nil {
		return Incident{}, err
	}
	return Load(ctx, s.pool, id)
}

// Resolve closes an incident with a final message. Resolving an incident
// that is already resolved is a conflict rather than a silent no-op, so a
// client never believes it published a message that was discarded.
func (s *Service) Resolve(ctx context.Context, id string, in ResolveInput) (Incident, error) {
	inc, err := s.PostUpdate(ctx, id, UpdateInput{Status: StatusResolved, Message: in.Message})
	if appErr, ok := apperror.As(err); ok && appErr.Code == "incident_resolved" {
		return Incident{}, apperror.Conflict("incident_already_resolved", "O incidente já está resolvido.")
	}
	return inc, err
}

// requireScope applies team scopes to an existing incident: a team-scoped role
// must own every affected component.
func requireScope(ctx context.Context, q database.Querier, id string) error {
	refs, err := component.RefsByOwner(ctx, q, component.OwnerIncident, []string{id})
	if err != nil {
		return err
	}
	return auth.RequireComponents(ctx, q, auth.PermIncidentsManage, component.RefIDs(refs[id]))
}

// event builds the public notification of an incident change.
func event(kind string, i Incident, message string, refs []component.Ref, at time.Time) notify.Event {
	comps := []notify.Component{}
	for _, r := range component.PublicRefs(refs) {
		comps = append(comps, notify.Component{ID: r.ID, Name: r.Name})
	}
	return notify.Event{Kind: kind, ResourceType: "incident", ResourceID: i.ID, Title: i.Title, Status: i.Status,
		Impact: i.Impact, Message: message, Components: comps, Path: "/incidents/" + i.ID, OccurredAt: at}
}
