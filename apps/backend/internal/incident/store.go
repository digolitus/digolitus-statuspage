package incident

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

const columns = "id, title, impact, status, internal_notes, started_at, resolved_at, template_id, created_at, updated_at"

// MaxTimelineInDetail bounds the updates embedded in a detail response; the
// full timeline is available through the paginated updates endpoint.
const MaxTimelineInDetail = 100

var sortColumns = map[string]string{
	"started_at": "started_at",
	"created_at": "created_at",
	"updated_at": "updated_at",
}

// Filter narrows an incident listing.
type Filter struct {
	State       string // "", "active", "resolved"
	Status      string
	Impact      string
	ComponentID string
}

func scanIncident(row pgx.Row) (Incident, error) {
	var i Incident
	err := row.Scan(&i.ID, &i.Title, &i.Impact, &i.Status, &i.InternalNotes, &i.StartedAt, &i.ResolvedAt, &i.TemplateID, &i.CreatedAt, &i.UpdatedAt)
	i.StartedAt, i.CreatedAt, i.UpdatedAt = i.StartedAt.UTC(), i.CreatedAt.UTC(), i.UpdatedAt.UTC()
	if i.ResolvedAt != nil {
		t := i.ResolvedAt.UTC()
		i.ResolvedAt = &t
	}
	return i, err
}

func get(ctx context.Context, q database.Querier, id string, forUpdate bool) (Incident, error) {
	sql := "SELECT " + columns + " FROM incidents WHERE id = $1"
	if forUpdate {
		sql += " FOR UPDATE"
	}
	i, err := scanIncident(q.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Incident{}, apperror.NotFound("incident")
	}
	if err != nil {
		return Incident{}, fmt.Errorf("get incident: %w", err)
	}
	return i, nil
}

func insert(ctx context.Context, q database.Querier, i Incident) (Incident, error) {
	created, err := scanIncident(q.QueryRow(ctx, `INSERT INTO incidents (title, impact, status, internal_notes, started_at, template_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7) RETURNING `+columns,
		i.Title, i.Impact, i.Status, i.InternalNotes, i.StartedAt, i.TemplateID, i.CreatedAt))
	if err != nil {
		if name, ok := database.ConstraintError(err, database.CodeForeignKeyViolation); ok && name == "incidents_template_id_fkey" {
			return Incident{}, &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
				Fields: []apperror.FieldError{{Field: "template_id", Message: "Template não encontrado."}}}
		}
		return Incident{}, fmt.Errorf("insert incident: %w", err)
	}
	return created, nil
}

func save(ctx context.Context, q database.Querier, i Incident) (Incident, error) {
	saved, err := scanIncident(q.QueryRow(ctx, `UPDATE incidents SET title = $2, impact = $3, status = $4, internal_notes = $5,
		started_at = $6, resolved_at = $7, updated_at = $8 WHERE id = $1 RETURNING `+columns,
		i.ID, i.Title, i.Impact, i.Status, i.InternalNotes, i.StartedAt, i.ResolvedAt, i.UpdatedAt))
	if err != nil {
		return Incident{}, fmt.Errorf("update incident: %w", err)
	}
	return saved, nil
}

