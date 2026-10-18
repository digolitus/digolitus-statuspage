// Package auth implements local authentication (e-mail + password, server
// side sessions) and role-based authorization for the admin area.
//
// Rules worth knowing:
//   - The first administrator is created through a one-time setup token
//     generated at startup while no user exists (see EnsureSetupToken).
//   - Permissions are a fixed catalogue; roles grant subsets; the system role
//     grants everything. A user's permissions are the union of their roles,
//     reloaded on every request, so changes apply immediately.
//   - No privilege escalation: an actor can only grant, edit or manage
//     permissions and users whose permissions they hold themselves.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/audit"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/clock"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
)

// Login throttling per account.
const (
	maxFailedLogins = 5
	lockDuration    = 15 * time.Minute
	// touchEvery limits session last-seen writes to one per interval.
	touchEvery = time.Minute
)

// Config configures sessions.
type Config struct {
	SessionTTL   time.Duration
	IdleTimeout  time.Duration
	CookieSecure bool
}

// Service implements authentication and user/role management.
type Service struct {
	pool *pgxpool.Pool
	now  clock.Clock
	cfg  Config
}

// NewService builds the service.
func NewService(pool *pgxpool.Pool, now clock.Clock, cfg Config) *Service {
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = 12 * time.Hour
	}
	if cfg.IdleTimeout == 0 || cfg.IdleTimeout > cfg.SessionTTL {
		cfg.IdleTimeout = min(2*time.Hour, cfg.SessionTTL)
	}
	return &Service{pool: pool, now: now, cfg: cfg}
}

// --- principal ---

// Principal is the authenticated user of a request.
type Principal struct {
	UserID    string
	Name      string
	Email     string
	Admin     bool
	SessionID string
	// MustChangePassword blocks the admin API until the user picks their
	// own password (set after an administrator chose it).
	MustChangePassword bool
	perms              map[string]bool
}

// Has reports whether the principal holds perm.
func (p *Principal) Has(perm string) bool {
	return p != nil && (p.Admin || p.perms[perm])
}

// Permissions lists the principal's permissions in catalogue order.
func (p *Principal) Permissions() []string {
	out := []string{}
	for _, c := range Catalogue {
		if p.Has(c.Key) {
			out = append(out, c.Key)
		}
	}
	return out
}

// covers reports whether p holds every permission in perms (admin holds all).
func (p *Principal) covers(admin bool, perms []string) bool {
	if p.Admin {
		return true
	}
	if admin {
		return false
	}
	for _, x := range perms {
		if !p.perms[x] {
			return false
		}
	}
	return true
}

type principalKey struct{}

// WithPrincipal attaches p to ctx (also as the audit actor).
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	ctx = context.WithValue(ctx, principalKey{}, p)
	return httpx.WithActor(ctx, httpx.Actor{ID: p.UserID, Name: p.Name + " <" + p.Email + ">"})
}

// FromContext returns the authenticated principal, if any.
func FromContext(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey{}).(*Principal)
	return p
}

// Can reports whether the request's principal holds perm. Without a
// principal it is false (fail closed).
func Can(ctx context.Context, perm string) bool {
	return FromContext(ctx).Has(perm)
}

// --- tokens ---

func newToken() (string, []byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
	}
	t := base64.RawURLEncoding.EncodeToString(b)
	return t, hashToken(t), nil
}

func hashToken(t string) []byte {
	h := sha256.Sum256([]byte(t))
	return h[:]
}

// --- users and roles model ---

// RoleRef is a role embedded in a user.
type RoleRef struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	IsSystem bool   `json:"is_system"`
}

// User is a user as exposed by the API (never with its password hash).
type User struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	IsActive bool   `json:"is_active"`
	// MustChangePassword: the password was set by an administrator and
	// must be replaced at the next sign-in.
	MustChangePassword bool       `json:"must_change_password"`
	Roles              []RoleRef  `json:"roles"`
	LastLoginAt        *time.Time `json:"last_login_at"`
	LockedUntil        *time.Time `json:"locked_until"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// Role groups permissions.
type Role struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Permissions []string  `json:"permissions"`
	IsSystem    bool      `json:"is_system"`
	UserCount   int       `json:"user_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Me is the current session's user and effective permissions.
