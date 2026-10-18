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
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
)

// Group organises components on the public page. It has no status of its
// own: the public summary derives it from the group's components.
type Group struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	DisplayOrder   int       `json:"display_order"`
	ComponentCount int       `json:"component_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// GroupInput creates (Name required) or patches (nil = unchanged) a group.
type GroupInput struct {
	Name         *string `json:"name"`
	Description  *string `json:"description"`
	DisplayOrder *int    `json:"display_order"`
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
	if !create && !changed {
		return g, apperror.Invalid("empty_update", "Informe ao menos um campo para atualizar.")
	}
	return g, v.Err()
}

const groupColumns = `id, name, description, display_order,
	(SELECT count(*) FROM components c WHERE c.group_id = component_groups.id)::int, created_at, updated_at`

func scanGroup(row pgx.Row) (Group, error) {
	var g Group
	err := row.Scan(&g.ID, &g.Name, &g.Description, &g.DisplayOrder, &g.ComponentCount, &g.CreatedAt, &g.UpdatedAt)
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

func groupWriteError(err error) error {
	if name, ok := database.ConstraintError(err, database.CodeUniqueViolation); ok && name == "component_groups_name_key" {
		return &apperror.Error{Kind: apperror.KindConflict, Code: "component_group_name_conflict", Message: "Já existe um grupo com este nome.",
			Fields: []apperror.FieldError{{Field: "name", Message: "Nome já utilizado."}}}
	}
	return fmt.Errorf("write component group: %w", err)
}

// CreateGroup stores a new group.
func (s *Service) CreateGroup(ctx context.Context, in GroupInput) (Group, error) {
	g, err := in.apply(Group{}, true)
	if err != nil {
		return Group{}, err
	}
	now := s.now()
	var created Group
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		created, err = scanGroup(tx.QueryRow(ctx, `INSERT INTO component_groups (name, description, display_order, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $4) RETURNING `+groupColumns, g.Name, g.Description, g.DisplayOrder, now))
		if err != nil {
			return groupWriteError(err)
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
		now := s.now()
		updated, err = scanGroup(tx.QueryRow(ctx, `UPDATE component_groups SET name = $2, description = $3, display_order = $4, updated_at = $5
			WHERE id = $1 RETURNING `+groupColumns, id, next.Name, next.Description, next.DisplayOrder, now))
		if err != nil {
			return groupWriteError(err)
		}
		return audit.Record(ctx, tx, audit.ResourceComponentGroup, id, "updated", audit.Diff(current, updated), now)
	})
	return updated, err
}

// DeleteGroup removes an empty group. Components (active or not) must be
// moved out first, so no component silently loses its grouping.
func (s *Service) DeleteGroup(ctx context.Context, id string) error {
	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := GetGroup(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if current.ComponentCount > 0 {
			return apperror.Conflict("component_group_not_empty",
				fmt.Sprintf("O grupo possui %d componente(s). Mova-os para outro grupo ou remova-os do grupo antes de excluí-lo.", current.ComponentCount))
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
