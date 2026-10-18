// Package templates manages reusable incident and maintenance templates.
//
// A template only pre-fills a form: creating an incident or maintenance
// copies the text, and the created resource keeps just the template ID for
// traceability. Editing or deactivating a template therefore never changes
// published history. Templates are deactivated, not deleted.
package templates

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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/audit"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/clock"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/statuspolicy"
)

// Valid enumerations, duplicated here as plain values to keep this package
// independent of the incident and maintenance modules.
var (
	incidentImpacts    = statuspolicy.Impacts
	incidentStatuses   = []string{"investigating", "identified", "monitoring", "resolved"}
	maintenanceImpacts = statuspolicy.Impacts
)

// IncidentTemplate pre-fills incident creation or update forms.
type IncidentTemplate struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Title       string    `json:"title"`
	Message     string    `json:"message"`
	Impact      string    `json:"impact"`
	Status      string    `json:"status"`
	Guidance    string    `json:"guidance"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// MaintenanceTemplate pre-fills maintenance creation forms.
type MaintenanceTemplate struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	Title          string    `json:"title"`
	Message        string    `json:"message"`
	ExpectedImpact string    `json:"expected_impact"`
	Guidance       string    `json:"guidance"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Input carries create (all required fields set) or patch (nil = unchanged)
// payloads for both template kinds; kind-specific fields are rejected for
// the other kind by the decoder via separate wrapper types below.
type commonInput struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Title       *string `json:"title"`
	Message     *string `json:"message"`
	Guidance    *string `json:"guidance"`
	IsActive    *bool   `json:"is_active"`
}

// IncidentInput is the create/patch payload for incident templates.
type IncidentInput struct {
	commonInput
	Impact *string `json:"impact"`
	Status *string `json:"status"`
}

// MaintenanceInput is the create/patch payload for maintenance templates.
type MaintenanceInput struct {
	commonInput
	ExpectedImpact *string `json:"expected_impact"`
}

type common struct {
	Name, Description, Title, Message, Guidance string
	IsActive                                    bool
}

func textField(v *apperror.Validator, field string, value *string, target *string, min, max int, create bool) bool {
	if value == nil {
		if create && min > 0 {
			v.Add(field, fmt.Sprintf("Campo obrigatório (%d a %d caracteres).", min, max))
		}
		return false
	}
	*target = strings.TrimSpace(*value)
	n := utf8.RuneCountInString(*target)
	v.Check(n >= min && n <= max, field, fmt.Sprintf("Informe de %d a %d caracteres.", min, max))
	return true
}

func (in commonInput) apply(v *apperror.Validator, c *common, create bool) bool {
	changed := false
	changed = textField(v, "name", in.Name, &c.Name, 1, 120, create) || changed
	changed = textField(v, "description", in.Description, &c.Description, 0, 1000, create) || changed
	changed = textField(v, "title", in.Title, &c.Title, 1, 200, create) || changed
	changed = textField(v, "message", in.Message, &c.Message, 1, 5000, create) || changed
	changed = textField(v, "guidance", in.Guidance, &c.Guidance, 0, 2000, create) || changed
	if in.IsActive != nil {
		c.IsActive = *in.IsActive
		changed = true
	}
	return changed
}

func enumField(v *apperror.Validator, field string, value *string, target *string, allowed []string) bool {
	if value == nil {
		return false
	}
	*target = *value
	v.Check(slices.Contains(allowed, *value), field, "Valor inválido. Valores aceitos: "+strings.Join(allowed, ", ")+".")
	return true
}

func nameConflict(err error) error {
	if name, ok := database.ConstraintError(err, database.CodeUniqueViolation); ok &&
		(name == "incident_templates_name_key" || name == "maintenance_templates_name_key") {
		return &apperror.Error{Kind: apperror.KindConflict, Code: "template_name_conflict", Message: "Já existe um template com este nome.",
			Fields: []apperror.FieldError{{Field: "name", Message: "Nome já utilizado."}}}
	}
	return err
}

var sortColumns = map[string]string{"name": "lower(name)", "created_at": "created_at", "updated_at": "updated_at"}

func orderBy(s httpx.Sort) string {
	dir := "ASC"
	if s.Desc {
		dir = "DESC"
	}
	return sortColumns[s.Field] + " " + dir + ", id " + dir
}