type Me struct {
	User        User     `json:"user"`
	Permissions []string `json:"permissions"`
	IsAdmin     bool     `json:"is_admin"`
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

const userColumns = `u.id, u.email, u.name, u.is_active, u.must_change_password, u.last_login_at, u.locked_until, u.created_at, u.updated_at,
	COALESCE((SELECT json_agg(json_build_object('id', r.id, 'name', r.name, 'is_system', r.is_system) ORDER BY r.is_system DESC, lower(r.name))
		FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = u.id), '[]')`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.IsActive, &u.MustChangePassword, &u.LastLoginAt, &u.LockedUntil, &u.CreatedAt, &u.UpdatedAt, &u.Roles)
	u.LastLoginAt, u.LockedUntil = utcPtr(u.LastLoginAt), utcPtr(u.LockedUntil)
	u.CreatedAt, u.UpdatedAt = u.CreatedAt.UTC(), u.UpdatedAt.UTC()
	return u, err
}

func getUser(ctx context.Context, q database.Querier, id string, lock bool) (User, error) {
	sql := "SELECT " + userColumns + " FROM users u WHERE u.id = $1"
	if lock {
		sql += " FOR UPDATE OF u"
	}
	u, err := scanUser(q.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, apperror.NotFound("user")
	}
	if err != nil {
		return User{}, fmt.Errorf("get user: %w", err)
	}
	return u, nil
}

const roleColumns = `id, name, description, permissions, is_system,
	(SELECT count(*) FROM user_roles ur WHERE ur.role_id = roles.id)::int, created_at, updated_at`

func scanRole(row pgx.Row) (Role, error) {
	var r Role
	err := row.Scan(&r.ID, &r.Name, &r.Description, &r.Permissions, &r.IsSystem, &r.UserCount, &r.CreatedAt, &r.UpdatedAt)
	if r.IsSystem {
		r.Permissions = allPermissions()
	}
	if r.Permissions == nil {
		r.Permissions = []string{}
	}
	r.CreatedAt, r.UpdatedAt = r.CreatedAt.UTC(), r.UpdatedAt.UTC()
	return r, err
}

func allPermissions() []string {
	out := make([]string, len(Catalogue))
	for i, c := range Catalogue {
		out[i] = c.Key
	}
	return out
}

func getRole(ctx context.Context, q database.Querier, id string, lock bool) (Role, error) {
	sql := "SELECT " + roleColumns + " FROM roles WHERE id = $1"
	if lock {
		sql += " FOR UPDATE"
	}
	r, err := scanRole(q.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Role{}, apperror.NotFound("role")
	}
	if err != nil {
		return Role{}, fmt.Errorf("get role: %w", err)
	}
	return r, nil
}

func listRoles(ctx context.Context, q database.Querier) ([]Role, error) {
	rows, err := q.Query(ctx, "SELECT "+roleColumns+" FROM roles ORDER BY is_system DESC, lower(name), id")
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Role, error) { return scanRole(r) })
}

// userGrants returns whether a user holds the system role and the union of
// permissions of their other roles.
func userGrants(ctx context.Context, q database.Querier, userID string) (bool, []string, error) {
	var admin bool
	var perms []string
	err := q.QueryRow(ctx, `SELECT COALESCE(bool_or(r.is_system), false),
		COALESCE(array_agg(DISTINCT p) FILTER (WHERE p IS NOT NULL), '{}')
		FROM user_roles ur JOIN roles r ON r.id = ur.role_id LEFT JOIN LATERAL unnest(r.permissions) p ON true
		WHERE ur.user_id = $1`, userID).Scan(&admin, &perms)
	if err != nil {
		return false, nil, fmt.Errorf("load grants: %w", err)
	}
	return admin, perms, nil
}

// --- authentication ---

// Session is a newly created session; Token goes in the cookie.
type Session struct {
	Token     string
	ExpiresAt time.Time
}

// Authenticate resolves a session token to a principal, enforcing absolute
// and idle expiry and the user's active flag.
func (s *Service) Authenticate(ctx context.Context, token string) (*Principal, error) {
	if token == "" || len(token) > 100 {
		return nil, apperror.Unauthenticated()
	}
	now := s.now()
	var p Principal
	var active bool
	var lastSeen, expires time.Time
	err := s.pool.QueryRow(ctx, `SELECT s.id, s.last_seen_at, s.expires_at, u.id, u.name, u.email, u.is_active, u.must_change_password
		FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token_hash = $1`, hashToken(token)).
		Scan(&p.SessionID, &lastSeen, &expires, &p.UserID, &p.Name, &p.Email, &active, &p.MustChangePassword)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperror.Unauthenticated()
	}
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}
	if !active || !now.Before(expires) || now.Sub(lastSeen) > s.cfg.IdleTimeout {
		_, _ = s.pool.Exec(ctx, "DELETE FROM sessions WHERE id = $1", p.SessionID)
		return nil, apperror.Unauthenticated()
	}
	admin, perms, err := userGrants(ctx, s.pool, p.UserID)
	if err != nil {
		return nil, err
	}
	p.Admin = admin
	p.perms = make(map[string]bool, len(perms))
	for _, x := range perms {
		p.perms[x] = true
	}
	if now.Sub(lastSeen) >= touchEvery {
		if _, err := s.pool.Exec(ctx, "UPDATE sessions SET last_seen_at = $2 WHERE id = $1", p.SessionID, now); err != nil {
			return nil, fmt.Errorf("touch session: %w", err)
		}
	}
	return &p, nil
}

