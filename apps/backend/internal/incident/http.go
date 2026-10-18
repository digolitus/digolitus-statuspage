package incident

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/statuspolicy"
)

// Handler exposes public and admin incident endpoints.
type Handler struct {
	svc  *Service
	pool *pgxpool.Pool
}

// NewHandler creates a Handler.
func NewHandler(svc *Service, pool *pgxpool.Pool) *Handler {
	return &Handler{svc: svc, pool: pool}
}

// RegisterPublic mounts read-only public routes.
func (h *Handler) RegisterPublic(rt *httpx.Router) {
	rt.Handle(http.MethodGet, "/api/v1/incidents", h.publicList)
	rt.Handle(http.MethodGet, "/api/v1/incidents/{id}", h.publicGet)
}

// RegisterAdmin mounts admin routes.
func (h *Handler) RegisterAdmin(rt *httpx.Router, admin func(http.HandlerFunc) http.HandlerFunc) {
	rt.Handle(http.MethodGet, "/api/v1/admin/incidents", admin(h.adminList))
	rt.Handle(http.MethodPost, "/api/v1/admin/incidents", admin(h.create))
	rt.Handle(http.MethodGet, "/api/v1/admin/incidents/{id}", admin(h.adminGet))
	rt.Handle(http.MethodPatch, "/api/v1/admin/incidents/{id}", admin(h.patch))
	rt.Handle(http.MethodGet, "/api/v1/admin/incidents/{id}/updates", admin(h.listUpdates))
	rt.Handle(http.MethodPost, "/api/v1/admin/incidents/{id}/updates", admin(h.postUpdate))
	rt.Handle(http.MethodPost, "/api/v1/admin/incidents/{id}/resolve", admin(h.resolve))
}

func parseList(r *http.Request, admin bool) (Filter, httpx.Sort, httpx.Page, error) {
	q := httpx.NewQuery(r.URL.Query())
	page := q.Page()
	sort := q.Sort([]string{"started_at", "created_at", "updated_at"}, httpx.Sort{Field: "started_at", Desc: true})
	f := Filter{
		State:  q.Enum("state", "active", "resolved"),
		Impact: q.Enum("impact", statuspolicy.Impacts...),
	}
	if admin {
		f.Status = q.Enum("status", Statuses...)
		f.ComponentID = q.UUID("component_id")
	}
	return f, sort, page, q.Err()
}

func (h *Handler) publicList(w http.ResponseWriter, r *http.Request) {
	f, sort, page, err := parseList(r, false)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, total, err := List(r.Context(), h.pool, f, sort, page)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]PublicIncident, len(items))
	for k, i := range items {
		out[k] = i.Public()
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewList(out, page, total))
}

func (h *Handler) publicGet(w http.ResponseWriter, r *http.Request) {
	inc, ok := h.load(w, r)
	if ok {
		httpx.WriteJSON(w, http.StatusOK, inc.Public())
	}
}

func (h *Handler) adminList(w http.ResponseWriter, r *http.Request) {
	f, sort, page, err := parseList(r, true)
	if err != nil {
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

func (h *Handler) adminGet(w http.ResponseWriter, r *http.Request) {
	if inc, ok := h.load(w, r); ok {
		httpx.WriteJSON(w, http.StatusOK, inc)
	}
}

func (h *Handler) load(w http.ResponseWriter, r *http.Request) (Incident, bool) {
	id, err := httpx.PathID(r, "incident")
	if err != nil {
		httpx.Fail(w, r, err)
		return Incident{}, false
	}
	inc, err := Load(r.Context(), h.pool, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return Incident{}, false
	}
	return inc, true
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	inc, err := h.svc.Create(r.Context(), in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/admin/incidents/"+inc.ID)
	httpx.WriteJSON(w, http.StatusCreated, inc)
}

func (h *Handler) patch(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "incident")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in PatchInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	inc, err := h.svc.Patch(r.Context(), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, inc)
}

func (h *Handler) listUpdates(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "incident")
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

func (h *Handler) postUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "incident")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in UpdateInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	inc, err := h.svc.PostUpdate(r.Context(), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, inc)
}

func (h *Handler) resolve(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "incident")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in ResolveInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	inc, err := h.svc.Resolve(r.Context(), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, inc)
}
