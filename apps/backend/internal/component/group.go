package component

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/audit"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/auth"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/optional"
)

// Group organises components on the public page. It has no status of its
// own: the public summary derives it from the group's components.
//
// A group with ParentID set is a subgroup. There are only two levels
// (Group -> Subgroup -> Component) and subgroups are optional: components
// may sit directly in a group.
type Group struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	DisplayOrder int    `json:"display_order"`
	// ParentID is the top-level group of a subgroup; nil for groups.
	ParentID *string `json:"parent_id"`
	// TeamID is the team responsible for the group; components without a
	// team of their own inherit it (subgroup's team first, then group's).
	TeamID *string `json:"team_id"`
	// ComponentCount includes the components of the group's subgroups.
	ComponentCount int       `json:"component_count"`
	SubgroupCount  int       `json:"subgroup_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// IsSubgroup reports whether g is a subgroup.
func (g Group) IsSubgroup() bool { return g.ParentID != nil }

// GroupInput creates (Name required) or patches (nil = unchanged) a group.
type GroupInput struct {
	Name         *string `json:"name"`
	Description  *string `json:"description"`
	DisplayOrder *int    `json:"display_order"`
	// ParentID: absent = unchanged, null = top-level group, id = subgroup
	// of that (top-level) group.
	ParentID optional.Nullable[string] `json:"parent_id"`
	// TeamID: absent = unchanged, null = no team.
	TeamID optional.Nullable[string] `json:"team_id"`
}

func (in GroupInput) apply(g Group, create bool) (Group, error) {
	var v apperror.Validator
	changed := false
	if in.Name != nil {
		g.Name = strings.TrimSpace(*in.Name)
		changed = true
	}
	if in.Name != nil || create {
		n := utf8.RuneCountInString(g.Name)
		v.Check(n >= 1 && n <= 120, "name", "Informe um nome com 1 a 120 caracteres.")
	}
	if in.Description != nil {
		g.Description = strings.TrimSpace(*in.Description)
		validateDescription(&v, g.Description)
		changed = true
	}
	if in.DisplayOrder != nil {
		g.DisplayOrder = *in.DisplayOrder
		validateOrder(&v, g.DisplayOrder)
		changed = true
	}
	if in.TeamID.Set {
		g.TeamID = normalizeRef(&v, "team_id", in.TeamID.Value)
		changed = true
	}
	if in.ParentID.Set {
		g.ParentID = normalizeRef(&v, "parent_id", in.ParentID.Value)
		v.Check(g.ParentID == nil || g.ID == "" || *g.ParentID != g.ID, "parent_id", "Um grupo não pode ser subgrupo de si mesmo.")
		changed = true
	}
	if !create && !changed {
		return g, apperror.Invalid("empty_update", "Informe ao menos um campo para atualizar.")
	}
	return g, v.Err()
}

// component_count: components.group_id is always the top-level group, so a
// group counts its subgroups' components too; a subgroup counts its own.
const groupColumns = `id, name, description, display_order, parent_id, team_id,
	(SELECT count(*) FROM components c WHERE c.group_id = component_groups.id OR c.subgroup_id = component_groups.id)::int,
	(SELECT count(*) FROM component_groups s WHERE s.parent_id = component_groups.id)::int, created_at, updated_at`

func scanGroup(row pgx.Row) (Group, error) {
	var g Group
	err := row.Scan(&g.ID, &g.Name, &g.Description, &g.DisplayOrder, &g.ParentID, &g.TeamID, &g.ComponentCount, &g.SubgroupCount, &g.CreatedAt, &g.UpdatedAt)
	g.CreatedAt, g.UpdatedAt = g.CreatedAt.UTC(), g.UpdatedAt.UTC()
	return g, err
}

// GetGroup loads a group, optionally locking it.
func GetGroup(ctx context.Context, q database.Querier, id string, lock bool) (Group, error) {
	sql := "SELECT " + groupColumns + " FROM component_groups WHERE id = $1"
	if lock {
		sql += " FOR UPDATE"
	}
	g, err := scanGroup(q.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Group{}, apperror.NotFound("component_group")
	}
	if err != nil {
		return Group{}, fmt.Errorf("get component group: %w", err)
	}
	return g, nil
}

// AllGroups returns every group in display order.
func AllGroups(ctx context.Context, q database.Querier) ([]Group, error) {
	rows, err := q.Query(ctx, "SELECT "+groupColumns+" FROM component_groups ORDER BY display_order, name, id")
	if err != nil {
		return nil, fmt.Errorf("list component groups: %w", err)
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

var groupSort = map[string]string{"display_order": "display_order", "name": "lower(name)", "created_at": "created_at"}

func listGroups(ctx context.Context, q database.Querier, sort httpx.Sort, page httpx.Page) ([]Group, int, error) {
	var total int
	if err := q.QueryRow(ctx, "SELECT count(*) FROM component_groups").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count component groups: %w", err)
	}
	dir := "ASC"
	if sort.Desc {
		dir = "DESC"
	}
	// #nosec G202 -- ORDER BY comes from the groupSort whitelist.
	rows, err := q.Query(ctx, "SELECT "+groupColumns+" FROM component_groups ORDER BY "+groupSort[sort.Field]+" "+dir+", name ASC, id ASC LIMIT $1 OFFSET $2",
		page.Limit, page.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list component groups: %w", err)
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, g)
	}
	return out, total, rows.Err()
}

func groupWriteError(err error, subgroup bool) error {
	if name, ok := database.ConstraintError(err, database.CodeUniqueViolation); ok && name == "component_groups_name_key" {
		msg := "Já existe um grupo com este nome."
		if subgroup {
			msg = "Já existe um subgrupo com este nome neste grupo."
		}
		return &apperror.Error{Kind: apperror.KindConflict, Code: "component_group_name_conflict", Message: msg,
			Fields: []apperror.FieldError{{Field: "name", Message: "Nome já utilizado."}}}
	}
	if name, ok := database.ConstraintError(err, database.CodeForeignKeyViolation); ok && name == "component_groups_team_id_fkey" {
		return &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
			Fields: []apperror.FieldError{{Field: "team_id", Message: "Time não encontrado."}}}
	}
	return fmt.Errorf("write component group: %w", err)
}

// checkParent validates the parent of a subgroup: it must exist and be a
// top-level group. It is locked so it cannot become a subgroup meanwhile.
func checkParent(ctx context.Context, q database.Querier, parentID string) error {
	parent, err := GetGroup(ctx, q, parentID, true)
	var v apperror.Validator
	if e, ok := apperror.As(err); ok && e.Kind == apperror.KindNotFound {
		v.Add("parent_id", "Grupo não encontrado.")
		return v.Err()
	}
	if err != nil {
		return err
	}
	if parent.IsSubgroup() {
		v.Add("parent_id", "Subgrupos só podem ficar dentro de um grupo principal (não há subgrupo de subgrupo).")
	}
	return v.Err()
}

// CreateGroup stores a new group.
func (s *Service) CreateGroup(ctx context.Context, in GroupInput) (Group, error) {
	g, err := in.apply(Group{}, true)
	if err != nil {
		return Group{}, err
	}
	if err := auth.RequireTeam(ctx, auth.PermComponentsManage, g.TeamID); err != nil {
		return Group{}, err
	}
	now := s.now()
	var created Group
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if g.ParentID != nil {
			if err := checkParent(ctx, tx, *g.ParentID); err != nil {
				return err
			}
		}
		created, err = scanGroup(tx.QueryRow(ctx, `INSERT INTO component_groups (name, description, display_order, parent_id, team_id, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $6) RETURNING `+groupColumns, g.Name, g.Description, g.DisplayOrder, g.ParentID, g.TeamID, now))
		if err != nil {
			return groupWriteError(err, g.ParentID != nil)
		}
		return audit.Record(ctx, tx, audit.ResourceComponentGroup, created.ID, "created", audit.Changes{After: created}, now)
	})
	return created, err
}

// UpdateGroup patches a group.
func (s *Service) UpdateGroup(ctx context.Context, id string, in GroupInput) (Group, error) {
	var updated Group
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := GetGroup(ctx, tx, id, true)
		if err != nil {
			return err
		}
		next, err := in.apply(current, false)
		if err != nil {
			return err
		}
		if err := auth.RequireTeam(ctx, auth.PermComponentsManage, current.TeamID); err != nil {
			return err
		}
		if err := auth.RequireTeam(ctx, auth.PermComponentsManage, next.TeamID); err != nil {
			return err
		}
		if err := checkLevelChange(ctx, tx, current, next); err != nil {
			return err
		}
		now := s.now()
		// Moving a subgroup to another group moves its components too
		// (components_subgroup_id_fkey cascades group_id).
		updated, err = scanGroup(tx.QueryRow(ctx, `UPDATE component_groups SET name = $2, description = $3, display_order = $4, parent_id = $5, team_id = $6, updated_at = $7
			WHERE id = $1 RETURNING `+groupColumns, id, next.Name, next.Description, next.DisplayOrder, next.ParentID, next.TeamID, now))
		if err != nil {
			return groupWriteError(err, next.ParentID != nil)
		}
		return audit.Record(ctx, tx, audit.ResourceComponentGroup, id, "updated", audit.Diff(current, updated), now)
	})
	return updated, err
}

// checkLevelChange enforces the two-level hierarchy on a parent change. A
// subgroup may move to another group; turning a group into a subgroup (or
// back) is only allowed while it has no components and no subgroups, so no
// component ends up pointing at the wrong level.
func checkLevelChange(ctx context.Context, q database.Querier, current, next Group) error {
	if sameGroup(current.ParentID, next.ParentID) {
		return nil
	}
	if current.IsSubgroup() != next.IsSubgroup() && (current.ComponentCount > 0 || current.SubgroupCount > 0) {
		return apperror.Conflict("component_group_level_locked",
			"Só é possível transformar um grupo em subgrupo (ou o contrário) quando ele não tem componentes nem subgrupos.")
	}
	if next.ParentID != nil {
		return checkParent(ctx, q, *next.ParentID)
	}
	return nil
}

// DeleteGroup removes an empty group. Components (active or not) and
// subgroups must be moved out first, so nothing silently loses its grouping.
func (s *Service) DeleteGroup(ctx context.Context, id string) error {
	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := GetGroup(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if err := auth.RequireTeam(ctx, auth.PermComponentsManage, current.TeamID); err != nil {
			return err
		}
		if current.ComponentCount > 0 {
			return apperror.Conflict("component_group_not_empty",
				fmt.Sprintf("O grupo possui %d componente(s). Mova-os para outro grupo ou remova-os do grupo antes de excluí-lo.", current.ComponentCount))
		}
		if current.SubgroupCount > 0 {
			return apperror.Conflict("component_group_has_subgroups",
				fmt.Sprintf("O grupo possui %d subgrupo(s). Exclua-os ou mova-os para outro grupo antes de excluí-lo.", current.SubgroupCount))
		}
		if _, err := tx.Exec(ctx, "DELETE FROM component_groups WHERE id = $1", id); err != nil {
			if _, ok := database.ConstraintError(err, database.CodeForeignKeyViolation); ok {
				return apperror.Conflict("component_group_not_empty", "O grupo ainda possui componentes.")
			}
			return fmt.Errorf("delete component group: %w", err)
		}
		return audit.Record(ctx, tx, audit.ResourceComponentGroup, id, "deleted", audit.Changes{Before: current}, s.now())
	})
}

// RegisterGroups mounts admin group routes.
func (h *Handler) RegisterGroups(rt *httpx.Router, admin func(http.HandlerFunc) http.HandlerFunc) {
	rt.Handle(http.MethodGet, "/api/v1/admin/component-groups", admin(h.listGroups))
	rt.Handle(http.MethodPost, "/api/v1/admin/component-groups", admin(h.createGroup))
	rt.Handle(http.MethodGet, "/api/v1/admin/component-groups/{id}", admin(h.getGroup))
	rt.Handle(http.MethodPatch, "/api/v1/admin/component-groups/{id}", admin(h.updateGroup))
	rt.Handle(http.MethodDelete, "/api/v1/admin/component-groups/{id}", admin(h.deleteGroup))
}

func (h *Handler) listGroups(w http.ResponseWriter, r *http.Request) {
	q := httpx.NewQuery(r.URL.Query())
	page := q.Page()
	sort := q.Sort([]string{"display_order", "name", "created_at"}, httpx.Sort{Field: "display_order"})
	if err := q.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, total, err := listGroups(r.Context(), h.pool, sort, page)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewList(items, page, total))
}

func (h *Handler) getGroup(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "component_group")
	if err == nil {
		var g Group
		if g, err = GetGroup(r.Context(), h.pool, id, false); err == nil {
			httpx.WriteJSON(w, http.StatusOK, g)
			return
		}
	}
	httpx.Fail(w, r, err)
}

func (h *Handler) createGroup(w http.ResponseWriter, r *http.Request) {
	var in GroupInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	g, err := h.svc.CreateGroup(r.Context(), in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/admin/component-groups/"+g.ID)
	httpx.WriteJSON(w, http.StatusCreated, g)
}

func (h *Handler) updateGroup(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "component_group")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in GroupInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	g, err := h.svc.UpdateGroup(r.Context(), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, g)
}

func (h *Handler) deleteGroup(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "component_group")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.DeleteGroup(r.Context(), id); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
