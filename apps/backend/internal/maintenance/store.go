package maintenance

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/component"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
)

const columns = `id, title, description, expected_impact, status, timezone, scheduled_start, scheduled_end,
	started_at, completed_at, cancelled_at, internal_notes, template_id, created_at, updated_at`

const updateColumns = "id, maintenance_id, kind, status, message, created_at"

// MaxTimelineInDetail bounds the updates embedded in a detail response.
const MaxTimelineInDetail = 100

var sortColumns = map[string]string{
	"scheduled_start": "scheduled_start",
	"created_at":      "created_at",
	"updated_at":      "updated_at",
}

// States accepted by the "state" filter.
var States = []string{"upcoming", "in_progress", "completed", "cancelled", "open", "past"}

// Filter narrows a maintenance listing.
type Filter struct {
	State       string
	ComponentID string
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func scan(row pgx.Row) (Maintenance, error) {
	var m Maintenance
	err := row.Scan(&m.ID, &m.Title, &m.Description, &m.ExpectedImpact, &m.Status, &m.Timezone, &m.ScheduledStart, &m.ScheduledEnd,
		&m.StartedAt, &m.CompletedAt, &m.CancelledAt, &m.InternalNotes, &m.TemplateID, &m.CreatedAt, &m.UpdatedAt)
	m.ScheduledStart, m.ScheduledEnd = m.ScheduledStart.UTC(), m.ScheduledEnd.UTC()
	m.StartedAt, m.CompletedAt, m.CancelledAt = utcPtr(m.StartedAt), utcPtr(m.CompletedAt), utcPtr(m.CancelledAt)
	m.CreatedAt, m.UpdatedAt = m.CreatedAt.UTC(), m.UpdatedAt.UTC()
	return m, err
}

func scanAll(rows pgx.Rows) ([]Maintenance, error) {
	defer rows.Close()
	var out []Maintenance
	for rows.Next() {
		m, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func get(ctx context.Context, q database.Querier, id string, lock bool) (Maintenance, error) {
	sql := "SELECT " + columns + " FROM maintenances WHERE id = $1"
	if lock {
		sql += " FOR UPDATE"
	}
	m, err := scan(q.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Maintenance{}, apperror.NotFound("maintenance")
	}
	if err != nil {
		return Maintenance{}, fmt.Errorf("get maintenance: %w", err)
	}
	return m, nil
}

func insert(ctx context.Context, q database.Querier, m Maintenance) (Maintenance, error) {
	created, err := scan(q.QueryRow(ctx, `INSERT INTO maintenances (title, description, expected_impact, status, timezone,
		scheduled_start, scheduled_end, internal_notes, template_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10) RETURNING `+columns,
		m.Title, m.Description, m.ExpectedImpact, m.Status, m.Timezone, m.ScheduledStart, m.ScheduledEnd, m.InternalNotes, m.TemplateID, m.CreatedAt))
	if err != nil {
		if name, ok := database.ConstraintError(err, database.CodeForeignKeyViolation); ok && name == "maintenances_template_id_fkey" {
			return Maintenance{}, &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
				Fields: []apperror.FieldError{{Field: "template_id", Message: "Template não encontrado."}}}
		}
		return Maintenance{}, fmt.Errorf("insert maintenance: %w", err)
	}
	return created, nil
}

func save(ctx context.Context, q database.Querier, m Maintenance) (Maintenance, error) {
	saved, err := scan(q.QueryRow(ctx, `UPDATE maintenances SET title = $2, description = $3, expected_impact = $4, status = $5,
		timezone = $6, scheduled_start = $7, scheduled_end = $8, started_at = $9, completed_at = $10, cancelled_at = $11,
		internal_notes = $12, updated_at = $13 WHERE id = $1 RETURNING `+columns,
		m.ID, m.Title, m.Description, m.ExpectedImpact, m.Status, m.Timezone, m.ScheduledStart, m.ScheduledEnd,
		m.StartedAt, m.CompletedAt, m.CancelledAt, m.InternalNotes, m.UpdatedAt))
	if err != nil {
		return Maintenance{}, fmt.Errorf("update maintenance: %w", err)
	}
	return saved, nil
}

func replaceComponents(ctx context.Context, q database.Querier, id string, ids []string) error {
	if _, err := q.Exec(ctx, "DELETE FROM maintenance_components WHERE maintenance_id = $1", id); err != nil {
		return fmt.Errorf("clear maintenance components: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}
	if _, err := q.Exec(ctx, "INSERT INTO maintenance_components (maintenance_id, component_id) SELECT $1, unnest($2::uuid[])", id, ids); err != nil {
		return fmt.Errorf("set maintenance components: %w", err)
	}
	return nil
}

func insertUpdate(ctx context.Context, q database.Querier, id, kind, status string, message *string, at time.Time) error {
	_, err := q.Exec(ctx, "INSERT INTO maintenance_updates (maintenance_id, kind, status, message, created_at) VALUES ($1, $2, $3, $4, $5)",
		id, kind, status, message, at)
	if err != nil {
		return fmt.Errorf("insert maintenance update: %w", err)
	}
	return nil
}

func scanUpdates(rows pgx.Rows) ([]Update, []string, error) {
	defer rows.Close()
	var out []Update
	var owners []string
	for rows.Next() {
		var u Update
		var owner string
		if err := rows.Scan(&u.ID, &owner, &u.Kind, &u.Status, &u.Message, &u.CreatedAt); err != nil {
			return nil, nil, err
		}
		u.CreatedAt = u.CreatedAt.UTC()
		out = append(out, u)
		owners = append(owners, owner)
	}
	return out, owners, rows.Err()
}

func listUpdates(ctx context.Context, q database.Querier, id string, page httpx.Page) ([]Update, int, error) {
	var total int
	if err := q.QueryRow(ctx, "SELECT count(*) FROM maintenance_updates WHERE maintenance_id = $1", id).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count maintenance updates: %w", err)
	}
	rows, err := q.Query(ctx, "SELECT "+updateColumns+` FROM maintenance_updates WHERE maintenance_id = $1
		ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3`, id, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list maintenance updates: %w", err)
	}
	updates, _, err := scanUpdates(rows)
	return updates, total, err
}

func hydrate(ctx context.Context, q database.Querier, list []Maintenance, withTimeline bool, now time.Time) error {
	for k := range list {
		list[k].flag(now)
	}
	if len(list) == 0 {
		return nil
	}
	ids := make([]string, len(list))
	for k, m := range list {
		ids[k] = m.ID
	}
	refs, err := component.RefsByOwner(ctx, q, component.OwnerMaintenance, ids)
	if err != nil {
		return err
	}
	var rows pgx.Rows
	if withTimeline {
		rows, err = q.Query(ctx, `SELECT `+updateColumns+` FROM (
			SELECT *, row_number() OVER (PARTITION BY maintenance_id ORDER BY created_at DESC, id DESC) AS rn
			FROM maintenance_updates WHERE maintenance_id = ANY($1::uuid[])) t
			WHERE rn <= $2 ORDER BY maintenance_id, created_at DESC, id DESC`, ids, MaxTimelineInDetail)
	} else {
		rows, err = q.Query(ctx, `SELECT DISTINCT ON (maintenance_id) `+updateColumns+`
			FROM maintenance_updates WHERE maintenance_id = ANY($1::uuid[])
			ORDER BY maintenance_id, created_at DESC, id DESC`, ids)
	}
	if err != nil {
		return fmt.Errorf("load maintenance updates: %w", err)
	}
	updates, owners, err := scanUpdates(rows)
	if err != nil {
		return err
	}
	byOwner := map[string][]Update{}
	for k, u := range updates {
		byOwner[owners[k]] = append(byOwner[owners[k]], u)
	}
	for k := range list {
		m := &list[k]
		m.Components = refs[m.ID]
		if m.Components == nil {
			m.Components = []component.Ref{}
		}
		ups := byOwner[m.ID]
		if withTimeline {
			if ups == nil {
				ups = []Update{}
			}
			m.Updates = ups
		}
		if len(ups) > 0 {
			latest := ups[0]
			m.LatestUpdate = &latest
		}
	}
	return nil
}

// Load returns one maintenance with components and timeline.
func Load(ctx context.Context, q database.Querier, id string, now time.Time) (Maintenance, error) {
	m, err := get(ctx, q, id, false)
	if err != nil {
		return Maintenance{}, err
	}
	list := []Maintenance{m}
	if err := hydrate(ctx, q, list, true, now); err != nil {
		return Maintenance{}, err
	}
	return list[0], nil
}

func stateClause(state string) string {
	switch state {
	case "upcoming":
		return "status = 'scheduled'"
	case "in_progress":
		return "status = 'in_progress'"
	case "completed":
		return "status = 'completed'"
	case "cancelled":
		return "status = 'cancelled'"
	case "open":
		return "status IN ('scheduled', 'in_progress')"
	case "past":
		return "status IN ('completed', 'cancelled')"
	default:
		return "true"
	}
}

// DefaultSort orders future work ascending and history descending.
func DefaultSort(state string) httpx.Sort {
	switch state {
	case "upcoming", "in_progress", "open":
		return httpx.Sort{Field: "scheduled_start"}
	default:
		return httpx.Sort{Field: "scheduled_start", Desc: true}
	}
}

// List returns a page of maintenances with components and latest update.
func List(ctx context.Context, q database.Querier, f Filter, sort httpx.Sort, page httpx.Page, now time.Time) ([]Maintenance, int, error) {
	where := "WHERE " + stateClause(f.State) + ` AND ($1 = '' OR EXISTS (SELECT 1 FROM maintenance_components mc
		WHERE mc.maintenance_id = maintenances.id AND mc.component_id = NULLIF($1, '')::uuid))`
	var total int
	// #nosec G202 -- the state clause comes from a whitelist (stateClause).
	if err := q.QueryRow(ctx, "SELECT count(*) FROM maintenances "+where, f.ComponentID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count maintenances: %w", err)
	}
	dir := "ASC"
	if sort.Desc {
		dir = "DESC"
	}
	// #nosec G202 -- the state clause and ORDER BY come from whitelists; filters are bound parameters.
	rows, err := q.Query(ctx, "SELECT "+columns+" FROM maintenances "+where+" ORDER BY "+sortColumns[sort.Field]+" "+dir+", id "+dir+" LIMIT $2 OFFSET $3",
		f.ComponentID, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list maintenances: %w", err)
	}
	list, err := scanAll(rows)
	if err != nil {
		return nil, 0, err
	}
	return list, total, hydrate(ctx, q, list, false, now)
}

// Open returns scheduled and in-progress maintenances (bounded), ordered by
// planned start, with their timelines.
func Open(ctx context.Context, q database.Querier, now time.Time) ([]Maintenance, error) {
	rows, err := q.Query(ctx, "SELECT "+columns+" FROM maintenances WHERE status IN ('scheduled', 'in_progress') ORDER BY scheduled_start, id LIMIT 100")
	if err != nil {
		return nil, fmt.Errorf("list open maintenances: %w", err)
	}
	list, err := scanAll(rows)
	if err != nil {
		return nil, err
	}
	return list, hydrate(ctx, q, list, true, now)
}
