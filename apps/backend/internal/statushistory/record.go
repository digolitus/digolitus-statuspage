// Package statushistory records the history of each component's effective
// status and derives daily uptime from it (docs/adr/0014-status-history.md).
//
// The effective status changes only through explicit writes (component
// status, incidents, maintenance transitions), so every service that can
// change it calls Record at the end of its transaction. Record recomputes
// the effective status of every component with statuspolicy and appends a
// row for each component whose status differs from its last recorded one.
// The history is therefore committed if and only if the change is.
package statushistory

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/statuspolicy"
)

// Cause types: what triggered a recalculation.
const (
	CauseBaseline    = "baseline"
	CauseComponent   = "component"
	CauseIncident    = "incident"
	CauseMaintenance = "maintenance"
)

// CauseTypes lists every cause type, for filter validation.
var CauseTypes = []string{CauseBaseline, CauseComponent, CauseIncident, CauseMaintenance}

// Cause identifies the change being recorded.
type Cause struct {
	Type string
	ID   string
}

// lockKey serializes recalculations across transactions and processes. A
// transaction that waits for it sees, once it holds it, everything committed
// by the previous holder (READ COMMITTED takes a new snapshot per statement),
// so the last recorded status is always the one actually in force.
const lockKey int64 = 0x5354_4154_5553_4831 // "STATUSH1"

type last struct {
	status    *string
	changedAt time.Time
	exists    bool
}

// Record recalculates effective statuses using q, which must be the
// transaction of the change, and appends the differences. at is the time of
// the change; it is never earlier than a component's previous record, so the
// history stays ordered even when transactions wait for the lock.
func Record(ctx context.Context, q database.Querier, cause Cause, at time.Time) error {
	if _, err := q.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockKey); err != nil {
		return fmt.Errorf("lock status history: %w", err)
	}
	type comp struct {
		id, status string
		active     bool
		prev       last
	}
	rows, err := q.Query(ctx, `SELECT c.id, c.status, c.is_active, h.status, h.changed_at, h.id IS NOT NULL
		FROM components c
		LEFT JOIN LATERAL (SELECT id, status, changed_at FROM component_status_changes s
			WHERE s.component_id = c.id ORDER BY changed_at DESC, id DESC LIMIT 1) h ON true`)
	if err != nil {
		return fmt.Errorf("load components for status history: %w", err)
	}
	var comps []comp
	for rows.Next() {
		var c comp
		var changedAt *time.Time
		if err := rows.Scan(&c.id, &c.status, &c.active, &c.prev.status, &changedAt, &c.prev.exists); err != nil {
			rows.Close()
			return err
		}
		if changedAt != nil {
			c.prev.changedAt = *changedAt
		}
		comps = append(comps, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	incidents, err := activeIncidents(ctx, q)
	if err != nil {
		return err
	}
	maintenances, err := runningMaintenances(ctx, q)
	if err != nil {
		return err
	}
	states := make([]statuspolicy.ComponentState, 0, len(comps))
	for _, c := range comps {
		if c.active {
			states = append(states, statuspolicy.ComponentState{ID: c.id, DeclaredStatus: c.status})
		}
	}
	effective, _ := statuspolicy.Evaluate(states, incidents, maintenances)
	var actorID, actorName *string
	if a, ok := httpx.ActorFrom(ctx); ok && cause.Type != CauseBaseline {
		actorID, actorName = &a.ID, &a.Name
	}

	for _, c := range comps {
		var next *string
		if c.active {
			st := effective[c.id]
			next = &st
		}
		if c.prev.exists && equal(c.prev.status, next) {
			continue
		}
		if !c.prev.exists && next == nil {
			continue // never tracked and inactive: nothing to say
		}
		causeType, causeID := cause.Type, nullable(cause.ID)
		if !c.prev.exists && !(cause.Type == CauseComponent && cause.ID == c.id) {
			// First record of a component that existed before tracking began.
			causeType, causeID = CauseBaseline, nil
		}
		when := at
		if c.prev.exists && when.Before(c.prev.changedAt) {
			when = c.prev.changedAt
		}
		if _, err := q.Exec(ctx, `INSERT INTO component_status_changes
			(component_id, status, previous_status, cause_type, cause_id, actor_id, actor_name, changed_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, c.id, next, c.prev.status, causeType, causeID, actorID, actorName, when); err != nil {
			return fmt.Errorf("insert status change: %w", err)
		}
	}
	return nil
}

// Baseline records the current status of components never recorded (for
// instance right after the history was introduced). It is idempotent.
func Baseline(ctx context.Context, pool *pgxpool.Pool, at time.Time) error {
	return database.InTx(ctx, pool, func(tx pgx.Tx) error {
		return Record(ctx, tx, Cause{Type: CauseBaseline}, at)
	})
}

func activeIncidents(ctx context.Context, q database.Querier) ([]statuspolicy.ActiveIncident, error) {
	rows, err := q.Query(ctx, `SELECT i.impact, array_agg(ic.component_id::text)
		FROM incidents i JOIN incident_components ic ON ic.incident_id = i.id
		WHERE i.status <> 'resolved' GROUP BY i.id, i.impact`)
	if err != nil {
		return nil, fmt.Errorf("load active incidents for status history: %w", err)
	}
	defer rows.Close()
	var out []statuspolicy.ActiveIncident
	for rows.Next() {
		var a statuspolicy.ActiveIncident
		if err := rows.Scan(&a.Impact, &a.ComponentIDs); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func runningMaintenances(ctx context.Context, q database.Querier) ([]statuspolicy.ActiveMaintenance, error) {
	var ids []string
	err := q.QueryRow(ctx, `SELECT coalesce(array_agg(DISTINCT mc.component_id::text), '{}')
		FROM maintenances m JOIN maintenance_components mc ON mc.maintenance_id = m.id
		WHERE m.status = 'in_progress'`).Scan(&ids)
	if err != nil {
		return nil, fmt.Errorf("load running maintenances for status history: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return []statuspolicy.ActiveMaintenance{{ComponentIDs: ids}}, nil
}

func equal(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