func (s *Service) createSession(ctx context.Context, tx pgx.Tx, userID, userAgent string, now time.Time) (Session, error) {
	token, hash, err := newToken()
	if err != nil {
		return Session{}, err
	}
	if utf8.RuneCountInString(userAgent) > 300 {
		userAgent = string([]rune(userAgent)[:300])
	}
	exp := now.Add(s.cfg.SessionTTL)
	if _, err := tx.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, created_at, last_seen_at, expires_at, user_agent)
		VALUES ($1, $2, $3, $3, $4, $5)`, hash, userID, now, exp, userAgent); err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	return Session{Token: token, ExpiresAt: exp}, nil
}

func invalidCredentials() error {
	return &apperror.Error{Kind: apperror.KindUnauthenticated, Code: "invalid_credentials", Message: "E-mail ou senha inválidos."}
}

func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

// Login verifies credentials and opens a session. Failures are
// indistinguishable to the client (unknown e-mail, wrong password, inactive
// or locked account); the reason is logged.
func (s *Service) Login(ctx context.Context, email, password, userAgent string) (Session, Me, error) {
	email = normalizeEmail(email)
	now := s.now()
	if email == "" || password == "" || utf8.RuneCountInString(password) > MaxPasswordLen {
		return Session{}, Me{}, invalidCredentials()
	}
	var id, hash string
	var active bool
	var lockedUntil *time.Time
	err := s.pool.QueryRow(ctx, "SELECT id, password_hash, is_active, locked_until FROM users WHERE lower(email) = $1", email).
		Scan(&id, &hash, &active, &lockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		_, _, _ = VerifyPassword(dummyHash(), password)
		slog.WarnContext(ctx, "login failed", "reason", "unknown_email", "request_id", httpx.RequestID(ctx))
		return Session{}, Me{}, invalidCredentials()
	}
	if err != nil {
		return Session{}, Me{}, fmt.Errorf("load user: %w", err)
	}
	ok, rehash, err := VerifyPassword(hash, password)
	if err != nil {
		return Session{}, Me{}, fmt.Errorf("verify password: %w", err)
	}
	locked := lockedUntil != nil && now.Before(*lockedUntil)
	if !ok || !active || locked {
		reason := "wrong_password"
		switch {
		case locked:
			reason = "locked"
		case !active:
			reason = "inactive"
		}
		if !ok && !locked {
			// Count the failure (restarting after an expired lock); lock
			// the account after maxFailedLogins in a row.
			if _, err := s.pool.Exec(ctx, `UPDATE users u SET failed_logins = f.n,
				locked_until = CASE WHEN f.n >= $2 THEN $3::timestamptz ELSE NULL END
				FROM (SELECT CASE WHEN locked_until IS NOT NULL THEN 1 ELSE failed_logins + 1 END AS n FROM users WHERE id = $1) f
				WHERE u.id = $1`, id, maxFailedLogins, now.Add(lockDuration)); err != nil {
				slog.ErrorContext(ctx, "record failed login", "error", err)
			}
		}
		slog.WarnContext(ctx, "login failed", "reason", reason, "user_id", id, "request_id", httpx.RequestID(ctx))
		return Session{}, Me{}, invalidCredentials()
	}
	var sess Session
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if rehash {
			if h, err := HashPassword(password); err == nil {
				if _, err := tx.Exec(ctx, "UPDATE users SET password_hash = $2 WHERE id = $1", id, h); err != nil {
					return err
				}
			}
		}
		if _, err := tx.Exec(ctx, "UPDATE users SET failed_logins = 0, locked_until = NULL, last_login_at = $2 WHERE id = $1", id, now); err != nil {
			return fmt.Errorf("record login: %w", err)
		}
		if _, err := tx.Exec(ctx, "DELETE FROM sessions WHERE expires_at < $1", now); err != nil {
			return fmt.Errorf("purge sessions: %w", err)
		}
		var err error
		sess, err = s.createSession(ctx, tx, id, userAgent, now)
		return err
	})
	if err != nil {
		return Session{}, Me{}, err
	}
	me, err := s.me(ctx, id)
	slog.InfoContext(ctx, "login", "user_id", id, "request_id", httpx.RequestID(ctx))
	return sess, me, err
}

// Logout ends a session.
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, "DELETE FROM sessions WHERE token_hash = $1", hashToken(token))
	return err
}

func (s *Service) me(ctx context.Context, userID string) (Me, error) {
	u, err := getUser(ctx, s.pool, userID, false)
	if err != nil {
		return Me{}, err
	}
	admin, perms, err := userGrants(ctx, s.pool, userID)
	if err != nil {
		return Me{}, err
	}
	p := Principal{Admin: admin, perms: map[string]bool{}}
	for _, x := range perms {
		p.perms[x] = true
	}
	return Me{User: u, Permissions: p.Permissions(), IsAdmin: admin}, nil
}

// Me returns the principal's user and permissions.
func (s *Service) Me(ctx context.Context, p *Principal) (Me, error) {
	return s.me(ctx, p.UserID)
}

// ChangePassword changes the principal's own password after checking the
// current one, and ends their other sessions.
func (s *Service) ChangePassword(ctx context.Context, p *Principal, current, next string) error {
	var hash string
	if err := s.pool.QueryRow(ctx, "SELECT password_hash FROM users WHERE id = $1", p.UserID).Scan(&hash); err != nil {
		return fmt.Errorf("load user: %w", err)
	}
	if ok, _, err := VerifyPassword(hash, current); err != nil || !ok {
		return &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
			Fields: []apperror.FieldError{{Field: "current_password", Message: "Senha atual incorreta."}}}
	}
	var v apperror.Validator
	CheckPasswordPolicy(&v, "new_password", next, p.Email)
	if current == next {
		v.Add("new_password", "A nova senha deve ser diferente da atual.")
	}
	if err := v.Err(); err != nil {
		return err
	}
	h, err := HashPassword(next)
	if err != nil {
		return err
	}
	now := s.now()
	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE users SET password_hash = $2, password_changed_at = $3, updated_at = $3,
			must_change_password = false WHERE id = $1`, p.UserID, h, now); err != nil {
			return fmt.Errorf("change password: %w", err)
		}
		if _, err := tx.Exec(ctx, "DELETE FROM sessions WHERE user_id = $1 AND id <> $2", p.UserID, p.SessionID); err != nil {
			return fmt.Errorf("end sessions: %w", err)
		}
		return audit.Record(ctx, tx, audit.ResourceUser, p.UserID, "password_changed", audit.Changes{}, now)
	})
}

