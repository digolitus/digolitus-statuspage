package announcement

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/audit"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
)

// Colors are the category colors. A closed palette keeps the public page
// consistent and accessible (no arbitrary CSS values).
var Colors = []string{"gray", "blue", "green", "yellow", "orange", "red", "purple"}

// Category classifies announcements (e.g. "Novidades", "Avisos").
type Category struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	Color             string    `json:"color"`
	DisplayOrder      int       `json:"display_order"`
	IsActive          bool      `json:"is_active"`
	AnnouncementCount int       `json:"announcement_count"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// CategoryInput creates (Name required) or patches (nil = unchanged).
type CategoryInput struct {
	Name         *string `json:"name"`
	Description  *string `json:"description"`
	Color        *string `json:"color"`
	DisplayOrder *int    `json:"display_order"`
	IsActive     *bool   `json:"is_active"`
}

// Team is the team responsible for an announcement.
type Team struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	IsActive          bool      `json:"is_active"`
	AnnouncementCount int       `json:"announcement_count"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// TeamInput creates (Name required) or patches (nil = unchanged).
type TeamInput struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	IsActive    *bool   `json:"is_active"`
}

func checkName(v *apperror.Validator, name string, maxLen int) {
	n := utf8.RuneCountInString(name)
	v.Check(n >= 1 && n <= maxLen, "name", fmt.Sprintf("Informe um nome com 1 a %d caracteres.", maxLen))
}

func checkDescription(v *apperror.Validator, d string) {
	v.Check(utf8.RuneCountInString(d) <= 500, "description", "A descrição deve ter no máximo 500 caracteres.")
}

func (in CategoryInput) apply(c Category, create bool) (Category, error) {
	var v apperror.Validator
	changed := false
	if in.Name != nil {
		c.Name = strings.TrimSpace(*in.Name)
		changed = true
	}
	if in.Name != nil || create {
		checkName(&v, c.Name, 60)
	}
	if in.Description != nil {
		c.Description = strings.TrimSpace(*in.Description)
		checkDescription(&v, c.Description)
		changed = true
	}
	if in.Color != nil {
		c.Color = *in.Color
		v.Check(slices.Contains(Colors, c.Color), "color", "Cor inválida. Valores aceitos: "+strings.Join(Colors, ", ")+".")
		changed = true
	}
	if in.DisplayOrder != nil {
		c.DisplayOrder = *in.DisplayOrder
		v.Check(c.DisplayOrder >= 0 && c.DisplayOrder <= 100000, "display_order", "Informe um inteiro entre 0 e 100000.")
		changed = true
	}
	if in.IsActive != nil {
		c.IsActive = *in.IsActive
		changed = true
	}
	if !create && !changed {
		return c, apperror.Invalid("empty_update", "Informe ao menos um campo para atualizar.")
	}
	return c, v.Err()
}

func (in TeamInput) apply(t Team, create bool) (Team, error) {
	var v apperror.Validator
	changed := false
	if in.Name != nil {
		t.Name = strings.TrimSpace(*in.Name)
		changed = true
	}
	if in.Name != nil || create {
		checkName(&v, t.Name, 80)
	}
	if in.Description != nil {
		t.Description = strings.TrimSpace(*in.Description)
		checkDescription(&v, t.Description)
		changed = true
	}
	if in.IsActive != nil {
		t.IsActive = *in.IsActive
		changed = true
	}
	if !create && !changed {
		return t, apperror.Invalid("empty_update", "Informe ao menos um campo para atualizar.")
	}
	return t, v.Err()
}

const categoryColumns = `id, name, description, color, display_order, is_active,
	(SELECT count(*) FROM announcements a WHERE a.category_id = announcement_categories.id)::int, created_at, updated_at`

func scanCategory(row pgx.Row) (Category, error) {
	var c Category
	err := row.Scan(&c.ID, &c.Name, &c.Description, &c.Color, &c.DisplayOrder, &c.IsActive, &c.AnnouncementCount, &c.CreatedAt, &c.UpdatedAt)
	c.CreatedAt, c.UpdatedAt = c.CreatedAt.UTC(), c.UpdatedAt.UTC()
	return c, err
}

