package monitoring

import (
	"io"
	"net/http"
	"strings"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
)

// WebhookPath receives alerts. The token goes in the query string (tools
// such as UptimeRobot cannot set headers) or as a Bearer token. The backend
// never logs query strings; the reverse proxy must not log this path either.
const WebhookPath = "/api/v1/hooks/monitoring"

// Handler exposes the webhook and the admin endpoints.
type Handler struct{ svc *Service }

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterPublic mounts the webhook (authenticated by its token).
func (h *Handler) RegisterPublic(rt *httpx.Router) {
	rt.Handle(http.MethodPost, WebhookPath, h.webhook)
}

// RegisterAdmin mounts admin routes.
func (h *Handler) RegisterAdmin(rt *httpx.Router, admin func(http.HandlerFunc) http.HandlerFunc) {
	const base = "/api/v1/admin/monitoring-integrations"
	rt.Handle(http.MethodGet, base, admin(h.list))
	rt.Handle(http.MethodPost, base, admin(h.create))
	rt.Handle(http.MethodGet, base+"/{id}", admin(h.get))
	rt.Handle(http.MethodPatch, base+"/{id}", admin(h.patch))
	rt.Handle(http.MethodDelete, base+"/{id}", admin(h.delete))
	rt.Handle(http.MethodPost, base+"/{id}/token", admin(h.rotate))
	rt.Handle(http.MethodGet, base+"/{id}/events", admin(h.events))
	rt.Handle(http.MethodPost, base+"/{id}/monitors", admin(h.addMonitor))
	rt.Handle(http.MethodGet, "/api/v1/admin/monitors", admin(h.monitors))
	rt.Handle(http.MethodPatch, "/api/v1/admin/monitors/{id}", admin(h.patchMonitor))
	rt.Handle(http.MethodDelete, "/api/v1/admin/monitors/{id}", admin(h.deleteMonitor))
}

type accepted struct {
	Status string `json:"status"`
}

func (h *Handler) webhook(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if auth := r.Header.Get("Authorization"); token == "" && strings.HasPrefix(auth, "Bearer ") {
		token = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, httpx.MaxBodyBytes))
	if err != nil {
		httpx.Fail(w, r, &apperror.Error{Kind: apperror.KindInvalid, Code: "payload_too_large", Message: "O corpo da requisição excede o limite permitido."})
		return
	}
	// A body that cannot be read is still authenticated and recorded.
	fields, ferr := Fields(r.URL.Query(), r.Header.Get("Content-Type"), body)
	if err := h.svc.Receive(r.Context(), token, fields, ferr); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, accepted{Status: "accepted"})
}

type integrationList struct {
	Data []Integration `json:"data"`
}

type monitorList struct {
	Data []Monitor `json:"data"`
}

type eventList struct {
	Data []Event `json:"data"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, integrationList{Data: items})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := h.svc.Create(r.Context(), in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/admin/monitoring-integrations/"+out.ID)
	httpx.WriteJSON(w, http.StatusCreated, out)
}

func pathID(w http.ResponseWriter, r *http.Request, resource string) (string, bool) {
	id, err := httpx.PathID(r, resource)
	if err != nil {
		httpx.Fail(w, r, err)
		return "", false
	}
	return id, true
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "monitoring_integration")
	if !ok {
		return
	}
	out, err := h.svc.Get(r.Context(), h.svc.pool, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) patch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "monitoring_integration")
	if !ok {
		return
	}
	var in PatchInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := h.svc.Patch(r.Context(), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "monitoring_integration")
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) rotate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "monitoring_integration")
	if !ok {
		return
	}
	out, err := h.svc.RotateToken(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "monitoring_integration")
	if !ok {
		return
	}
	items, err := h.svc.Events(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, eventList{Data: items})
}

func (h *Handler) addMonitor(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "monitoring_integration")
	if !ok {
		return
	}
	var in MonitorInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := h.svc.AddMonitor(r.Context(), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, out)
}

func (h *Handler) monitors(w http.ResponseWriter, r *http.Request) {
	q := httpx.NewQuery(r.URL.Query())
	f := MonitorFilter{
		IntegrationID: q.UUID("integration_id"),
		ComponentID:   q.UUID("component_id"),
		State:         q.Enum("state", StateUnknown, StateUp, StateDown),
	}
	if err := q.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, err := h.svc.Monitors(r.Context(), f)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, monitorList{Data: items})
}

func (h *Handler) patchMonitor(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "monitor")
	if !ok {
		return
	}
	var in MonitorPatch
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := h.svc.PatchMonitor(r.Context(), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) deleteMonitor(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "monitor")
	if !ok {
		return
	}
	if err := h.svc.DeleteMonitor(r.Context(), id); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
