package component

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
)

// columns includes the effective owner: the component's team or, when
// unset, its group's team.
const columns = `id, name, slug, description, show_description, status, is_public, is_active, display_order, group_id, url, team_id,
	COALESCE(team_id, (SELECT g.team_id FROM component_groups g WHERE g.id = components.group_id)), created_at, updated_at`

// SortFields maps API sort keys to columns.
var SortFields = map[string]string{
	"display_order": "display_order",
	"name":          "name",
	"created_at":    "created_at",
	"updated_at":    "updated_at",
}

// Filter narrows a component listing.
type Filter struct {
	Active  *bool
	Public  *bool
	GroupID string
	// TeamID filters by effective owner team.
	TeamID string
}

func scan(row pgx.Row) (Component, error) {
	var c Component
	err := row.Scan(&c.ID, &c.Name, &c.Slug, &c.Description, &c.ShowDescription, &c.Status, &c.IsPublic, &c.IsActive, &c.DisplayOrder, &c.GroupID, &c.URL, &c.TeamID, &c.OwnerTeamID, &c.CreatedAt, &c.UpdatedAt)
	c.CreatedAt, c.UpdatedAt = c.CreatedAt.UTC(), c.UpdatedAt.UTC()
	return c, err
}

// List returns a page of components.
func List(ctx context.Context, q database.Querier, f Filter, sort httpx.Sort, page httpx.Page) ([]Component, int, error) {
	where := `WHERE ($1::boolean IS NULL OR is_active = $1) AND ($2::boolean IS NULL OR is_public = $2) AND ($3 = '' OR group_id = NULLIF($3, '')::uuid)
		AND ($4 = '' OR COALESCE(team_id, (SELECT g.team_id FROM component_groups g WHERE g.id = components.group_id)) = NULLIF($4, '')::uuid)`
	var total int
	if err := q.QueryRow(ctx, "SELECT count(*) FROM components "+where, f.Active, f.Public, f.GroupID, f.TeamID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count components: %w", err)
	}
	dir := "ASC"
	if sort.Desc {
		dir = "DESC"
	}
	order := SortFields[sort.Field] + " " + dir + ", name ASC, id ASC"
	// #nosec G202 -- ORDER BY comes from the SortFields whitelist; filters are bound parameters.
	rows, err := q.Query(ctx, "SELECT "+columns+" FROM components "+where+" ORDER BY "+order+" LIMIT $5 OFFSET $6",
		f.Active, f.Public, f.GroupID, f.TeamID, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list components: %w", err)
	}
	defer rows.Close()
	var out []Component
	for rows.Next() {
		c, err := scan(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

// AllActive returns every active component in display order.
func AllActive(ctx context.Context, q database.Querier) ([]Component, error) {
	rows, err := q.Query(ctx, "SELECT "+columns+" FROM components WHERE is_active ORDER BY display_order, name, id")
	if err != nil {
		return nil, fmt.Errorf("list active components: %w", err)
	}
	defer rows.Close()
	var out []Component
	for rows.Next() {
		c, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Get loads a component, optionally locking it for update.
func Get(ctx context.Context, q database.Querier, id string, forUpdate bool) (Component, error) {
	sql := "SELECT " + columns + " FROM components WHERE id = $1"
	if forUpdate {
		sql += " FOR UPDATE"
	}
	c, err := scan(q.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Component{}, apperror.NotFound("component")
	}
	if err != nil {
		return Component{}, fmt.Errorf("get component: %w", err)
	}
	return c, nil
}

func insert(ctx context.Context, q database.Querier, c Component) (Component, error) {
	created, err := scan(q.QueryRow(ctx, `INSERT INTO components (name, slug, description, status, is_public, is_active, display_order, group_id, url, team_id, created_at, updated_at, show_description)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11, $12) RETURNING `+columns,
		c.Name, c.Slug, c.Description, c.Status, c.IsPublic, c.IsActive, c.DisplayOrder, c.GroupID, c.URL, c.TeamID, c.CreatedAt, c.ShowDescription))
	return created, mapWriteError(err)
}

func update(ctx context.Context, q database.Querier, c Component) (Component, error) {
	updated, err := scan(q.QueryRow(ctx, `UPDATE components SET name = $2, slug = $3, description = $4, status = $5,
		is_public = $6, is_active = $7, display_order = $8, group_id = $9, url = $10, team_id = $11, updated_at = $12, show_description = $13 WHERE id = $1 RETURNING `+columns,
		c.ID, c.Name, c.Slug, c.Description, c.Status, c.IsPublic, c.IsActive, c.DisplayOrder, c.GroupID, c.URL, c.TeamID, c.UpdatedAt, c.ShowDescription))
	return updated, mapWriteError(err)
}

func mapWriteError(err error) error {
	if err == nil {
		return nil
	}
	if name, ok := database.ConstraintError(err, database.CodeUniqueViolation); ok && name == "components_slug_key" {
		return &apperror.Error{Kind: apperror.KindConflict, Code: "component_slug_conflict", Message: "Já existe um componente com este slug.",
			Fields: []apperror.FieldError{{Field: "slug", Message: "Slug já utilizado."}}}
	}
	if name, ok := database.ConstraintError(err, database.CodeForeignKeyViolation); ok && name == "components_group_id_fkey" {
		return &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
			Fields: []apperror.FieldError{{Field: "group_id", Message: "Grupo não encontrado."}}}
	}
	if name, ok := database.ConstraintError(err, database.CodeForeignKeyViolation); ok && name == "components_team_id_fkey" {
		return &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
			Fields: []apperror.FieldError{{Field: "team_id", Message: "Time não encontrado."}}}
	}
	return fmt.Errorf("write component: %w", err)
}

// ResolveRefs validates that ids reference existing components and returns
// their references in display order. Inactive components are rejected unless
// listed in keep (associations that already exist may be preserved). field
// names the input field for error details.
func ResolveRefs(ctx context.Context, q database.Querier, ids []string, keep map[string]bool, field string) ([]Ref, error) {
	if len(ids) == 0 {
		return []Ref{}, nil
	}
	if len(ids) > 50 {
		return nil, &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
			Fields: []apperror.FieldError{{Field: field, Message: "Informe no máximo 50 componentes."}}}
	}
	seen := map[string]bool{}
	clean := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.ToLower(id)
		if !httpx.IsUUID(id) {
			return nil, &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
				Fields: []apperror.FieldError{{Field: field, Message: "Contém um identificador inválido."}}}
		}
		if !seen[id] {
			seen[id] = true
			clean = append(clean, id)
		}
	}
	rows, err := q.Query(ctx, "SELECT id, name, slug, is_public, is_active FROM components WHERE id = ANY($1::uuid[]) ORDER BY display_order, name, id", clean)
	if err != nil {
		return nil, fmt.Errorf("resolve components: %w", err)
	}
	defer rows.Close()
	var refs []Ref
	found := map[string]bool{}
	var inactive []string
	for rows.Next() {
		var r Ref
		var active bool
		if err := rows.Scan(&r.ID, &r.Name, &r.Slug, &r.Public, &active); err != nil {
			return nil, err
		}
		found[r.ID] = true
		if !active && !keep[r.ID] {
			inactive = append(inactive, r.Name)
		}
		refs = append(refs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(found) != len(clean) {
		return nil, &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
			Fields: []apperror.FieldError{{Field: field, Message: "Um ou mais componentes não existem."}}}
	}
	if len(inactive) > 0 {
		return nil, &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
			Fields: []apperror.FieldError{{Field: field, Message: "Componentes desativados não podem ser associados: " + strings.Join(inactive, ", ") + "."}}}
	}
	return refs, nil
}