// --- first setup ---

// setupLock serialises first-admin creation across processes.
const setupLock = 0x64737075 // "dspu"

// SetupRequired reports whether no user exists yet.
func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users)").Scan(&exists)
	return !exists, err
}

// EnsureSetupToken creates a new one-time setup token when no user exists
// and returns it (empty when setup is already done). Earlier tokens are
// discarded, so only the latest printed token works.
func (s *Service) EnsureSetupToken(ctx context.Context) (string, error) {
	var token string
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", setupLock); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users)").Scan(&exists); err != nil || exists {
			return err
		}
		t, hash, err := newToken()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM setup_tokens"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO setup_tokens (token_hash, created_at) VALUES ($1, $2)", hash, s.now()); err != nil {
			return err
		}
		token = t
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("setup token: %w", err)
	}
	return token, nil
}

// SetupInput creates the first administrator.
type SetupInput struct {
	SetupToken string `json:"setup_token"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	Password   string `json:"password"`
}

// Setup creates the first administrator (system role) and signs them in.
func (s *Service) Setup(ctx context.Context, in SetupInput, userAgent string) (Session, Me, error) {
	var v apperror.Validator
	name, email := strings.TrimSpace(in.Name), normalizeEmail(in.Email)
	checkName(&v, name)
	checkEmail(&v, email)
	CheckPasswordPolicy(&v, "password", in.Password, email)
	v.Check(strings.TrimSpace(in.SetupToken) != "", "setup_token", "Informe o token de configuração exibido no log do backend.")
	if err := v.Err(); err != nil {
		return Session{}, Me{}, err
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		return Session{}, Me{}, err
	}
	now := s.now()
	var sess Session
	var userID string
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", setupLock); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users)").Scan(&exists); err != nil {
			return err
		}
		if exists {
			return apperror.Conflict("setup_already_done", "A configuração inicial já foi concluída. Faça login.")
		}
		tag, err := tx.Exec(ctx, "DELETE FROM setup_tokens WHERE token_hash = $1", hashToken(strings.TrimSpace(in.SetupToken)))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
				Fields: []apperror.FieldError{{Field: "setup_token", Message: "Token inválido. Use o token mais recente exibido no log do backend."}}}
		}
		if _, err := tx.Exec(ctx, "DELETE FROM setup_tokens"); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO users (email, name, password_hash, password_changed_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $4, $4) RETURNING id`, email, name, hash, now).Scan(&userID); err != nil {
			return fmt.Errorf("create admin: %w", err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE is_system", userID); err != nil {
			return fmt.Errorf("grant admin: %w", err)
		}
		actx := httpx.WithActor(ctx, httpx.Actor{ID: userID, Name: name + " <" + email + ">"})
		if err := audit.Record(actx, tx, audit.ResourceUser, userID, "setup_admin_created", audit.Changes{After: map[string]any{"email": email, "name": name}}, now); err != nil {
			return err
		}
		sess, err = s.createSession(ctx, tx, userID, userAgent, now)
		return err
	})
	if err != nil {
		return Session{}, Me{}, err
	}
	slog.InfoContext(ctx, "initial setup completed: first administrator created", "user_id", userID)
	me, err := s.me(ctx, userID)
	return sess, me, err
}

