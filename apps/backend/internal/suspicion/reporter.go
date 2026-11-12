package suspicion

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/auth/social"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/subscription"
)

// Reporter sessions are separate from admin sessions: another cookie, another
// table, and no path to the admin API.
const (
	CookieName = "dsp_reporter"
	sessionTTL = 30 * 24 * time.Hour
	linkTTL    = 15 * time.Minute
	// At most linkLimit sign-in links per address within linkWindow.
	linkLimit  = 3
	linkWindow = 15 * time.Minute
)

// Reporter is the signed-in visitor.
type Reporter struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type reporterKey struct{}

func withReporter(ctx context.Context, r *Reporter) context.Context {
	return context.WithValue(ctx, reporterKey{}, r)
}

func reporterFrom(ctx context.Context) *Reporter {
	r, _ := ctx.Value(reporterKey{}).(*Reporter)
	return r
}

func newToken() (string, []byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, fmt.Errorf("random token: %w", err)
	}
	t := base64.RawURLEncoding.EncodeToString(b)
	return t, hashToken(t), nil
}

func hashToken(t string) []byte {
	sum := sha256.Sum256([]byte(t))
	return sum[:]
}

func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

func validEmail(e string) bool {
	if len(e) < 3 || len(e) > 254 || strings.ContainsAny(e, " \t\r\n") {
		return false
	}
	a, err := mail.ParseAddress(e)
	return err == nil && a.Address == e
}

func notSignedIn() error {
	return &apperror.Error{Kind: apperror.KindUnauthenticated, Code: "reporter_unauthenticated", Message: "Entre para continuar."}
}

func blocked() error {
	return &apperror.Error{Kind: apperror.KindForbidden, Code: "reporter_blocked", Message: "Sua conta foi bloqueada para reportar suspeitas."}
}

// authenticate resolves the reporter of a session token.
func (s *Service) authenticate(ctx context.Context, token string) (*Reporter, error) {
	if token == "" {
		return nil, notSignedIn()
	}
	var r Reporter
	var isBlocked bool
	err := s.pool.QueryRow(ctx, `SELECT r.id, r.name, r.email, r.is_blocked FROM reporter_sessions s
		JOIN reporters r ON r.id = s.reporter_id WHERE s.token_hash = $1 AND s.expires_at > $2`,
		hashToken(token), s.now()).Scan(&r.ID, &r.Name, &r.Email, &isBlocked)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, notSignedIn()
	}
	if err != nil {
		return nil, fmt.Errorf("authenticate reporter: %w", err)
	}
	if isBlocked {
		return nil, blocked()
	}
	return &r, nil
}

// session is a new reporter session.
type session struct {
	Token     string
	ExpiresAt time.Time
}

func (s *Service) createSession(ctx context.Context, tx pgx.Tx, reporterID string, now time.Time) (session, error) {
	token, hash, err := newToken()
	if err != nil {
		return session{}, err
	}
	exp := now.Add(sessionTTL)
	if _, err := tx.Exec(ctx, `INSERT INTO reporter_sessions (token_hash, reporter_id, created_at, expires_at) VALUES ($1, $2, $3, $4)`,
		hash, reporterID, now, exp); err != nil {
		return session{}, fmt.Errorf("create reporter session: %w", err)
	}
	if _, err := tx.Exec(ctx, "UPDATE reporters SET last_login_at = $2 WHERE id = $1", reporterID, now); err != nil {
		return session{}, fmt.Errorf("touch reporter: %w", err)
	}
	return session{Token: token, ExpiresAt: exp}, nil
}

// LinkInput asks for a sign-in link.
type LinkInput struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// SendLink e-mails a single-use sign-in link. The answer is the same whether
// or not the address has an account (no enumeration).
func (s *Service) SendLink(ctx context.Context, in LinkInput) error {
	if s.mailer == nil || s.baseURL == "" {
		return &apperror.Error{Kind: apperror.KindUnavailable, Code: "email_unavailable", Message: "O envio de e-mails não está configurado: entre com o Google."}
	}
	var v apperror.Validator
	email := normalizeEmail(in.Email)
	name := strings.Join(strings.Fields(in.Name), " ")
	v.Check(validEmail(email), "email", "Informe um e-mail válido.")
	v.Check(len([]rune(name)) <= 120, "name", "Use até 120 caracteres.")
	if err := v.Err(); err != nil {
		return err
	}
	now := s.now()
	token, hash, err := newToken()
	if err != nil {
		return err
	}
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		// Serialise requests for the same address so the limit holds.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('reporter_link:' || $1))", email); err != nil {
			return fmt.Errorf("lock link address: %w", err)
		}
		var recent int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM reporter_login_tokens WHERE lower(email) = $1 AND created_at > $2",
			email, now.Add(-linkWindow)).Scan(&recent); err != nil {
			return fmt.Errorf("count links: %w", err)
		}
		if recent >= linkLimit {
			return &apperror.Error{Kind: apperror.KindTooMany, Code: "too_many_links", Message: "Muitos links pedidos para este e-mail. Aguarde alguns minutos."}
		}
		if _, err := tx.Exec(ctx, "DELETE FROM reporter_login_tokens WHERE expires_at < $1", now.Add(-24*time.Hour)); err != nil {
			return fmt.Errorf("purge links: %w", err)
		}
		_, err := tx.Exec(ctx, `INSERT INTO reporter_login_tokens (token_hash, email, name, created_at, expires_at) VALUES ($1, $2, $3, $4, $5)`,
			hash, email, name, now, now.Add(linkTTL))
		return err
	})
	if err != nil {
		return err
	}
	site := s.siteName()
	link := s.baseURL + "/suspeitas/entrar?token=" + token
	m := subscription.Mail{To: email, Subject: "Seu link de acesso · " + site,
		Text: fmt.Sprintf("Use o link abaixo para entrar e reportar suspeitas em %s. Ele vale por 15 minutos e só pode ser usado uma vez.\n\n%s\n\nSe você não pediu este acesso, ignore este e-mail.\n", site, link)}
	sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := s.mailer.Send(sendCtx, m); err != nil {
		return &apperror.Error{Kind: apperror.KindUnavailable, Code: "email_failed", Message: "Não foi possível enviar o e-mail agora. Tente de novo em instantes."}
	}
	return nil
}

