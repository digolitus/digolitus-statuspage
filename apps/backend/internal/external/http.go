package external

import (
	"net/http"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
)

// Handler exposes admin endpoints for third-party sources.
type Handler struct{ svc *Service }

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register mounts admin routes.
func (h *Handler) Register(rt *httpx.Router, admin func(http.HandlerFunc) http.HandlerFunc) {
	rt.Handle(http.MethodGet, "/api/v1/admin/external-sources", admin(h.list))
	rt.Handle(http.MethodPost, "/api/v1/admin/external-sources", admin(h.create))
	rt.Handle(http.MethodGet, "/api/v1/admin/external-sources/{id}", admin(h.get))
	rt.Handle(http.MethodPatch, "/api/v1/admin/external-sources/{id}", admin(h.patch))
	rt.Handle(http.MethodDelete, "/api/v1/admin/external-sources/{id}", admin(h.delete))
	rt.Handle(http.MethodPost, "/api/v1/admin/external-sources/{id}/sync", admin(h.sync))
	rt.Handle(http.MethodGet, "/api/v1/admin/external-sources/{id}/components", admin(h.components))
	rt.Handle(http.MethodPut, "/api/v1/admin/external-sources/{id}/selection", admin(h.selection))
}

type sourceList struct {
	Data []Source `json:"data"`
}

type componentList struct {
	Data []Component `json:"data"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context(), h.svc.pool, false)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if items == nil {
		items = []Source{}
	}
	httpx.WriteJSON(w, http.StatusOK, sourceList{Data: items})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	src, err := h.svc.Create(r.Context(), in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/admin/external-sources/"+src.ID)
	httpx.WriteJSON(w, http.StatusCreated, src)
}

func (h *Handler) withID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, err := httpx.PathID(r, "external_source")
	if err != nil {
		httpx.Fail(w, r, err)
		return "", false
	}
	return id, true
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := h.withID(w, r)
	if !ok {
		return
	}
	src, err := h.svc.Get(r.Context(), h.svc.pool, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, src)
}

func (h *Handler) patch(w http.ResponseWriter, r *http.Request) {
	id, ok := h.withID(w, r)
	if !ok {
		return
	}
	var in PatchInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	src, err := h.svc.Patch(r.Context(), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, src)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := h.withID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) sync(w http.ResponseWriter, r *http.Request) {
	id, ok := h.withID(w, r)
	if !ok {
		return
	}
	src, err := h.svc.Sync(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, src)
}

func (h *Handler) components(w http.ResponseWriter, r *http.Request) {
	id, ok := h.withID(w, r)
	if !ok {
		return
	}
	if _, err := h.svc.Get(r.Context(), h.svc.pool, id); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, err := Components(r.Context(), h.svc.pool, id, false)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, componentList{Data: items})
}

func (h *Handler) selection(w http.ResponseWriter, r *http.Request) {
	id, ok := h.withID(w, r)
	if !ok {
		return
	}
	var in SelectionInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, err := h.svc.SetSelection(r.Context(), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, componentList{Data: items})
}
