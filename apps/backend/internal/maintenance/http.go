package maintenance

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/clock"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
)

// Handler exposes public and admin maintenance endpoints.
type Handler struct {
	svc  *Service
	pool *pgxpool.Pool
	now  clock.Clock
}

// NewHandler creates a Handler.
func NewHandler(svc *Service, pool *pgxpool.Pool, now clock.Clock) *Handler {
	return &Handler{svc: svc, pool: pool, now: now}
}

// RegisterPublic mounts read-only public routes.
func (h *Handler) RegisterPublic(rt *httpx.Router) {
	rt.Handle(http.MethodGet, "/api/v1/maintenances", h.publicList)
	rt.Handle(http.MethodGet, "/api/v1/maintenances/{id}", h.publicGet)
}

// RegisterAdmin mounts admin routes.
func (h *Handler) RegisterAdmin(rt *httpx.Router, admin func(http.HandlerFunc) http.HandlerFunc) {
	rt.Handle(http.MethodGet, "/api/v1/admin/maintenances", admin(h.adminList))
	rt.Handle(http.MethodPost, "/api/v1/admin/maintenances", admin(h.create))
	rt.Handle(http.MethodGet, "/api/v1/admin/maintenances/{id}", admin(h.adminGet))
	rt.Handle(http.MethodPatch, "/api/v1/admin/maintenances/{id}", admin(h.patch))
	rt.Handle(http.MethodGet, "/api/v1/admin/maintenances/{id}/updates", admin(h.listUpdates))
	rt.Handle(http.MethodPost, "/api/v1/admin/maintenances/{id}/updates", admin(h.postMessage))
	rt.Handle(http.MethodPost, "/api/v1/admin/maintenances/{id}/start", admin(h.transition(Start)))
	rt.Handle(http.MethodPost, "/api/v1/admin/maintenances/{id}/complete", admin(h.transition(Complete)))
	rt.Handle(http.MethodPost, "/api/v1/admin/maintenances/{id}/cancel", admin(h.transition(Cancel)))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) ([]Maintenance, httpx.Page, int, bool) {
	q := httpx.NewQuery(r.URL.Query())
	page := q.Page()
	f := Filter{State: q.Enum("state", States...), ComponentID: q.UUID("component_id")}
	sort := q.Sort([]string{"scheduled_start", "created_at", "updated_at"}, DefaultSort(f.State))
	if err := q.Err(); err != nil {
		httpx.Fail(w, r, err)
		return nil, page, 0, false
	}
	items, total, err := List(r.Context(), h.pool, f, sort, page, h.now())
	if err != nil {
		httpx.Fail(w, r, err)
		return nil, page, 0, false
	}
	return items, page, total, true
}

func (h *Handler) publicList(w http.ResponseWriter, r *http.Request) {
	items, page, total, ok := h.list(w, r)
	if !ok {
		return
	}
	out := make([]PublicMaintenance, len(items))
	for k, m := range items {
		out[k] = m.Public()
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewList(out, page, total))
}

func (h *Handler) adminList(w http.ResponseWriter, r *http.Request) {
	if items, page, total, ok := h.list(w, r); ok {
		httpx.WriteJSON(w, http.StatusOK, httpx.NewList(items, page, total))
	}
}

func (h *Handler) load(w http.ResponseWriter, r *http.Request) (Maintenance, bool) {
	id, err := httpx.PathID(r, "maintenance")
	if err != nil {
		httpx.Fail(w, r, err)
		return Maintenance{}, false
	}
	m, err := Load(r.Context(), h.pool, id, h.now())
	if err != nil {
		httpx.Fail(w, r, err)
		return Maintenance{}, false
	}
	return m, true
}

func (h *Handler) publicGet(w http.ResponseWriter, r *http.Request) {
	if m, ok := h.load(w, r); ok {
		httpx.WriteJSON(w, http.StatusOK, m.Public())
	}
}

func (h *Handler) adminGet(w http.ResponseWriter, r *http.Request) {
	if m, ok := h.load(w, r); ok {
		httpx.WriteJSON(w, http.StatusOK, m)
	}
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m, err := h.svc.Create(r.Context(), in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/admin/maintenances/"+m.ID)
	httpx.WriteJSON(w, http.StatusCreated, m)
}

func (h *Handler) patch(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "maintenance")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in PatchInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m, err := h.svc.Patch(r.Context(), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, m)
}

func (h *Handler) listUpdates(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "maintenance")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	q := httpx.NewQuery(r.URL.Query())
	page := q.Page()
	if err := q.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, err := get(r.Context(), h.pool, id, false); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	updates, total, err := listUpdates(r.Context(), h.pool, id, page)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewList(updates, page, total))
}

func (h *Handler) postMessage(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "maintenance")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in MessageInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m, err := h.svc.PostMessage(r.Context(), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, m)
}

func (h *Handler) transition(t Transition) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := httpx.PathID(r, "maintenance")
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		var in MessageInput
		// The body is optional for transitions.
		if r.ContentLength != 0 {
			if err := httpx.DecodeJSON(r, &in); err != nil {
				httpx.Fail(w, r, err)
				return
			}
		}
		m, err := h.svc.Transition(r.Context(), id, t, in)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, m)
	}
}
