package auth

import (
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
)

// CookieName is the session cookie. It is HttpOnly, SameSite=Strict and,
// unless disabled for plain-HTTP local use, Secure.
const CookieName = "dsp_session"

// Policy maps a route pattern ("METHOD /path", as set in r.Pattern) to the
// permissions that allow it: any one of them suffices; an empty list means
// any authenticated user. Routes absent from the policy are denied.
type Policy map[string][]string

// Guard authenticates the request, checks the origin of state-changing
// requests and enforces the route's permissions before calling next.
func (s *Service) Guard(policy Policy, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := s.principal(r)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if err := checkOrigin(r); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if p.MustChangePassword {
			httpx.Fail(w, r, &apperror.Error{Kind: apperror.KindForbidden, Code: "password_change_required",
				Message: "Defina uma nova senha para continuar (POST /api/v1/auth/password)."})
			return
		}
		perms, ok := policy[r.Pattern]
		if !ok {
			slog.ErrorContext(r.Context(), "route without authorization policy", "pattern", r.Pattern)
			httpx.Fail(w, r, apperror.Forbidden(""))
			return
		}
		if len(perms) > 0 && !anyOf(p, perms) {
			httpx.Fail(w, r, apperror.Forbidden(""))
			return
		}
		next(w, r.WithContext(WithPrincipal(r.Context(), p)))
	}
}

func anyOf(p *Principal, perms []string) bool {
	for _, x := range perms {
		if p.Has(x) {
			return true
		}
	}
	return false
}

func (s *Service) principal(r *http.Request) (*Principal, error) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return nil, apperror.Unauthenticated()
	}
	return s.Authenticate(r.Context(), c.Value)
}

// checkOrigin rejects cross-site state-changing requests (CSRF defence in
// depth on top of SameSite=Strict cookies and the JSON content type). It
// relies on Sec-Fetch-Site, sent by current browsers, and falls back to
// comparing Origin with Host. Non-browser clients send neither.
func checkOrigin(r *http.Request) error {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return nil
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		if site == "same-origin" || site == "none" {
			return nil
		}
		return &apperror.Error{Kind: apperror.KindForbidden, Code: "cross_site_request", Message: "Requisição de outro site recusada."}
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host != r.Host {
			return &apperror.Error{Kind: apperror.KindForbidden, Code: "cross_site_request", Message: "Requisição de outro site recusada."}
		}
	}
	return nil
}

func (s *Service) setCookie(w http.ResponseWriter, sess Session) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: sess.Token, Path: "/", Expires: sess.ExpiresAt,
		MaxAge: int(time.Until(sess.ExpiresAt).Seconds()), HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteStrictMode,
	})
}

func (s *Service) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteStrictMode})
}

// Handler exposes authentication and user/role management.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterAuth mounts session endpoints (outside the admin guard).
func (h *Handler) RegisterAuth(rt *httpx.Router) {
	rt.Handle(http.MethodGet, "/api/v1/auth/setup", h.setupStatus)
	rt.Handle(http.MethodPost, "/api/v1/auth/setup", h.setup)
	rt.Handle(http.MethodPost, "/api/v1/auth/login", h.login)
	rt.Handle(http.MethodPost, "/api/v1/auth/logout", h.logout)
	rt.Handle(http.MethodGet, "/api/v1/auth/me", h.me)
	rt.Handle(http.MethodPost, "/api/v1/auth/password", h.changePassword)
}

// RegisterAdmin mounts user and role management behind the admin guard.
func (h *Handler) RegisterAdmin(rt *httpx.Router, admin func(http.HandlerFunc) http.HandlerFunc) {
	rt.Handle(http.MethodGet, "/api/v1/admin/permissions", admin(h.permissions))
	rt.Handle(http.MethodGet, "/api/v1/admin/users", admin(h.listUsers))
	rt.Handle(http.MethodPost, "/api/v1/admin/users", admin(h.createUser))
	rt.Handle(http.MethodGet, "/api/v1/admin/users/{id}", admin(h.getUser))
	rt.Handle(http.MethodPatch, "/api/v1/admin/users/{id}", admin(h.updateUser))
	rt.Handle(http.MethodPost, "/api/v1/admin/users/{id}/password", admin(h.resetPassword))
	rt.Handle(http.MethodGet, "/api/v1/admin/roles", admin(h.listRoles))
	rt.Handle(http.MethodPost, "/api/v1/admin/roles", admin(h.createRole))
	rt.Handle(http.MethodGet, "/api/v1/admin/roles/{id}", admin(h.getRole))
	rt.Handle(http.MethodPatch, "/api/v1/admin/roles/{id}", admin(h.updateRole))
	rt.Handle(http.MethodDelete, "/api/v1/admin/roles/{id}", admin(h.deleteRole))
}

