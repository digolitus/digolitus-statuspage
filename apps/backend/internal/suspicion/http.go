package suspicion

import (
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/auth"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/auth/social"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/incident"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
)

const (
	oauthCookie     = "dsp_reporter_oauth"
	oauthCookiePath = "/api/v1/reporter/oauth/"
	oauthMaxAge     = 600
	// Pages of the public app the sign-in flows return to.
	afterLogin = "/suspeitas/minhas"
	loginPage  = "/suspeitas/entrar"
)

// Handler exposes the public, reporter and admin routes. Every route is
// behind gate: while the feature is off they answer 404.
type Handler struct {
	svc          *Service
	gate         func(http.HandlerFunc) http.HandlerFunc
	cookieSecure bool
	google       social.Provider
}

// NewHandler builds the handler. google may be nil (sign-in by e-mail only).
func NewHandler(svc *Service, gate func(http.HandlerFunc) http.HandlerFunc, cookieSecure bool, google social.Provider) *Handler {
	return &Handler{svc: svc, gate: gate, cookieSecure: cookieSecure, google: google}
}

// RegisterPublic mounts the public and reporter routes.
func (h *Handler) RegisterPublic(rt *httpx.Router) {
	g := h.gate
	rt.Handle(http.MethodGet, "/api/v1/suspicions", g(h.publicList))
	rt.Handle(http.MethodGet, "/api/v1/reporter/providers", g(h.providers))
	rt.Handle(http.MethodPost, "/api/v1/reporter/login-link", g(h.origin(h.sendLink)))
	rt.Handle(http.MethodPost, "/api/v1/reporter/login", g(h.origin(h.redeem)))
	rt.Handle(http.MethodPost, "/api/v1/reporter/logout", g(h.origin(h.logout)))
	rt.Handle(http.MethodGet, "/api/v1/reporter/me", g(h.signedIn(h.me)))
	rt.Handle(http.MethodGet, "/api/v1/reporter/suspicions", g(h.signedIn(h.mine)))
	rt.Handle(http.MethodPost, "/api/v1/reporter/suspicions", g(h.origin(h.signedIn(h.report))))
	rt.Handle(http.MethodPost, "/api/v1/reporter/suspicions/{id}/messages", g(h.origin(h.signedIn(h.reply))))
	rt.Handle(http.MethodGet, "/api/v1/reporter/oauth/google", g(h.oauthStart))
	rt.Handle(http.MethodGet, "/api/v1/reporter/oauth/google/callback", g(h.oauthCallback))
}

// RegisterAdmin mounts the triage routes.
func (h *Handler) RegisterAdmin(rt *httpx.Router, admin func(http.HandlerFunc) http.HandlerFunc) {
	g := func(f http.HandlerFunc) http.HandlerFunc { return h.gate(admin(f)) }
	rt.Handle(http.MethodGet, "/api/v1/admin/suspicions", g(h.adminList))
	rt.Handle(http.MethodGet, "/api/v1/admin/suspicions/{id}", g(h.adminGet))
	rt.Handle(http.MethodPatch, "/api/v1/admin/suspicions/{id}", g(h.patch))
	rt.Handle(http.MethodPost, "/api/v1/admin/suspicions/{id}/publish", g(h.publish))
	rt.Handle(http.MethodPost, "/api/v1/admin/suspicions/{id}/close", g(h.closeSuspicion))
	rt.Handle(http.MethodPost, "/api/v1/admin/suspicions/{id}/incident", g(h.openIncident))
	rt.Handle(http.MethodPost, "/api/v1/admin/suspicions/{id}/messages", g(h.addMessage))
	rt.Handle(http.MethodPatch, "/api/v1/admin/reporters/{id}", g(h.block))
}

// origin rejects cross-site state-changing requests (as the admin API).
func (h *Handler) origin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := auth.CheckOrigin(r); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		next(w, r)
	}
}

// signedIn requires a reporter session.
func (h *Handler) signedIn(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := ""
		if c, err := r.Cookie(CookieName); err == nil {
			token = c.Value
		}
		rep, err := h.svc.authenticate(r.Context(), token)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		next(w, r.WithContext(withReporter(r.Context(), rep)))
	}
}

type dataList[T any] struct {
	Data []T `json:"data"`
}

func (h *Handler) publicList(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListPublic(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	httpx.WriteJSON(w, http.StatusOK, dataList[PublicSuspicion]{Data: list})
}

// Providers tells the sign-in page which methods are available.
type Providers struct {
	Email  bool `json:"email"`
	Google bool `json:"google"`
}

func (h *Handler) providers(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, Providers{Email: h.svc.EmailEnabled(), Google: h.google != nil && h.svc.baseURL != ""})
}

func (h *Handler) sendLink(w http.ResponseWriter, r *http.Request) {
	var in LinkInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.SendLink(r.Context(), in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// RedeemInput carries the token of a sign-in link.
type RedeemInput struct {
	Token string `json:"token"`
}

func (h *Handler) redeem(w http.ResponseWriter, r *http.Request) {
	var in RedeemInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sess, rep, err := h.svc.RedeemLink(r.Context(), in.Token)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.setCookie(w, sess)
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, rep)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(CookieName); err == nil {
		if err := h.svc.Logout(r.Context(), c.Value); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	h.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, reporterFrom(r.Context()))
}

func (h *Handler) mine(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.Mine(r.Context(), reporterFrom(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, dataList[MySuspicion]{Data: list})
}

func (h *Handler) report(w http.ResponseWriter, r *http.Request) {
	var in ReportInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s, err := h.svc.Report(r.Context(), reporterFrom(r.Context()), in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, s)
}

// BodyInput is a message text.
type BodyInput struct {
	Body string `json:"body"`
}

func (h *Handler) reply(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "suspicion")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in BodyInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s, err := h.svc.Reply(r.Context(), reporterFrom(r.Context()), id, in.Body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, s)
}

// --- Google sign-in for reporters ---

func (h *Handler) redirectURI() string {
	return h.svc.baseURL + oauthCookiePath + "google/callback"
}

func (h *Handler) setOAuthCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: oauthCookie, Value: value, Path: oauthCookiePath, MaxAge: maxAge,
		HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode})
}