// RefIDs extracts IDs from refs.
func RefIDs(refs []Ref) []string {
	ids := make([]string, len(refs))
	for i, r := range refs {
		ids[i] = r.ID
	}
	return ids
}

// Owner identifies a resource type that associates components. It is a
// closed set so table and column names never come from callers' strings.
type Owner int

// Owners with a component join table.
const (
	OwnerIncident Owner = iota + 1
	OwnerMaintenance
	OwnerAnnouncement
)

var ownerQueries = map[Owner]string{
	OwnerIncident: `SELECT j.incident_id, c.id, c.name, c.slug, c.is_public
		FROM incident_components j JOIN components c ON c.id = j.component_id
		WHERE j.incident_id = ANY($1::uuid[]) ORDER BY c.display_order, c.name, c.id`,
	OwnerMaintenance: `SELECT j.maintenance_id, c.id, c.name, c.slug, c.is_public
		FROM maintenance_components j JOIN components c ON c.id = j.component_id
		WHERE j.maintenance_id = ANY($1::uuid[]) ORDER BY c.display_order, c.name, c.id`,
	OwnerAnnouncement: `SELECT j.announcement_id, c.id, c.name, c.slug, c.is_public
		FROM announcement_components j JOIN components c ON c.id = j.component_id
		WHERE j.announcement_id = ANY($1::uuid[]) ORDER BY c.display_order, c.name, c.id`,
}

// RefsByOwner loads component references for many owners at once, keyed by
// owner ID.
func RefsByOwner(ctx context.Context, q database.Querier, owner Owner, ownerIDs []string) (map[string][]Ref, error) {
	out := map[string][]Ref{}
	if len(ownerIDs) == 0 {
		return out, nil
	}
	sql, ok := ownerQueries[owner]
	if !ok {
		return nil, fmt.Errorf("unknown component owner %d", owner)
	}
	rows, err := q.Query(ctx, sql, ownerIDs)
	if err != nil {
		return nil, fmt.Errorf("load component refs: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var owner string
		var r Ref
		if err := rows.Scan(&owner, &r.ID, &r.Name, &r.Slug, &r.Public); err != nil {
			return nil, err
		}
		out[owner] = append(out[owner], r)
	}
	return out, rows.Err()
}

// RefsByID loads references for arbitrary component IDs (e.g. historical
// snapshots), keyed by ID. Unknown IDs are omitted.
func RefsByID(ctx context.Context, q database.Querier, ids []string) (map[string]Ref, error) {
	out := map[string]Ref{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, "SELECT id, name, slug, is_public FROM components WHERE id = ANY($1::uuid[])", ids)
	if err != nil {
		return nil, fmt.Errorf("load component refs: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var r Ref
		if err := rows.Scan(&r.ID, &r.Name, &r.Slug, &r.Public); err != nil {
			return nil, err
		}
		out[r.ID] = r
	}
	return out, rows.Err()
}
