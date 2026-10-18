package component

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/audit"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/auth"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/clock"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
)

// Service implements the component use cases.
type Service struct {
	pool *pgxpool.Pool
	now  clock.Clock
}

// NewService creates a Service.
func NewService(pool *pgxpool.Pool, now clock.Clock) *Service {
	return &Service{pool: pool, now: now}
}

// Create validates and stores a new component, recording an audit event.
func (s *Service) Create(ctx context.Context, in CreateInput) (Component, error) {
	c, err := in.normalize()
	if err != nil {
		return Component{}, err
	}
	c.CreatedAt = s.now()
	var created Component
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		if created, err = insert(ctx, tx, c); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.ResourceComponent, created.ID, "created", audit.Changes{After: created}, created.CreatedAt)
	})
	return created, err
}

// Update applies a partial update. Deactivation keeps the component and its
// history; it only removes it from the public page and from new associations.
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (Component, error) {
	var updated Component
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := Get(ctx, tx, id, true)
		if err != nil {
			return err
		}
		next, err := in.apply(current)
		if err != nil {
			return err
		}
		next.UpdatedAt = s.now()
		if updated, err = update(ctx, tx, next); err != nil {
			return err
		}
		action := "updated"
		switch {
		case current.IsActive && !updated.IsActive:
			action = "deactivated"
		case !current.IsActive && updated.IsActive:
			action = "reactivated"
		case current.Status != updated.Status:
			action = "status_changed"
		case !sameGroup(current.GroupID, updated.GroupID):
			action = "group_changed"
		}
		return audit.Record(ctx, tx, audit.ResourceComponent, id, action, audit.Diff(current, updated), updated.UpdatedAt)
	})
	return updated, err
}

// Handler exposes admin component endpoints.
type Handler struct {
	svc  *Service
	pool *pgxpool.Pool
}

// NewHandler creates a Handler.
func NewHandler(svc *Service, pool *pgxpool.Pool) *Handler {
	return &Handler{svc: svc, pool: pool}
}

// Register mounts admin routes.
func (h *Handler) Register(rt *httpx.Router, admin func(http.HandlerFunc) http.HandlerFunc) {
	rt.Handle(http.MethodGet, "/api/v1/admin/components", admin(h.list))
	rt.Handle(http.MethodPost, "/api/v1/admin/components", admin(h.create))
	rt.Handle(http.MethodGet, "/api/v1/admin/components/{id}", admin(h.get))
	rt.Handle(http.MethodPatch, "/api/v1/admin/components/{id}", admin(h.update))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := httpx.NewQuery(r.URL.Query())
	page := q.Page()
	sort := q.Sort([]string{"display_order", "name", "created_at", "updated_at"}, httpx.Sort{Field: "display_order"})
	f := Filter{Active: q.Bool("is_active"), Public: q.Bool("is_public"), GroupID: q.UUID("group_id")}
	if err := q.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, total, err := List(r.Context(), h.pool, f, sort, page)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewList(items, page, total))
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := h.svc.Create(r.Context(), in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/admin/components/"+c.ID)
	httpx.WriteJSON(w, http.StatusCreated, c)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "component")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := Get(r.Context(), h.pool, id, false)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, c)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "component")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in UpdateInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Users with only components.status may change the status, nothing else.
	if !auth.Can(r.Context(), auth.PermComponentsManage) && !in.statusOnly() {
		httpx.Fail(w, r, apperror.Forbidden("Sua permissão permite alterar apenas o status do componente."))
		return
	}
	c, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, c)
}

func sameGroup(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
