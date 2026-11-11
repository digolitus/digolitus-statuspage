package branding

import (
	"io"
	"net/http"
	"strings"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
)

// Handler exposes the branding over HTTP.
type Handler struct{ svc *Service }

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterPublic registers the visitor routes.
func (h *Handler) RegisterPublic(rt *httpx.Router) {
	rt.Handle(http.MethodGet, "/api/v1/branding", h.public)
	rt.Handle(http.MethodGet, "/api/v1/branding/images/{kind}", h.image)
}

// RegisterAdmin registers the admin routes behind admin.
func (h *Handler) RegisterAdmin(rt *httpx.Router, admin func(http.HandlerFunc) http.HandlerFunc) {
	rt.Handle(http.MethodGet, "/api/v1/admin/branding", admin(h.get))
	rt.Handle(http.MethodPut, "/api/v1/admin/branding", admin(h.update))
	rt.Handle(http.MethodPut, "/api/v1/admin/branding/images/{kind}", admin(h.putImage))
	rt.Handle(http.MethodDelete, "/api/v1/admin/branding/images/{kind}", admin(h.deleteImage))
}

func pathKind(r *http.Request) (string, error) {
	k := r.PathValue("kind")
	if k != KindLogo && k != KindFavicon {
		return "", apperror.NotFound("branding_image")
	}
	return k, nil
}

func (h *Handler) public(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Public(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Short shared caching: every page load reads it.
	w.Header().Set("Cache-Control", "public, max-age=30")
	httpx.WriteJSON(w, http.StatusOK, p)
}

func (h *Handler) image(w http.ResponseWriter, r *http.Request) {
	kind, err := pathKind(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	img, err := h.svc.Image(r.Context(), kind)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	hdr := w.Header()
	etag := `"` + img.SHA256 + `"`
	hdr.Set("ETag", etag)
	// Links carry ?v=<hash prefix>: a matching version never changes.
	if v := r.URL.Query().Get("v"); v != "" && strings.HasPrefix(img.SHA256, v) && len(v) >= 16 {
		hdr.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		hdr.Set("Cache-Control", "public, max-age=300")
	}
	// Opened directly, the file must not run anything.
	hdr.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	hdr.Set("Content-Type", img.ContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(img.Data)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	a, err := h.svc.Admin(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var in Settings
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	a, err := h.svc.Update(r.Context(), in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a)
}

func (h *Handler) putImage(w http.ResponseWriter, r *http.Request) {
	kind, err := pathKind(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/") && !strings.HasPrefix(ct, "application/octet-stream") {
		httpx.Fail(w, r, &apperror.Error{Kind: apperror.KindInvalid, Code: "unsupported_media_type", Message: "Envie o arquivo da imagem no corpo (Content-Type image/*)."})
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, int64(MaxImageBytes[kind])+1))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	a, err := h.svc.PutImage(r.Context(), kind, data)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a)
}

func (h *Handler) deleteImage(w http.ResponseWriter, r *http.Request) {
	kind, err := pathKind(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	a, err := h.svc.DeleteImage(r.Context(), kind)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a)
}