// ---- incident templates ----

const incidentColumns = "id, name, description, title, message, impact, status, guidance, is_active, created_at, updated_at"

func scanIncident(row pgx.Row) (IncidentTemplate, error) {
	var t IncidentTemplate
	err := row.Scan(&t.ID, &t.Name, &t.Description, &t.Title, &t.Message, &t.Impact, &t.Status, &t.Guidance, &t.IsActive, &t.CreatedAt, &t.UpdatedAt)
	t.CreatedAt, t.UpdatedAt = t.CreatedAt.UTC(), t.UpdatedAt.UTC()
	return t, err
}

func getIncident(ctx context.Context, q database.Querier, id string, lock bool) (IncidentTemplate, error) {
	sql := "SELECT " + incidentColumns + " FROM incident_templates WHERE id = $1"
	if lock {
		sql += " FOR UPDATE"
	}
	t, err := scanIncident(q.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return t, apperror.NotFound("incident_template")
	}
	return t, err
}

// ---- maintenance templates ----

const maintenanceColumns = "id, name, description, title, message, expected_impact, guidance, is_active, created_at, updated_at"

func scanMaintenance(row pgx.Row) (MaintenanceTemplate, error) {
	var t MaintenanceTemplate
	err := row.Scan(&t.ID, &t.Name, &t.Description, &t.Title, &t.Message, &t.ExpectedImpact, &t.Guidance, &t.IsActive, &t.CreatedAt, &t.UpdatedAt)
	t.CreatedAt, t.UpdatedAt = t.CreatedAt.UTC(), t.UpdatedAt.UTC()
	return t, err
}

func getMaintenance(ctx context.Context, q database.Querier, id string, lock bool) (MaintenanceTemplate, error) {
	sql := "SELECT " + maintenanceColumns + " FROM maintenance_templates WHERE id = $1"
	if lock {
		sql += " FOR UPDATE"
	}
	t, err := scanMaintenance(q.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return t, apperror.NotFound("maintenance_template")
	}
	return t, err
}

// Service implements template use cases.
type Service struct {
	pool *pgxpool.Pool
	now  clock.Clock
}

// NewService creates a Service.
func NewService(pool *pgxpool.Pool, now clock.Clock) *Service {
	return &Service{pool: pool, now: now}
}

// CreateIncident validates and stores an incident template.
func (s *Service) CreateIncident(ctx context.Context, in IncidentInput) (IncidentTemplate, error) {
	var v apperror.Validator
	c := common{IsActive: true}
	in.apply(&v, &c, true)
	t := IncidentTemplate{Status: "investigating"}
	if in.Impact == nil {
		v.Add("impact", "Campo obrigatório.")
	}
	enumField(&v, "impact", in.Impact, &t.Impact, incidentImpacts)
	enumField(&v, "status", in.Status, &t.Status, incidentStatuses)
	if err := v.Err(); err != nil {
		return t, err
	}
	now := s.now()
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		t, err = scanIncident(tx.QueryRow(ctx, `INSERT INTO incident_templates (name, description, title, message, impact, status, guidance, is_active, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9) RETURNING `+incidentColumns,
			c.Name, c.Description, c.Title, c.Message, t.Impact, t.Status, c.Guidance, c.IsActive, now))
		if err != nil {
			return nameConflict(err)
		}
		return audit.Record(ctx, tx, audit.ResourceIncidentTemplate, t.ID, "created", audit.Changes{After: t}, now)
	})
	return t, err
}