func (h *Handler) oauthStart(w http.ResponseWriter, r *http.Request) {
	if h.google == nil || h.svc.baseURL == "" {
		httpx.Fail(w, r, apperror.NotFound("provider"))
		return
	}
	params, err := social.NewParams(h.redirectURI())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.setOAuthCookie(w, strings.Join([]string{params.State, params.Nonce, params.Verifier}, "."), oauthMaxAge)
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, h.google.AuthURL(params), http.StatusFound)
}

func (h *Handler) oauthCallback(w http.ResponseWriter, r *http.Request) {
	if h.google == nil || h.svc.baseURL == "" {
		httpx.Fail(w, r, apperror.NotFound("provider"))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	fail := func(code, reason string, err error) {
		slog.WarnContext(r.Context(), "reporter sign-in failed", "reason", reason, "error", err, "request_id", httpx.RequestID(r.Context()))
		http.Redirect(w, r, loginPage+"?"+url.Values{"login_error": {code}}.Encode(), http.StatusFound)
	}
	q := r.URL.Query()
	c, cookieErr := r.Cookie(oauthCookie)
	h.setOAuthCookie(w, "", -1)
	if e := q.Get("error"); e != "" {
		code := "failed"
		if e == "access_denied" {
			code = "cancelled"
		}
		fail(code, "provider_error", errors.New(e))
		return
	}
	if cookieErr != nil {
		fail("failed", "missing_state_cookie", cookieErr)
		return
	}
	parts := strings.Split(c.Value, ".")
	state := q.Get("state")
	if len(parts) != 3 || state == "" || subtle.ConstantTimeCompare([]byte(parts[0]), []byte(state)) != 1 {
		fail("failed", "state_mismatch", nil)
		return
	}
	code := q.Get("code")
	if code == "" {
		fail("failed", "missing_code", nil)
		return
	}
	id, err := h.google.Exchange(r.Context(), code, social.Params{State: parts[0], Nonce: parts[1], Verifier: parts[2], RedirectURI: h.redirectURI()})
	if err != nil {
		fail("failed", "exchange", err)
		return
	}
	sess, err := h.svc.GoogleLogin(r.Context(), id, "")
	if e, ok := apperror.As(err); ok && e.Code == "reporter_blocked" {
		http.Redirect(w, r, loginPage+"?login_error=blocked", http.StatusFound)
		return
	}
	if err != nil {
		fail("failed", "session", err)
		return
	}
	h.setCookie(w, sess)
	http.Redirect(w, r, afterLogin, http.StatusFound)
}

// --- admin ---

// AdminList is the queue with the open counts for the menu badge.
type AdminList struct {
	httpx.List[AdminSuspicion]
	Counts Counts `json:"counts"`
}

func (h *Handler) adminList(w http.ResponseWriter, r *http.Request) {
	q := httpx.NewQuery(r.URL.Query())
	page := q.Page()
	f := AdminFilter{
		Status: q.Enum("status", append([]string{"open"}, Statuses...)...),
		Queue:  q.Enum("queue", "mine", "general", "all"),
	}
	if err := q.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, total, counts, err := h.svc.List(r.Context(), f, page)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, AdminList{List: httpx.NewList(items, page, total), Counts: counts})
}

func (h *Handler) withID(w http.ResponseWriter, r *http.Request, fn func(id string) (Detail, error), status int) {
	id, err := httpx.PathID(r, "suspicion")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	d, err := fn(id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, status, d)
}

func decodeThen[T any](w http.ResponseWriter, r *http.Request, h *Handler, status int, fn func(id string, in T) (Detail, error)) {
	var in T
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.withID(w, r, func(id string) (Detail, error) { return fn(id, in) }, status)
}

func (h *Handler) adminGet(w http.ResponseWriter, r *http.Request) {
	h.withID(w, r, func(id string) (Detail, error) { return h.svc.Get(r.Context(), id) }, http.StatusOK)
}

func (h *Handler) publish(w http.ResponseWriter, r *http.Request) {
	h.withID(w, r, func(id string) (Detail, error) { return h.svc.Publish(r.Context(), id) }, http.StatusOK)
}

func (h *Handler) patch(w http.ResponseWriter, r *http.Request) {
	decodeThen(w, r, h, http.StatusOK, func(id string, in PatchInput) (Detail, error) { return h.svc.Patch(r.Context(), id, in) })
}

func (h *Handler) closeSuspicion(w http.ResponseWriter, r *http.Request) {
	decodeThen(w, r, h, http.StatusOK, func(id string, in CloseInput) (Detail, error) { return h.svc.Close(r.Context(), id, in) })
}

func (h *Handler) openIncident(w http.ResponseWriter, r *http.Request) {
	decodeThen(w, r, h, http.StatusCreated, func(id string, in incident.CreateInput) (Detail, error) {
		return h.svc.OpenIncident(r.Context(), id, in)
	})
}

func (h *Handler) addMessage(w http.ResponseWriter, r *http.Request) {
	decodeThen(w, r, h, http.StatusCreated, func(id string, in MessageInput) (Detail, error) { return h.svc.AddMessage(r.Context(), id, in) })
}

func (h *Handler) block(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "reporter")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in BlockInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rep, err := h.svc.SetBlocked(r.Context(), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, rep)
}