func invalidLink() error {
	return &apperror.Error{Kind: apperror.KindInvalid, Code: "invalid_link", Message: "Link inválido ou expirado. Peça um novo."}
}

// RedeemLink exchanges a sign-in link for a session, creating the account
// on first use.
func (s *Service) RedeemLink(ctx context.Context, token string) (session, Reporter, error) {
	if token == "" || len(token) > 100 {
		return session{}, Reporter{}, invalidLink()
	}
	now := s.now()
	var sess session
	var rep Reporter
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var email, name string
		err := tx.QueryRow(ctx, `UPDATE reporter_login_tokens SET used_at = $2
			WHERE token_hash = $1 AND used_at IS NULL AND expires_at > $2 RETURNING email, name`, hashToken(token), now).Scan(&email, &name)
		if errors.Is(err, pgx.ErrNoRows) {
			return invalidLink()
		}
		if err != nil {
			return fmt.Errorf("redeem link: %w", err)
		}
		rep, err = upsertReporter(ctx, tx, email, name, "", now)
		if err != nil {
			return err
		}
		sess, err = s.createSession(ctx, tx, rep.ID, now)
		return err
	})
	return sess, rep, err
}

// defaultName derives a display name from the address when none was given.
func defaultName(email string) string {
	local, _, _ := strings.Cut(email, "@")
	if local == "" {
		return "Visitante"
	}
	if r := []rune(local); len(r) > 120 {
		return string(r[:120])
	}
	return local
}

// upsertReporter finds the account of email (or googleSubject) or creates
// it. Blocked accounts are refused.
func upsertReporter(ctx context.Context, tx pgx.Tx, email, name, googleSubject string, now time.Time) (Reporter, error) {
	var r Reporter
	var isBlocked bool
	var err error
	if googleSubject != "" {
		err = tx.QueryRow(ctx, "SELECT id, name, email, is_blocked FROM reporters WHERE google_subject = $1 FOR UPDATE", googleSubject).
			Scan(&r.ID, &r.Name, &r.Email, &isBlocked)
		if err == nil {
			if isBlocked {
				return Reporter{}, blocked()
			}
			return r, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Reporter{}, fmt.Errorf("find reporter: %w", err)
		}
	}
	err = tx.QueryRow(ctx, "SELECT id, name, email, is_blocked FROM reporters WHERE lower(email) = $1 FOR UPDATE", email).
		Scan(&r.ID, &r.Name, &r.Email, &isBlocked)
	switch {
	case err == nil:
		if isBlocked {
			return Reporter{}, blocked()
		}
		if googleSubject != "" {
			if _, err := tx.Exec(ctx, "UPDATE reporters SET google_subject = $2 WHERE id = $1 AND google_subject IS NULL", r.ID, googleSubject); err != nil {
				return Reporter{}, fmt.Errorf("link google: %w", err)
			}
		}
		return r, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return Reporter{}, fmt.Errorf("find reporter: %w", err)
	}
	if name == "" {
		name = defaultName(email)
	}
	var subject *string
	if googleSubject != "" {
		subject = &googleSubject
	}
	err = tx.QueryRow(ctx, `INSERT INTO reporters (email, name, google_subject, created_at) VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING RETURNING id, name, email`, email, name, subject, now).Scan(&r.ID, &r.Name, &r.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		return Reporter{}, apperror.Conflict("reporter_conflict", "Tente entrar de novo.")
	}
	if err != nil {
		return Reporter{}, fmt.Errorf("create reporter: %w", err)
	}
	return r, nil
}

// GoogleLogin opens a session for a Google identity with a verified e-mail.
func (s *Service) GoogleLogin(ctx context.Context, id social.Identity, name string) (session, error) {
	email := normalizeEmail(id.Email)
	if !id.EmailVerified || !validEmail(email) || id.Subject == "" {
		return session{}, invalidLink()
	}
	now := s.now()
	var sess session
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		rep, err := upsertReporter(ctx, tx, email, name, id.Subject, now)
		if err != nil {
			return err
		}
		sess, err = s.createSession(ctx, tx, rep.ID, now)
		return err
	})
	return sess, err
}

// Logout ends the session of token.
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if _, err := s.pool.Exec(ctx, "DELETE FROM reporter_sessions WHERE token_hash = $1", hashToken(token)); err != nil {
		return fmt.Errorf("logout reporter: %w", err)
	}
	return nil
}

func (h *Handler) setCookie(w http.ResponseWriter, sess session) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: sess.Token, Path: "/", Expires: sess.ExpiresAt,
		MaxAge: int(sessionTTL.Seconds()), HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteStrictMode})
}

func (h *Handler) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteStrictMode})
}