const teamColumns = `id, name, description, is_active,
	(SELECT count(*) FROM announcements a WHERE a.team_id = teams.id)::int, created_at, updated_at`

func scanTeam(row pgx.Row) (Team, error) {
	var t Team
	err := row.Scan(&t.ID, &t.Name, &t.Description, &t.IsActive, &t.AnnouncementCount, &t.CreatedAt, &t.UpdatedAt)
	t.CreatedAt, t.UpdatedAt = t.CreatedAt.UTC(), t.UpdatedAt.UTC()
	return t, err
}

func getCategory(ctx context.Context, q database.Querier, id string, lock bool) (Category, error) {
	sql := "SELECT " + categoryColumns + " FROM announcement_categories WHERE id = $1"
	if lock {
		sql += " FOR UPDATE"
	}
	c, err := scanCategory(q.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Category{}, apperror.NotFound("announcement_category")
	}
	if err != nil {
		return Category{}, fmt.Errorf("get announcement category: %w", err)
	}
	return c, nil
}

func getTeam(ctx context.Context, q database.Querier, id string, lock bool) (Team, error) {
	sql := "SELECT " + teamColumns + " FROM teams WHERE id = $1"
	if lock {
		sql += " FOR UPDATE"
	}
	t, err := scanTeam(q.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Team{}, apperror.NotFound("team")
	}
	if err != nil {
		return Team{}, fmt.Errorf("get team: %w", err)
	}
	return t, nil
}

// listCategories returns categories in display order (at most 100: a
// small, curated set). active filters by is_active when not nil.
func listCategories(ctx context.Context, q database.Querier, active *bool) ([]Category, error) {
	rows, err := q.Query(ctx, "SELECT "+categoryColumns+` FROM announcement_categories
		WHERE ($1::boolean IS NULL OR is_active = $1) ORDER BY display_order, lower(name), id LIMIT 100`, active)
	if err != nil {
		return nil, fmt.Errorf("list announcement categories: %w", err)
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Category, error) { return scanCategory(r) })
}

func listTeams(ctx context.Context, q database.Querier, active *bool) ([]Team, error) {
	rows, err := q.Query(ctx, "SELECT "+teamColumns+` FROM teams
		WHERE ($1::boolean IS NULL OR is_active = $1) ORDER BY lower(name), id LIMIT 100`, active)
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Team, error) { return scanTeam(r) })
}

func nameConflict(err error, constraint, code, message string) error {
	if name, ok := database.ConstraintError(err, database.CodeUniqueViolation); ok && name == constraint {
		return &apperror.Error{Kind: apperror.KindConflict, Code: code, Message: message,
			Fields: []apperror.FieldError{{Field: "name", Message: "Nome já utilizado."}}}
	}
	return err
}

const maxTaxonomy = 100

func (s *Service) countRows(ctx context.Context, tx pgx.Tx, table string) (int, error) {
	var n int
	// #nosec G202 -- table is one of two constants in this file.
	err := tx.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n)
	return n, err
}

// CreateCategory stores a new category.
func (s *Service) CreateCategory(ctx context.Context, in CategoryInput) (Category, error) {
	c, err := in.apply(Category{Color: "gray", IsActive: true}, true)
	if err != nil {
		return Category{}, err
	}
	now := s.now()
	var created Category
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if n, err := s.countRows(ctx, tx, "announcement_categories"); err != nil {
			return err
		} else if n >= maxTaxonomy {
			return apperror.Conflict("announcement_category_limit", "Limite de 100 categorias atingido. Desative ou reutilize categorias existentes.")
		}
		created, err = scanCategory(tx.QueryRow(ctx, `INSERT INTO announcement_categories (name, description, color, display_order, is_active, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $6) RETURNING `+categoryColumns, c.Name, c.Description, c.Color, c.DisplayOrder, c.IsActive, now))
		if err != nil {
			return nameConflict(err, "announcement_categories_name_key", "announcement_category_name_conflict", "Já existe uma categoria com este nome.")
		}
		return audit.Record(ctx, tx, audit.ResourceAnnouncementCategory, created.ID, "created", audit.Changes{After: created}, now)
	})
	return created, err
}

