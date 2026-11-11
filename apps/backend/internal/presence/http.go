package presence

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/auth"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/clock"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
)

// Handler exposes presence over HTTP.
type Handler struct{ svc *Service }

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterPublic registers the anonymous heartbeat.
func (h *Handler) RegisterPublic(rt *httpx.Router) {
	rt.Handle(http.MethodPost, "/api/v1/presence", h.public)
}

// RegisterAdmin registers the admin heartbeat and the snapshot.
func (h *Handler) RegisterAdmin(rt *httpx.Router, admin func(http.HandlerFunc) http.HandlerFunc) {
	rt.Handle(http.MethodPost, "/api/v1/admin/presence", admin(h.adminBeat))
	rt.Handle(http.MethodGet, "/api/v1/admin/presence", admin(h.snapshot))
}

func (h *Handler) public(w http.ResponseWriter, r *http.Request) {
	var b PublicBeat
	if err := httpx.DecodeJSON(r, &b); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.Public(r.Context(), b); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) adminBeat(w http.ResponseWriter, r *http.Request) {
	var b AdminBeat
	if err := httpx.DecodeJSON(r, &b); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p := auth.FromContext(r.Context())
	if p == nil {
		httpx.Fail(w, r, apperror.Unauthenticated())
		return
	}
	if err := h.svc.Admin(r.Context(), p.UserID, b); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) snapshot(w http.ResponseWriter, r *http.Request) {
	loc := time.UTC
	if raw := r.URL.Query().Get("timezone"); raw != "" {
		l, err := clock.LoadZone(raw)
		if err != nil {
			httpx.Fail(w, r, &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
				Fields: []apperror.FieldError{{Field: "timezone", Message: "Informe um timezone IANA válido (ex.: America/Sao_Paulo)."}}})
			return
		}
		loc = l
	}
	snap, err := h.svc.Snapshot(r.Context(), loc)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, snap)
}

// Run calls Tick once a minute until ctx ends. Failures are logged and
// retried on the next tick.
func (s *Service) Run(ctx context.Context, log *slog.Logger) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		if err := s.Tick(ctx); err != nil && ctx.Err() == nil {
			log.Warn("presence sampling failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