// PatchIncident updates an incident template.
func (s *Service) PatchIncident(ctx context.Context, id string, in IncidentInput) (IncidentTemplate, error) {
	var out IncidentTemplate
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		cur, err := getIncident(ctx, tx, id, true)
		if err != nil {
			return err
		}
		var v apperror.Validator
		c := common{Name: cur.Name, Description: cur.Description, Title: cur.Title, Message: cur.Message, Guidance: cur.Guidance, IsActive: cur.IsActive}
		next := cur
		changed := in.apply(&v, &c, false)
		changed = enumField(&v, "impact", in.Impact, &next.Impact, incidentImpacts) || changed
		changed = enumField(&v, "status", in.Status, &next.Status, incidentStatuses) || changed
		if !changed {
			return apperror.Invalid("empty_update", "Informe ao menos um campo para atualizar.")
		}
		if err := v.Err(); err != nil {
			return err
		}
		now := s.now()
		out, err = scanIncident(tx.QueryRow(ctx, `UPDATE incident_templates SET name = $2, description = $3, title = $4, message = $5,
			impact = $6, status = $7, guidance = $8, is_active = $9, updated_at = $10 WHERE id = $1 RETURNING `+incidentColumns,
			id, c.Name, c.Description, c.Title, c.Message, next.Impact, next.Status, c.Guidance, c.IsActive, now))
		if err != nil {
			return nameConflict(err)
		}
		return audit.Record(ctx, tx, audit.ResourceIncidentTemplate, id, activationAction(cur.IsActive, out.IsActive), audit.Diff(cur, out), now)
	})
	return out, err
}

// CreateMaintenance validates and stores a maintenance template.
func (s *Service) CreateMaintenance(ctx context.Context, in MaintenanceInput) (MaintenanceTemplate, error) {
	var v apperror.Validator
	c := common{IsActive: true}
	in.apply(&v, &c, true)
	t := MaintenanceTemplate{ExpectedImpact: statuspolicy.Operational}
	enumField(&v, "expected_impact", in.ExpectedImpact, &t.ExpectedImpact, maintenanceImpacts)
	if err := v.Err(); err != nil {
		return t, err
	}
	now := s.now()
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		t, err = scanMaintenance(tx.QueryRow(ctx, `INSERT INTO maintenance_templates (name, description, title, message, expected_impact, guidance, is_active, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8) RETURNING `+maintenanceColumns,
			c.Name, c.Description, c.Title, c.Message, t.ExpectedImpact, c.Guidance, c.IsActive, now))
		if err != nil {
			return nameConflict(err)
		}
		return audit.Record(ctx, tx, audit.ResourceMaintenanceTemplate, t.ID, "created", audit.Changes{After: t}, now)
	})
	return t, err
}

// PatchMaintenance updates a maintenance template.
func (s *Service) PatchMaintenance(ctx context.Context, id string, in MaintenanceInput) (MaintenanceTemplate, error) {
	var out MaintenanceTemplate
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		cur, err := getMaintenance(ctx, tx, id, true)
		if err != nil {
			return err
		}
		var v apperror.Validator
		c := common{Name: cur.Name, Description: cur.Description, Title: cur.Title, Message: cur.Message, Guidance: cur.Guidance, IsActive: cur.IsActive}
		next := cur
		changed := in.apply(&v, &c, false)
		changed = enumField(&v, "expected_impact", in.ExpectedImpact, &next.ExpectedImpact, maintenanceImpacts) || changed
		if !changed {
			return apperror.Invalid("empty_update", "Informe ao menos um campo para atualizar.")
		}
		if err := v.Err(); err != nil {
			return err
		}
		now := s.now()
		out, err = scanMaintenance(tx.QueryRow(ctx, `UPDATE maintenance_templates SET name = $2, description = $3, title = $4, message = $5,
			expected_impact = $6, guidance = $7, is_active = $8, updated_at = $9 WHERE id = $1 RETURNING `+maintenanceColumns,
			id, c.Name, c.Description, c.Title, c.Message, next.ExpectedImpact, c.Guidance, c.IsActive, now))
		if err != nil {
			return nameConflict(err)
		}
		return audit.Record(ctx, tx, audit.ResourceMaintenanceTemplate, id, activationAction(cur.IsActive, out.IsActive), audit.Diff(cur, out), now)
	})
	return out, err
}

func activationAction(before, after bool) string {
	switch {
	case before && !after:
		return "deactivated"
	case !before && after:
		return "reactivated"
	default:
		return "updated"
	}
}

// ---- HTTP ----

// Handler exposes admin template endpoints.
type Handler struct {
	svc  *Service
	pool *pgxpool.Pool
}

// NewHandler creates a Handler.
func NewHandler(svc *Service, pool *pgxpool.Pool) *Handler { return &Handler{svc: svc, pool: pool} }