func replaceComponents(ctx context.Context, q database.Querier, incidentID string, ids []string) error {
	if _, err := q.Exec(ctx, "DELETE FROM incident_components WHERE incident_id = $1", incidentID); err != nil {
		return fmt.Errorf("clear incident components: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}
	_, err := q.Exec(ctx, `INSERT INTO incident_components (incident_id, component_id)
		SELECT $1, unnest($2::uuid[])`, incidentID, ids)
	if err != nil {
		return fmt.Errorf("set incident components: %w", err)
	}
	return nil
}

func insertUpdate(ctx context.Context, q database.Querier, incidentID, status, message string, componentIDs []string, at time.Time) (Update, error) {
	u := Update{Status: status, Message: message, CreatedAt: at, componentIDs: componentIDs}
	err := q.QueryRow(ctx, `INSERT INTO incident_updates (incident_id, status, message, component_ids, created_at)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`, incidentID, status, message, componentIDs, at).Scan(&u.ID)
	if err != nil {
		return Update{}, fmt.Errorf("insert incident update: %w", err)
	}
	return u, nil
}

const updateColumns = "id, incident_id, status, message, component_ids, created_at"

func scanUpdates(rows pgx.Rows) ([]Update, []string, error) {
	defer rows.Close()
	var out []Update
	var owners []string
	for rows.Next() {
		var u Update
		var owner string
		if err := rows.Scan(&u.ID, &owner, &u.Status, &u.Message, &u.componentIDs, &u.CreatedAt); err != nil {
			return nil, nil, err
		}
		u.CreatedAt = u.CreatedAt.UTC()
		out = append(out, u)
		owners = append(owners, owner)
	}
	return out, owners, rows.Err()
}

// resolveSnapshots fills Update.Components from the stored ID snapshots.
func resolveSnapshots(ctx context.Context, q database.Querier, updates []Update) error {
	var ids []string
	for _, u := range updates {
		ids = append(ids, u.componentIDs...)
	}
	refs, err := component.RefsByID(ctx, q, ids)
	if err != nil {
		return err
	}
	for k := range updates {
		if updates[k].componentIDs == nil {
			continue
		}
		list := make([]component.Ref, 0, len(updates[k].componentIDs))
		for _, id := range updates[k].componentIDs {
			if r, ok := refs[id]; ok {
				list = append(list, r)
			}
		}
		updates[k].Components = &list
	}
	return nil
}

// listUpdates returns a page of an incident's timeline, newest first.
func listUpdates(ctx context.Context, q database.Querier, incidentID string, page httpx.Page) ([]Update, int, error) {
	var total int
	if err := q.QueryRow(ctx, "SELECT count(*) FROM incident_updates WHERE incident_id = $1", incidentID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count incident updates: %w", err)
	}
	rows, err := q.Query(ctx, "SELECT "+updateColumns+` FROM incident_updates WHERE incident_id = $1
		ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3`, incidentID, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list incident updates: %w", err)
	}
	updates, _, err := scanUpdates(rows)
	if err != nil {
		return nil, 0, err
	}
	return updates, total, resolveSnapshots(ctx, q, updates)
}

// hydrate attaches components and either the latest update or the timeline.
func hydrate(ctx context.Context, q database.Querier, incidents []Incident, withTimeline bool) error {
	if len(incidents) == 0 {
		return nil
	}
	ids := make([]string, len(incidents))
	for k, i := range incidents {
		ids[k] = i.ID
	}
	refs, err := component.RefsByOwner(ctx, q, component.OwnerIncident, ids)
	if err != nil {
		return err
	}
	var rows pgx.Rows
	if withTimeline {
		rows, err = q.Query(ctx, `SELECT `+updateColumns+` FROM (
			SELECT *, row_number() OVER (PARTITION BY incident_id ORDER BY created_at DESC, id DESC) AS rn
			FROM incident_updates WHERE incident_id = ANY($1::uuid[])) t
			WHERE rn <= $2 ORDER BY incident_id, created_at DESC, id DESC`, ids, MaxTimelineInDetail)
	} else {
		rows, err = q.Query(ctx, `SELECT DISTINCT ON (incident_id) `+updateColumns+`
			FROM incident_updates WHERE incident_id = ANY($1::uuid[])
			ORDER BY incident_id, created_at DESC, id DESC`, ids)
	}
	if err != nil {
		return fmt.Errorf("load incident updates: %w", err)
	}
	updates, owners, err := scanUpdates(rows)
	if err != nil {
		return err
	}
	if err := resolveSnapshots(ctx, q, updates); err != nil {
		return err
	}
	byOwner := map[string][]Update{}
	for k, u := range updates {
		byOwner[owners[k]] = append(byOwner[owners[k]], u)
	}
	for k := range incidents {
		inc := &incidents[k]
		inc.Components = refs[inc.ID]
		if inc.Components == nil {
			inc.Components = []component.Ref{}
		}
		list := byOwner[inc.ID]
		if withTimeline {
			if list == nil {
				list = []Update{}
			}
			inc.Updates = list
		}
		if len(list) > 0 {
			latest := list[0]
			inc.LatestUpdate = &latest
		}
	}
	return nil
}

// Load returns one incident with components and timeline.
func Load(ctx context.Context, q database.Querier, id string) (Incident, error) {
	inc, err := get(ctx, q, id, false)
	if err != nil {
		return Incident{}, err
	}
	list := []Incident{inc}
	if err := hydrate(ctx, q, list, true); err != nil {
		return Incident{}, err
	}
	return list[0], nil
}

// List returns a page of incidents with components and latest update.
func List(ctx context.Context, q database.Querier, f Filter, sort httpx.Sort, page httpx.Page) ([]Incident, int, error) {
	where := `WHERE ($1 = '' OR ($1 = 'active' AND status <> 'resolved') OR ($1 = 'resolved' AND status = 'resolved'))
		AND ($2 = '' OR status = $2) AND ($3 = '' OR impact = $3)
		AND ($4 = '' OR EXISTS (SELECT 1 FROM incident_components ic WHERE ic.incident_id = incidents.id AND ic.component_id = NULLIF($4, '')::uuid))`
	args := []any{f.State, f.Status, f.Impact, f.ComponentID}
	var total int
	if err := q.QueryRow(ctx, "SELECT count(*) FROM incidents "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count incidents: %w", err)
	}
	dir := "ASC"
	if sort.Desc {
		dir = "DESC"
	}
	order := sortColumns[sort.Field] + " " + dir + ", id " + dir
	// #nosec G202 -- ORDER BY comes from the sortColumns whitelist; filters are bound parameters.
	rows, err := q.Query(ctx, "SELECT "+columns+" FROM incidents "+where+" ORDER BY "+order+" LIMIT $5 OFFSET $6",
		append(args, page.Limit, page.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list incidents: %w", err)
	}
	defer rows.Close()
	var out []Incident
	for rows.Next() {
		i, err := scanIncident(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, i)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	rows.Close()
	return out, total, hydrate(ctx, q, out, false)
}

// Active returns every open incident with its full timeline, most recent
// first. The number of simultaneously open incidents is small by nature.
func Active(ctx context.Context, q database.Querier) ([]Incident, error) {
	rows, err := q.Query(ctx, "SELECT "+columns+" FROM incidents WHERE status <> 'resolved' ORDER BY started_at DESC, id DESC LIMIT 100")
	if err != nil {
		return nil, fmt.Errorf("list active incidents: %w", err)
	}
	var out []Incident
	for rows.Next() {
		i, err := scanIncident(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, i)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, hydrate(ctx, q, out, true)
}