// --- validation helpers ---

func checkName(v *apperror.Validator, name string) {
	n := utf8.RuneCountInString(name)
	v.Check(n >= 1 && n <= 120, "name", "Informe um nome com 1 a 120 caracteres.")
}

func checkEmail(v *apperror.Validator, email string) {
	at := strings.IndexByte(email, '@')
	ok := len(email) >= 3 && len(email) <= 254 && at > 0 && at == strings.LastIndexByte(email, '@') && at < len(email)-1 &&
		!strings.ContainsAny(email, " \t\r\n")
	v.Check(ok, "email", "Informe um e-mail válido.")
}

func uniqueIDs(v *apperror.Validator, field string, ids []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, id := range ids {
		id = strings.ToLower(strings.TrimSpace(id))
		if !httpx.IsUUID(id) {
			v.Add(field, "Contém um identificador inválido.")
			return nil
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	v.Check(len(out) <= 20, field, "Informe no máximo 20 papéis.")
	return out
}

// sortedPerms validates and orders permissions by the catalogue.
func sortedPerms(v *apperror.Validator, perms []string) []string {
	set := map[string]bool{}
	for _, p := range perms {
		if !Known(p) {
			v.Add("permissions", "Permissão desconhecida: "+p+".")
			return nil
		}
		set[p] = true
	}
	out := []string{}
	for _, c := range Catalogue {
		if set[c.Key] {
			out = append(out, c.Key)
		}
	}
	return out
}

func forbiddenEscalation() error {
	return apperror.Forbidden("Você só pode conceder ou administrar permissões que você mesmo possui.")
}

// mayManageUser: the actor must hold every permission the target holds.
func (s *Service) mayManageUser(ctx context.Context, q database.Querier, actor *Principal, targetID string) error {
	admin, perms, err := userGrants(ctx, q, targetID)
	if err != nil {
		return err
	}
	if !actor.covers(admin, perms) {
		return apperror.Forbidden("Este usuário tem permissões que você não possui; peça a um administrador.")
	}
	return nil
}

// resolveRoles loads roles for assignment and checks the actor may grant them.
func resolveRoles(ctx context.Context, q database.Querier, actor *Principal, ids []string) ([]Role, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := q.Query(ctx, "SELECT "+roleColumns+" FROM roles WHERE id = ANY($1::uuid[])", ids)
	if err != nil {
		return nil, fmt.Errorf("resolve roles: %w", err)
	}
	roles, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Role, error) { return scanRole(r) })
	if err != nil {
		return nil, err
	}
	if len(roles) != len(ids) {
		return nil, &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
			Fields: []apperror.FieldError{{Field: "role_ids", Message: "Um ou mais papéis não existem."}}}
	}
	for _, r := range roles {
		if !actor.covers(r.IsSystem, r.Permissions) {
			return nil, forbiddenEscalation()
		}
	}
	return roles, nil
}