type setupStatus struct {
	SetupRequired bool `json:"setup_required"`
}

func (h *Handler) setupStatus(w http.ResponseWriter, r *http.Request) {
	req, err := h.svc.SetupRequired(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, setupStatus{SetupRequired: req})
}

func (h *Handler) setup(w http.ResponseWriter, r *http.Request) {
	if err := checkOrigin(r); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in SetupInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sess, me, err := h.svc.Setup(r.Context(), in, r.UserAgent())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.svc.setCookie(w, sess)
	httpx.WriteJSON(w, http.StatusCreated, me)
}

type loginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if err := checkOrigin(r); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in loginInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sess, me, err := h.svc.Login(r.Context(), in.Email, in.Password, r.UserAgent())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.svc.setCookie(w, sess)
	httpx.WriteJSON(w, http.StatusOK, me)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if err := checkOrigin(r); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if c, err := r.Cookie(CookieName); err == nil {
		if err := h.svc.Logout(r.Context(), c.Value); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	h.svc.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.principal(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	me, err := h.svc.Me(r.Context(), p)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, me)
}

type passwordChange struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.principal(r)
	if err == nil {
		err = checkOrigin(r)
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in passwordChange
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := WithPrincipal(r.Context(), p)
	if err := h.svc.ChangePassword(ctx, p, in.CurrentPassword, in.NewPassword); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type list[T any] struct {
	Data []T `json:"data"`
}

func listOf[T any](xs []T) list[T] {
	if xs == nil {
		xs = []T{}
	}
	return list[T]{Data: xs}
}

func (h *Handler) permissions(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, listOf(Catalogue))
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	us, err := h.svc.ListUsers(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, listOf(us))
}

func (h *Handler) getUser(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "user")
	if err == nil {
		var u User
		if u, err = h.svc.GetUser(r.Context(), id); err == nil {
			httpx.WriteJSON(w, http.StatusOK, u)
			return
		}
	}
	httpx.Fail(w, r, err)
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var in UserCreate
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	u, err := h.svc.CreateUser(r.Context(), FromContext(r.Context()), in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/admin/users/"+u.ID)
	httpx.WriteJSON(w, http.StatusCreated, u)
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "user")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in UserPatch
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	u, err := h.svc.UpdateUser(r.Context(), FromContext(r.Context()), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, u)
}

type passwordReset struct {
	Password string `json:"password"`
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "user")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in passwordReset
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.ResetPassword(r.Context(), FromContext(r.Context()), id, in.Password); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listRoles(w http.ResponseWriter, r *http.Request) {
	rs, err := h.svc.ListRoles(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, listOf(rs))
}

func (h *Handler) getRole(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "role")
	if err == nil {
		var x Role
		if x, err = h.svc.GetRole(r.Context(), id); err == nil {
			httpx.WriteJSON(w, http.StatusOK, x)
			return
		}
	}
	httpx.Fail(w, r, err)
}

func (h *Handler) createRole(w http.ResponseWriter, r *http.Request) {
	var in RoleInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	x, err := h.svc.CreateRole(r.Context(), FromContext(r.Context()), in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/admin/roles/"+x.ID)
	httpx.WriteJSON(w, http.StatusCreated, x)
}

func (h *Handler) updateRole(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "role")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in RoleInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	x, err := h.svc.UpdateRole(r.Context(), FromContext(r.Context()), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, x)
}

func (h *Handler) deleteRole(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "role")
	if err == nil {
		err = h.svc.DeleteRole(r.Context(), FromContext(r.Context()), id)
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