// UpdateCategory patches a category.
func (s *Service) UpdateCategory(ctx context.Context, id string, in CategoryInput) (Category, error) {
	var updated Category
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := getCategory(ctx, tx, id, true)
		if err != nil {
			return err
		}
		next, err := in.apply(current, false)
		if err != nil {
			return err
		}
		now := s.now()
		updated, err = scanCategory(tx.QueryRow(ctx, `UPDATE announcement_categories SET name = $2, description = $3, color = $4,
			display_order = $5, is_active = $6, updated_at = $7 WHERE id = $1 RETURNING `+categoryColumns,
			id, next.Name, next.Description, next.Color, next.DisplayOrder, next.IsActive, now))
		if err != nil {
			return nameConflict(err, "announcement_categories_name_key", "announcement_category_name_conflict", "Já existe uma categoria com este nome.")
		}
		return audit.Record(ctx, tx, audit.ResourceAnnouncementCategory, id, "updated", audit.Diff(current, updated), now)
	})
	return updated, err
}

// DeleteCategory removes a category that no announcement uses.
func (s *Service) DeleteCategory(ctx context.Context, id string) error {
	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := getCategory(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if current.AnnouncementCount > 0 {
			return apperror.Conflict("announcement_category_in_use",
				fmt.Sprintf("A categoria é usada por %d comunicado(s). Desative-a para que não seja mais oferecida.", current.AnnouncementCount))
		}
		if _, err := tx.Exec(ctx, "DELETE FROM announcement_categories WHERE id = $1", id); err != nil {
			if _, ok := database.ConstraintError(err, database.CodeForeignKeyViolation); ok {
				return apperror.Conflict("announcement_category_in_use", "A categoria ainda é usada por comunicados.")
			}
			return fmt.Errorf("delete announcement category: %w", err)
		}
		return audit.Record(ctx, tx, audit.ResourceAnnouncementCategory, id, "deleted", audit.Changes{Before: current}, s.now())
	})
}

// CreateTeam stores a new team.
func (s *Service) CreateTeam(ctx context.Context, in TeamInput) (Team, error) {
	t, err := in.apply(Team{IsActive: true}, true)
	if err != nil {
		return Team{}, err
	}
	now := s.now()
	var created Team
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if n, err := s.countRows(ctx, tx, "teams"); err != nil {
			return err
		} else if n >= maxTaxonomy {
			return apperror.Conflict("team_limit", "Limite de 100 times atingido. Desative ou reutilize times existentes.")
		}
		created, err = scanTeam(tx.QueryRow(ctx, `INSERT INTO teams (name, description, is_active, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $4) RETURNING `+teamColumns, t.Name, t.Description, t.IsActive, now))
		if err != nil {
			return nameConflict(err, "teams_name_key", "team_name_conflict", "Já existe um time com este nome.")
		}
		return audit.Record(ctx, tx, audit.ResourceTeam, created.ID, "created", audit.Changes{After: created}, now)
	})
	return created, err
}

// UpdateTeam patches a team.
func (s *Service) UpdateTeam(ctx context.Context, id string, in TeamInput) (Team, error) {
	var updated Team
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := getTeam(ctx, tx, id, true)
		if err != nil {
			return err
		}
		next, err := in.apply(current, false)
		if err != nil {
			return err
		}
		now := s.now()
		updated, err = scanTeam(tx.QueryRow(ctx, `UPDATE teams SET name = $2, description = $3, is_active = $4, updated_at = $5
			WHERE id = $1 RETURNING `+teamColumns, id, next.Name, next.Description, next.IsActive, now))
		if err != nil {
			return nameConflict(err, "teams_name_key", "team_name_conflict", "Já existe um time com este nome.")
		}
		return audit.Record(ctx, tx, audit.ResourceTeam, id, "updated", audit.Diff(current, updated), now)
	})
	return updated, err
}

// DeleteTeam removes a team that no announcement references.
func (s *Service) DeleteTeam(ctx context.Context, id string) error {
	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := getTeam(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if current.AnnouncementCount > 0 {
			return apperror.Conflict("team_in_use",
				fmt.Sprintf("O time é responsável por %d comunicado(s). Desative-o para que não seja mais oferecido.", current.AnnouncementCount))
		}
		if _, err := tx.Exec(ctx, "DELETE FROM teams WHERE id = $1", id); err != nil {
			if _, ok := database.ConstraintError(err, database.CodeForeignKeyViolation); ok {
				return apperror.Conflict("team_in_use", "O time ainda é usado por comunicados.")
			}
			return fmt.Errorf("delete team: %w", err)
		}
		return audit.Record(ctx, tx, audit.ResourceTeam, id, "deleted", audit.Changes{Before: current}, s.now())
	})
}