func setUserRoles(ctx context.Context, tx pgx.Tx, userID string, ids []string) error {
	if _, err := tx.Exec(ctx, "DELETE FROM user_roles WHERE user_id = $1", userID); err != nil {
		return fmt.Errorf("clear roles: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, "INSERT INTO user_roles (user_id, role_id) SELECT $1, unnest($2::uuid[])", userID, ids)
	if err != nil {
		return fmt.Errorf("set roles: %w", err)
	}
	return nil
}

// ensureAnotherAdmin fails when removing admin rights from userID would
// leave no active administrator.
func ensureAnotherAdmin(ctx context.Context, tx pgx.Tx, userID string) error {
	var n int
	err := tx.QueryRow(ctx, `SELECT count(DISTINCT u.id) FROM users u JOIN user_roles ur ON ur.user_id = u.id
		JOIN roles r ON r.id = ur.role_id WHERE r.is_system AND u.is_active AND u.id <> $1`, userID).Scan(&n)
	if err != nil {
		return fmt.Errorf("count admins: %w", err)
	}
	if n == 0 {
		return apperror.Conflict("last_admin", "Este é o último administrador ativo. Conceda o papel Administrador a outro usuário antes.")
	}
	return nil
}

func emailConflict(err error) error {
	if name, ok := database.ConstraintError(err, database.CodeUniqueViolation); ok && name == "users_email_key" {
		return &apperror.Error{Kind: apperror.KindConflict, Code: "user_email_conflict", Message: "Já existe um usuário com este e-mail.",
			Fields: []apperror.FieldError{{Field: "email", Message: "E-mail já cadastrado."}}}
	}
	return err
}

// --- user management ---

// UserCreate creates a user with an initial password.
type UserCreate struct {
	Email    string   `json:"email"`
	Name     string   `json:"name"`
	Password string   `json:"password"`
	RoleIDs  []string `json:"role_ids"`
}

// UserPatch edits a user; nil fields are unchanged.
type UserPatch struct {
	Name     *string   `json:"name"`
	Email    *string   `json:"email"`
	IsActive *bool     `json:"is_active"`
	RoleIDs  *[]string `json:"role_ids"`
	// Unlock clears a temporary lock caused by failed logins.
	Unlock *bool `json:"unlock"`
}

func roleIDs(rs []RoleRef) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.ID
	}
	return out
}

func hasSystem(rs []RoleRef) bool {
	return slices.ContainsFunc(rs, func(r RoleRef) bool { return r.IsSystem })
}

// ListUsers returns all users (bounded: local accounts of one organisation).
func (s *Service) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx, "SELECT "+userColumns+" FROM users u ORDER BY u.is_active DESC, lower(u.name), u.id LIMIT 500")
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (User, error) { return scanUser(r) })
}

// CreateUser creates a user with roles the actor may grant.
func (s *Service) CreateUser(ctx context.Context, actor *Principal, in UserCreate) (User, error) {
	var v apperror.Validator
	name, email := strings.TrimSpace(in.Name), normalizeEmail(in.Email)
	checkName(&v, name)
	checkEmail(&v, email)
	CheckPasswordPolicy(&v, "password", in.Password, email)
	ids := uniqueIDs(&v, "role_ids", in.RoleIDs)
	if err := v.Err(); err != nil {
		return User{}, err
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		return User{}, err
	}
	now := s.now()
	var created User
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := resolveRoles(ctx, tx, actor, ids); err != nil {
			return err
		}
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO users (email, name, password_hash, must_change_password, password_changed_at, created_at, updated_at)
			VALUES ($1, $2, $3, true, $4, $4, $4) RETURNING id`, email, name, hash, now).Scan(&id); err != nil {
			return emailConflict(err)
		}
		if err := setUserRoles(ctx, tx, id, ids); err != nil {
			return err
		}
		if created, err = getUser(ctx, tx, id, false); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.ResourceUser, id, "created", audit.Changes{After: created}, now)
	})
	return created, err
}

// GetUser returns one user.
func (s *Service) GetUser(ctx context.Context, id string) (User, error) {
	return getUser(ctx, s.pool, id, false)
}

// UpdateUser edits a user the actor may manage.
func (s *Service) UpdateUser(ctx context.Context, actor *Principal, id string, in UserPatch) (User, error) {
	now := s.now()
	var updated User
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := getUser(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if err := s.mayManageUser(ctx, tx, actor, id); err != nil {
			return err
		}
		var v apperror.Validator
		next := current
		changed := false
		if in.Name != nil {
			next.Name = strings.TrimSpace(*in.Name)
			checkName(&v, next.Name)
			changed = true
		}
		if in.Email != nil {
			next.Email = normalizeEmail(*in.Email)
			checkEmail(&v, next.Email)
			changed = true
		}
		if in.IsActive != nil {
			next.IsActive = *in.IsActive
			changed = true
		}
		unlock := in.Unlock != nil && *in.Unlock
		changed = changed || unlock || in.RoleIDs != nil
		var ids []string
		if in.RoleIDs != nil {
			ids = uniqueIDs(&v, "role_ids", *in.RoleIDs)
		}
		if !changed {
			return apperror.Invalid("empty_update", "Informe ao menos um campo para atualizar.")
		}
		if err := v.Err(); err != nil {
			return err
		}
		if id == actor.UserID && !next.IsActive {
			return apperror.Conflict("cannot_deactivate_self", "Você não pode desativar a própria conta.")
		}
		willBeAdmin := hasSystem(current.Roles)
		if in.RoleIDs != nil {
			roles, err := resolveRoles(ctx, tx, actor, ids)
			if err != nil {
				return err
			}
			willBeAdmin = slices.ContainsFunc(roles, func(r Role) bool { return r.IsSystem })
		}
		if hasSystem(current.Roles) && current.IsActive && (!willBeAdmin || !next.IsActive) {
			if err := ensureAnotherAdmin(ctx, tx, id); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET name = $2, email = $3, is_active = $4, updated_at = $5,
			failed_logins = CASE WHEN $6 THEN 0 ELSE failed_logins END,
			locked_until = CASE WHEN $6 THEN NULL ELSE locked_until END WHERE id = $1`,
			id, next.Name, next.Email, next.IsActive, now, unlock); err != nil {
			return emailConflict(err)
		}
		if in.RoleIDs != nil {
			if err := setUserRoles(ctx, tx, id, ids); err != nil {
				return err
			}
		}
		if !next.IsActive {
			if _, err := tx.Exec(ctx, "DELETE FROM sessions WHERE user_id = $1", id); err != nil {
				return fmt.Errorf("end sessions: %w", err)
			}
		}
		if updated, err = getUser(ctx, tx, id, false); err != nil {
			return err
		}
		action := "updated"
		switch {
		case current.IsActive && !updated.IsActive:
			action = "deactivated"
		case !current.IsActive && updated.IsActive:
			action = "reactivated"
		case in.RoleIDs != nil && !slices.Equal(roleIDs(current.Roles), roleIDs(updated.Roles)):
			action = "roles_changed"
		}
		return audit.Record(ctx, tx, audit.ResourceUser, id, action, audit.Diff(current, updated), now)
	})
	return updated, err
}

