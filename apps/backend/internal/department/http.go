package department

import (
	"net/http"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
)

// Handler exposes departments over HTTP.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register mounts the admin routes.
func (h *Handler) Register(rt *httpx.Router, admin func(http.HandlerFunc) http.HandlerFunc) {
	rt.Handle(http.MethodGet, "/api/v1/admin/department-levels", admin(h.getLevels))
	rt.Handle(http.MethodPut, "/api/v1/admin/department-levels", admin(h.setLevels))
	rt.Handle(http.MethodGet, "/api/v1/admin/departments", admin(h.list))
	rt.Handle(http.MethodPost, "/api/v1/admin/departments", admin(h.create))
	rt.Handle(http.MethodGet, "/api/v1/admin/departments/{id}", admin(h.get))
	rt.Handle(http.MethodPatch, "/api/v1/admin/departments/{id}", admin(h.update))
	rt.Handle(http.MethodDelete, "/api/v1/admin/departments/{id}", admin(h.remove))
}

type itemsEnvelope struct {
	Data []Department `json:"data"`
}

func (h *Handler) getLevels(w http.ResponseWriter, r *http.Request) {
	l, err := h.svc.GetLevels(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, l)
}

func (h *Handler) setLevels(w http.ResponseWriter, r *http.Request) {
	var in Levels
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	l, err := h.svc.SetLevels(r.Context(), in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, l)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ds, err := h.svc.List(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, itemsEnvelope{Data: ds})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "department")
	if err == nil {
		var d Department
		if d, err = h.svc.Get(r.Context(), id); err == nil {
			httpx.WriteJSON(w, http.StatusOK, d)
			return
		}
	}
	httpx.Fail(w, r, err)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in Input
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	d, err := h.svc.Create(r.Context(), in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/admin/departments/"+d.ID)
	httpx.WriteJSON(w, http.StatusCreated, d)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "department")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in Input
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	d, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "department")
	if err == nil {
		err = h.svc.Delete(r.Context(), id)
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