// Register mounts admin routes.
func (h *Handler) Register(rt *httpx.Router, admin func(http.HandlerFunc) http.HandlerFunc) {
	rt.Handle(http.MethodGet, "/api/v1/admin/incident-templates", admin(h.listIncident))
	rt.Handle(http.MethodPost, "/api/v1/admin/incident-templates", admin(h.createIncident))
	rt.Handle(http.MethodGet, "/api/v1/admin/incident-templates/{id}", admin(h.getIncident))
	rt.Handle(http.MethodPatch, "/api/v1/admin/incident-templates/{id}", admin(h.patchIncident))
	rt.Handle(http.MethodGet, "/api/v1/admin/maintenance-templates", admin(h.listMaintenance))
	rt.Handle(http.MethodPost, "/api/v1/admin/maintenance-templates", admin(h.createMaintenance))
	rt.Handle(http.MethodGet, "/api/v1/admin/maintenance-templates/{id}", admin(h.getMaintenance))
	rt.Handle(http.MethodPatch, "/api/v1/admin/maintenance-templates/{id}", admin(h.patchMaintenance))
}

type listParams struct {
	active *bool
	sort   httpx.Sort
	page   httpx.Page
}

func parseList(r *http.Request) (listParams, error) {
	q := httpx.NewQuery(r.URL.Query())
	p := listParams{page: q.Page(), sort: q.Sort([]string{"name", "created_at", "updated_at"}, httpx.Sort{Field: "name"}), active: q.Bool("is_active")}
	return p, q.Err()
}

func listRows[T any](r *http.Request, pool *pgxpool.Pool, table, cols string, p listParams, scan func(pgx.Row) (T, error)) ([]T, int, error) {
	ctx := r.Context()
	var total int
	where := " WHERE ($1::boolean IS NULL OR is_active = $1)"
	// #nosec G202 -- table is a package constant chosen by the handler, never user input.
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table+where, p.active).Scan(&total); err != nil {
		return nil, 0, err
	}
	// #nosec G202 -- table and cols are package constants; ORDER BY comes from the sortColumns whitelist.
	rows, err := pool.Query(ctx, "SELECT "+cols+" FROM "+table+where+" ORDER BY "+orderBy(p.sort)+" LIMIT $2 OFFSET $3", p.active, p.page.Limit, p.page.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		t, err := scan(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, t)
	}
	return out, total, rows.Err()
}

func (h *Handler) listIncident(w http.ResponseWriter, r *http.Request) {
	p, err := parseList(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, total, err := listRows(r, h.pool, "incident_templates", incidentColumns, p, scanIncident)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewList(items, p.page, total))
}

func (h *Handler) listMaintenance(w http.ResponseWriter, r *http.Request) {
	p, err := parseList(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, total, err := listRows(r, h.pool, "maintenance_templates", maintenanceColumns, p, scanMaintenance)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewList(items, p.page, total))
}

func (h *Handler) getIncident(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "incident_template")
	if err == nil {
		var t IncidentTemplate
		if t, err = getIncident(r.Context(), h.pool, id, false); err == nil {
			httpx.WriteJSON(w, http.StatusOK, t)
			return
		}
	}
	httpx.Fail(w, r, err)
}

func (h *Handler) getMaintenance(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "maintenance_template")
	if err == nil {
		var t MaintenanceTemplate
		if t, err = getMaintenance(r.Context(), h.pool, id, false); err == nil {
			httpx.WriteJSON(w, http.StatusOK, t)
			return
		}
	}
	httpx.Fail(w, r, err)
}

func (h *Handler) createIncident(w http.ResponseWriter, r *http.Request) {
	var in IncidentInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, err := h.svc.CreateIncident(r.Context(), in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/admin/incident-templates/"+t.ID)
	httpx.WriteJSON(w, http.StatusCreated, t)
}

func (h *Handler) createMaintenance(w http.ResponseWriter, r *http.Request) {
	var in MaintenanceInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, err := h.svc.CreateMaintenance(r.Context(), in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/admin/maintenance-templates/"+t.ID)
	httpx.WriteJSON(w, http.StatusCreated, t)
}

func (h *Handler) patchIncident(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "incident_template")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in IncidentInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, err := h.svc.PatchIncident(r.Context(), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, t)
}

func (h *Handler) patchMaintenance(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "maintenance_template")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in MaintenanceInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, err := h.svc.PatchMaintenance(r.Context(), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, t)
}