// --- HTTP ---

func (h *Handler) registerTaxonomy(rt *httpx.Router, admin func(http.HandlerFunc) http.HandlerFunc) {
	rt.Handle(http.MethodGet, "/api/v1/admin/announcement-categories", admin(h.listCategories))
	rt.Handle(http.MethodPost, "/api/v1/admin/announcement-categories", admin(h.createCategory))
	rt.Handle(http.MethodGet, "/api/v1/admin/announcement-categories/{id}", admin(h.getCategory))
	rt.Handle(http.MethodPatch, "/api/v1/admin/announcement-categories/{id}", admin(h.updateCategory))
	rt.Handle(http.MethodDelete, "/api/v1/admin/announcement-categories/{id}", admin(h.deleteCategory))
	rt.Handle(http.MethodGet, "/api/v1/admin/teams", admin(h.listTeams))
	rt.Handle(http.MethodPost, "/api/v1/admin/teams", admin(h.createTeam))
	rt.Handle(http.MethodGet, "/api/v1/admin/teams/{id}", admin(h.getTeam))
	rt.Handle(http.MethodPatch, "/api/v1/admin/teams/{id}", admin(h.updateTeam))
	rt.Handle(http.MethodDelete, "/api/v1/admin/teams/{id}", admin(h.deleteTeam))
}

// itemsEnvelope is a non-paginated collection (small, bounded sets).
type itemsEnvelope[T any] struct {
	Data []T `json:"data"`
}

func items[T any](xs []T) itemsEnvelope[T] {
	if xs == nil {
		xs = []T{}
	}
	return itemsEnvelope[T]{Data: xs}
}

func (h *Handler) listCategories(w http.ResponseWriter, r *http.Request) {
	q := httpx.NewQuery(r.URL.Query())
	active := q.Bool("is_active")
	if err := q.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cs, err := listCategories(r.Context(), h.svc.pool, active)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, items(cs))
}

func (h *Handler) getCategory(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "announcement_category")
	if err == nil {
		var c Category
		if c, err = getCategory(r.Context(), h.svc.pool, id, false); err == nil {
			httpx.WriteJSON(w, http.StatusOK, c)
			return
		}
	}
	httpx.Fail(w, r, err)
}

func (h *Handler) createCategory(w http.ResponseWriter, r *http.Request) {
	var in CategoryInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := h.svc.CreateCategory(r.Context(), in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/admin/announcement-categories/"+c.ID)
	httpx.WriteJSON(w, http.StatusCreated, c)
}

func (h *Handler) updateCategory(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "announcement_category")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in CategoryInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := h.svc.UpdateCategory(r.Context(), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, c)
}

func (h *Handler) deleteCategory(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "announcement_category")
	if err == nil {
		err = h.svc.DeleteCategory(r.Context(), id)
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listTeams(w http.ResponseWriter, r *http.Request) {
	q := httpx.NewQuery(r.URL.Query())
	active := q.Bool("is_active")
	if err := q.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ts, err := listTeams(r.Context(), h.svc.pool, active)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, items(ts))
}

func (h *Handler) getTeam(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "team")
	if err == nil {
		var t Team
		if t, err = getTeam(r.Context(), h.svc.pool, id, false); err == nil {
			httpx.WriteJSON(w, http.StatusOK, t)
			return
		}
	}
	httpx.Fail(w, r, err)
}

func (h *Handler) createTeam(w http.ResponseWriter, r *http.Request) {
	var in TeamInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, err := h.svc.CreateTeam(r.Context(), in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/admin/teams/"+t.ID)
	httpx.WriteJSON(w, http.StatusCreated, t)
}

func (h *Handler) updateTeam(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "team")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in TeamInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, err := h.svc.UpdateTeam(r.Context(), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, t)
}

func (h *Handler) deleteTeam(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "team")
	if err == nil {
		err = h.svc.DeleteTeam(r.Context(), id)
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
