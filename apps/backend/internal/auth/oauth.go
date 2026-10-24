package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/auth/social"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
)

// oauthCookie keeps state, nonce and PKCE verifier between the redirect to
// the provider and its callback.
const (
	oauthCookie     = "dsp_oauth"
	oauthCookiePath = "/api/v1/auth/oauth/"
	oauthMaxAge     = 600
	// loginPage receives ?login_error=<code> when social sign-in fails.
	loginPage = "/admin"
)

// errSocialNotAllowed: the provider authenticated the person, but no active
// user matches. Users must be registered by an administrator first.
var errSocialNotAllowed = errors.New("social sign-in not allowed")

func (s *Service) provider(id string) social.Provider {
	for _, p := range s.cfg.Providers {
		if p.ID() == id {
			return p
		}
	}
	return nil
}

func (s *Service) redirectURI(p social.Provider) string {
	return s.cfg.BaseURL + oauthCookiePath + p.ID() + "/callback"
}

// SocialLogin opens a session for the user linked to a provider identity.
// An identity not linked yet is linked when the provider vouches for an
// e-mail equal to an existing user's. Inactive users are refused; password
// lockouts do not apply, since no password is involved.
func (s *Service) SocialLogin(ctx context.Context, provider string, id social.Identity, userAgent string) (Session, error) {
	now := s.now()
	var sess Session
	var userID, reason string
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var active bool
		err := tx.QueryRow(ctx, `SELECT u.id, u.is_active FROM user_identities i JOIN users u ON u.id = i.user_id
			WHERE i.provider = $1 AND i.subject = $2`, provider, id.Subject).Scan(&userID, &active)
		linked := err == nil
		if errors.Is(err, pgx.ErrNoRows) {
			email := normalizeEmail(id.Email)
			if email == "" || !id.EmailVerified {
				reason = "unverified_email"
				return errSocialNotAllowed
			}
			err = tx.QueryRow(ctx, "SELECT id, is_active FROM users WHERE lower(email) = $1", email).Scan(&userID, &active)
			if errors.Is(err, pgx.ErrNoRows) {
				reason = "unknown_email"
				return errSocialNotAllowed
			}
		}
		if err != nil {
			return fmt.Errorf("find user: %w", err)
		}
		if !active {
			reason = "inactive"
			return errSocialNotAllowed
		}
		if linked {
			_, err = tx.Exec(ctx, "UPDATE user_identities SET last_used_at = $3 WHERE provider = $1 AND subject = $2", provider, id.Subject, now)
		} else {
			_, err = tx.Exec(ctx, `INSERT INTO user_identities (provider, subject, user_id, email, created_at, last_used_at)
				VALUES ($1, $2, $3, $4, $5, $5) ON CONFLICT (provider, subject) DO NOTHING`,
				provider, id.Subject, userID, normalizeEmail(id.Email), now)
		}
		if err != nil {
			return fmt.Errorf("record identity: %w", err)
		}
		if _, err := tx.Exec(ctx, "UPDATE users SET last_login_at = $2 WHERE id = $1", userID, now); err != nil {
			return fmt.Errorf("record login: %w", err)
		}
		if _, err := tx.Exec(ctx, "DELETE FROM sessions WHERE expires_at < $1", now); err != nil {
			return fmt.Errorf("purge sessions: %w", err)
		}
		sess, err = s.createSession(ctx, tx, userID, userAgent, now)
		return err
	})
	if errors.Is(err, errSocialNotAllowed) {
		slog.WarnContext(ctx, "social login refused", "provider", provider, "reason", reason, "user_id", userID,
			"request_id", httpx.RequestID(ctx))
		return Session{}, err
	}
	if err != nil {
		return Session{}, err
	}
	slog.InfoContext(ctx, "login", "method", provider, "user_id", userID, "request_id", httpx.RequestID(ctx))
	return sess, nil
}

type providerInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (h *Handler) providers(w http.ResponseWriter, _ *http.Request) {
	out := make([]providerInfo, 0, len(h.svc.cfg.Providers))
	for _, p := range h.svc.cfg.Providers {
		out = append(out, providerInfo{ID: p.ID(), Name: p.Name()})
	}
	httpx.WriteJSON(w, http.StatusOK, listOf(out))
}

func notFoundProvider() error {
	return &apperror.Error{Kind: apperror.KindNotFound, Code: "provider_not_found", Message: "Provedor de login não configurado."}
}

// oauthStart sends the browser to the provider's sign-in page.
func (h *Handler) oauthStart(w http.ResponseWriter, r *http.Request) {
	p := h.svc.provider(r.PathValue("provider"))
	if p == nil {
		httpx.Fail(w, r, notFoundProvider())
		return
	}
	params, err := social.NewParams(h.svc.redirectURI(p))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.svc.setOAuthCookie(w, strings.Join([]string{p.ID(), params.State, params.Nonce, params.Verifier}, "."), oauthMaxAge)
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, p.AuthURL(params), http.StatusFound)
}

// oauthCallback receives the provider's answer (a GET, or a form POST for
// Apple), opens the session and returns to the admin area. Failures go back
// to the login page with a short code; details are only logged.
func (h *Handler) oauthCallback(w http.ResponseWriter, r *http.Request) {
	p := h.svc.provider(r.PathValue("provider"))
	if p == nil {
		httpx.Fail(w, r, notFoundProvider())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	fail := func(code, reason string, err error) {
		slog.WarnContext(r.Context(), "social login failed", "provider", p.ID(), "reason", reason, "error", err,
			"request_id", httpx.RequestID(r.Context()))
		http.Redirect(w, r, loginPage+"?"+url.Values{"login_error": {code}}.Encode(), http.StatusFound)
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		fail("failed", "bad_request", err)
		return
	}
	c, cookieErr := r.Cookie(oauthCookie)
	// Single use, whatever the outcome.
	h.svc.setOAuthCookie(w, "", -1)
	if e := r.Form.Get("error"); e != "" {
		code := "failed"
		if e == "access_denied" || e == "user_cancelled_authorize" {
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
	state := r.Form.Get("state")
	if len(parts) != 4 || parts[0] != p.ID() || state == "" || subtle.ConstantTimeCompare([]byte(parts[1]), []byte(state)) != 1 {
		fail("failed", "state_mismatch", nil)
		return
	}
	code := r.Form.Get("code")
	if code == "" {
		fail("failed", "missing_code", nil)
		return
	}
	params := social.Params{State: parts[1], Nonce: parts[2], Verifier: parts[3], RedirectURI: h.svc.redirectURI(p)}
	id, err := p.Exchange(r.Context(), code, params)
	if err != nil {
		fail("failed", "exchange", err)
		return
	}
	sess, err := h.svc.SocialLogin(r.Context(), p.ID(), id, r.UserAgent())
	if errors.Is(err, errSocialNotAllowed) {
		http.Redirect(w, r, loginPage+"?login_error=not_allowed", http.StatusFound)
		return
	}
	if err != nil {
		fail("failed", "session", err)
		return
	}
	h.svc.setCookie(w, sess)
	http.Redirect(w, r, loginPage, http.StatusFound)
}

// setOAuthCookie writes the flow cookie. Apple posts the callback from its
// own site, so with HTTPS the cookie is SameSite=None (it is single-use and
// only checked against the state parameter); over plain HTTP it falls back
// to Lax, which covers the GET callbacks.
func (s *Service) setOAuthCookie(w http.ResponseWriter, value string, maxAge int) {
	same := http.SameSiteLaxMode
	if s.cfg.CookieSecure {
		same = http.SameSiteNoneMode
	}
	http.SetCookie(w, &http.Cookie{Name: oauthCookie, Value: value, Path: oauthCookiePath, MaxAge: maxAge,
		HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: same})
}