// ResetPassword sets a new password chosen by an administrator and ends the
// user's sessions.
func (s *Service) ResetPassword(ctx context.Context, actor *Principal, id, password string) error {
	now := s.now()
	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		u, err := getUser(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if err := s.mayManageUser(ctx, tx, actor, id); err != nil {
			return err
		}
		var v apperror.Validator
		CheckPasswordPolicy(&v, "password", password, u.Email)
		if err := v.Err(); err != nil {
			return err
		}
		h, err := HashPassword(password)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET password_hash = $2, password_changed_at = $3, updated_at = $3,
			failed_logins = 0, locked_until = NULL, must_change_password = true WHERE id = $1`, id, h, now); err != nil {
			return fmt.Errorf("reset password: %w", err)
		}
		if _, err := tx.Exec(ctx, "DELETE FROM sessions WHERE user_id = $1", id); err != nil {
			return fmt.Errorf("end sessions: %w", err)
		}
		return audit.Record(ctx, tx, audit.ResourceUser, id, "password_reset", audit.Changes{}, now)
	})
}

// RecoverAccess is the server-side way back in when nobody can sign in
// (e.g. the only administrator forgot the password). It sets a random
// temporary password, unlocks and reactivates the account, ends its
// sessions and returns the password; the user must replace it on sign-in.
// It requires shell access to the server, never the HTTP API.
func (s *Service) RecoverAccess(ctx context.Context, email string) (string, error) {
	email = normalizeEmail(email)
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate password: %w", err)
	}
	password := base64.RawURLEncoding.EncodeToString(b)
	h, err := HashPassword(password)
	if err != nil {
		return "", err
	}
	now := s.now()
	ctx = httpx.ContextWithSource(ctx, httpx.SourceCLI)
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var id string
		err := tx.QueryRow(ctx, `UPDATE users SET password_hash = $2, must_change_password = true, failed_logins = 0,
			locked_until = NULL, is_active = true, password_changed_at = $3, updated_at = $3
			WHERE lower(email) = $1 RETURNING id`, email, h, now).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("no user with e-mail %q", email)
		}
		if err != nil {
			return fmt.Errorf("recover access: %w", err)
		}
		if _, err := tx.Exec(ctx, "DELETE FROM sessions WHERE user_id = $1", id); err != nil {
			return fmt.Errorf("end sessions: %w", err)
		}
		return audit.Record(ctx, tx, audit.ResourceUser, id, "access_recovered", audit.Changes{}, now)
	})
	if err != nil {
		return "", err
	}
	return password, nil
}

// --- role management ---

// RoleInput creates (Name required) or patches a role.
type RoleInput struct {
	Name        *string   `json:"name"`
	Description *string   `json:"description"`
	Permissions *[]string `json:"permissions"`
}

func (in RoleInput) apply(r Role, create bool) (Role, error) {
	var v apperror.Validator
	changed := false
	if in.Name != nil {
		r.Name = strings.TrimSpace(*in.Name)
		changed = true
	}
	if in.Name != nil || create {
		n := utf8.RuneCountInString(r.Name)
		v.Check(n >= 1 && n <= 60, "name", "Informe um nome com 1 a 60 caracteres.")
	}
	if in.Description != nil {
		r.Description = strings.TrimSpace(*in.Description)
		v.Check(utf8.RuneCountInString(r.Description) <= 500, "description", "A descrição deve ter no máximo 500 caracteres.")
		changed = true
	}
	if in.Permissions != nil {
		r.Permissions = sortedPerms(&v, *in.Permissions)
		changed = true
	}
	if !create && !changed {
		return r, apperror.Invalid("empty_update", "Informe ao menos um campo para atualizar.")
	}
	return r, v.Err()
}

func roleConflict(err error) error {
	if name, ok := database.ConstraintError(err, database.CodeUniqueViolation); ok && name == "roles_name_key" {
		return &apperror.Error{Kind: apperror.KindConflict, Code: "role_name_conflict", Message: "Já existe um papel com este nome.",
			Fields: []apperror.FieldError{{Field: "name", Message: "Nome já utilizado."}}}
	}
	return err
}

// ListRoles returns all roles.
func (s *Service) ListRoles(ctx context.Context) ([]Role, error) { return listRoles(ctx, s.pool) }

// GetRole returns one role.
func (s *Service) GetRole(ctx context.Context, id string) (Role, error) {
	return getRole(ctx, s.pool, id, false)
}

// CreateRole creates a role with permissions the actor holds.
func (s *Service) CreateRole(ctx context.Context, actor *Principal, in RoleInput) (Role, error) {
	r, err := in.apply(Role{Permissions: []string{}}, true)
	if err != nil {
		return Role{}, err
	}
	if !actor.covers(false, r.Permissions) {
		return Role{}, forbiddenEscalation()
	}
	now := s.now()
	var created Role
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM roles").Scan(&n); err != nil {
			return err
		}
		if n >= 100 {
			return apperror.Conflict("role_limit", "Limite de 100 papéis atingido.")
		}
		created, err = scanRole(tx.QueryRow(ctx, `INSERT INTO roles (name, description, permissions, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $4) RETURNING `+roleColumns, r.Name, r.Description, r.Permissions, now))
		if err != nil {
			return roleConflict(err)
		}
		return audit.Record(ctx, tx, audit.ResourceRole, created.ID, "created", audit.Changes{After: created}, now)
	})
	return created, err
}

// UpdateRole edits a role. The system role is immutable; the actor must
// hold both the current and the new permissions.
func (s *Service) UpdateRole(ctx context.Context, actor *Principal, id string, in RoleInput) (Role, error) {
	now := s.now()
	var updated Role
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := getRole(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if current.IsSystem {
			return apperror.Conflict("system_role_immutable", "O papel Administrador é fixo e sempre tem acesso total.")
		}
		next, err := in.apply(current, false)
		if err != nil {
			return err
		}
		if !actor.covers(false, current.Permissions) || !actor.covers(false, next.Permissions) {
			return forbiddenEscalation()
		}
		updated, err = scanRole(tx.QueryRow(ctx, `UPDATE roles SET name = $2, description = $3, permissions = $4, updated_at = $5
			WHERE id = $1 RETURNING `+roleColumns, id, next.Name, next.Description, next.Permissions, now))
		if err != nil {
			return roleConflict(err)
		}
		return audit.Record(ctx, tx, audit.ResourceRole, id, "updated", audit.Diff(current, updated), now)
	})
	return updated, err
}

// DeleteRole removes a role no user holds.
func (s *Service) DeleteRole(ctx context.Context, actor *Principal, id string) error {
	now := s.now()
	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := getRole(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if current.IsSystem {
			return apperror.Conflict("system_role_immutable", "O papel Administrador não pode ser excluído.")
		}
		if !actor.covers(false, current.Permissions) {
			return forbiddenEscalation()
		}
		if current.UserCount > 0 {
			return apperror.Conflict("role_in_use", fmt.Sprintf("O papel está atribuído a %d usuário(s). Remova-o dos usuários antes.", current.UserCount))
		}
		if _, err := tx.Exec(ctx, "DELETE FROM roles WHERE id = $1", id); err != nil {
			if _, ok := database.ConstraintError(err, database.CodeForeignKeyViolation); ok {
				return apperror.Conflict("role_in_use", "O papel ainda está atribuído a usuários.")
			}
			return fmt.Errorf("delete role: %w", err)
		}
		return audit.Record(ctx, tx, audit.ResourceRole, id, "deleted", audit.Changes{Before: current}, now)
	})
}
