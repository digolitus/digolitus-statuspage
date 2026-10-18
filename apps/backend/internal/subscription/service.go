// Package subscription lets people follow the status page: e-mail (double
// opt-in), webhooks (HMAC-signed), Slack incoming webhooks and Atom/RSS
// feeds. Changes record notify events in their own transaction; the Worker
// fans them out to matching subscriptions and delivers with retries.
//
// Public self-service always requires confirmation by e-mail, so it is only
// offered when SMTP is configured. Administrators can create webhook and
// Slack integrations directly.
package subscription

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/audit"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/clock"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
)

// Channels.
const (
	ChannelEmail   = "email"
	ChannelWebhook = "webhook"
	ChannelSlack   = "slack"
)

// Statuses.
const (
	StatusPending  = "pending"
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

// Limits.
const (
	maxComponents      = 50
	resendAfter        = 10 * time.Minute
	pendingPerHour     = 300
	disableAfterFailed = 15
)

// ComponentRef names a followed component.
type ComponentRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Subscription is the admin view.
type Subscription struct {
	ID             string         `json:"id"`
	Channel        string         `json:"channel"`
	Email          *string        `json:"email"`
	TargetURL      *string        `json:"target_url"`
	Status         string         `json:"status"`
	AllComponents  bool           `json:"all_components"`
	Components     []ComponentRef `json:"components"`
	CreatedBy      string         `json:"created_by"`
	FailureCount   int            `json:"failure_count"`
	DisabledReason *string        `json:"disabled_reason"`
	LastDeliveryAt *time.Time     `json:"last_delivery_at"`
	LastError      *string        `json:"last_error"`
	ConfirmedAt    *time.Time     `json:"confirmed_at"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`

	signingSecret      *string
	confirmationSentAt *time.Time
}

// ManageView is what the holder of a manage link sees.
type ManageView struct {
	ID            string         `json:"id"`
	Channel       string         `json:"channel"`
	Email         *string        `json:"email"`
	TargetURL     *string        `json:"target_url"`
	Status        string         `json:"status"`
	AllComponents bool           `json:"all_components"`
	Components    []ComponentRef `json:"components"`
	// SigningSecret verifies webhook signatures (webhook channel only).
	SigningSecret *string   `json:"signing_secret"`
	CreatedAt     time.Time `json:"created_at"`
}

func (s Subscription) manageView() ManageView {
	return ManageView{ID: s.ID, Channel: s.Channel, Email: s.Email, TargetURL: s.TargetURL, Status: s.Status,
		AllComponents: s.AllComponents, Components: s.Components, SigningSecret: s.signingSecret, CreatedAt: s.CreatedAt}
}

// Service implements subscriptions.
type Service struct {
	pool     *pgxpool.Pool
	now      clock.Clock
	mailer   Mailer
	poster   Poster
	baseURL  string
	siteName string
	key      tokenKey
}

// Options configures the service.
type Options struct {
	// Mailer sends e-mail; nil disables e-mail (and public self-service).
	Mailer Mailer
	Poster Poster
	// BaseURL is the public page's absolute URL (links in messages).
	BaseURL  string
	SiteName string
}

// NewService builds the service.
func NewService(pool *pgxpool.Pool, now clock.Clock, o Options) *Service {
	if o.SiteName == "" {
		o.SiteName = "Status"
	}
	return &Service{pool: pool, now: now, mailer: o.Mailer, poster: o.Poster, baseURL: strings.TrimRight(o.BaseURL, "/"), siteName: o.SiteName}
}

// EmailEnabled reports whether e-mail (and so public self-service) works.
func (s *Service) EmailEnabled() bool { return s.mailer != nil && s.baseURL != "" }

func (s *Service) link(ctx context.Context, path, id string, purpose byte) (string, error) {
	key, err := s.key.get(ctx, s.pool)
	if err != nil {
		return "", err
	}
	return s.baseURL + path + "?token=" + sign(key, id, purpose), nil
}

func (s *Service) resolve(ctx context.Context, token string, purpose byte) (string, error) {
	key, err := s.key.get(ctx, s.pool)
	if err != nil {
		return "", err
	}
	id, ok := verify(key, token, purpose)
	if !ok {
		return "", invalidToken()
	}
	return id, nil
}

func invalidToken() error {
	return &apperror.Error{Kind: apperror.KindNotFound, Code: "subscription_not_found",
		Message: "Link inválido ou expirado. A inscrição pode ter sido cancelada; inscreva-se novamente se quiser."}
}

// --- persistence ---

const columns = `s.id, s.channel, s.email, s.target_url, s.signing_secret, s.status, s.all_components, s.created_by,
	s.failure_count, s.disabled_reason, s.last_delivery_at, s.last_error, s.confirmation_sent_at, s.confirmed_at, s.created_at, s.updated_at,
	COALESCE((SELECT json_agg(json_build_object('id', c.id, 'name', c.name) ORDER BY c.display_order, c.name)
		FROM subscription_components sc JOIN components c ON c.id = sc.component_id WHERE sc.subscription_id = s.id), '[]')`

func scan(row pgx.Row) (Subscription, error) {
	var x Subscription
	err := row.Scan(&x.ID, &x.Channel, &x.Email, &x.TargetURL, &x.signingSecret, &x.Status, &x.AllComponents, &x.CreatedBy,
		&x.FailureCount, &x.DisabledReason, &x.LastDeliveryAt, &x.LastError, &x.confirmationSentAt, &x.ConfirmedAt,
		&x.CreatedAt, &x.UpdatedAt, &x.Components)
	return x, err
}

func get(ctx context.Context, q database.Querier, id string, lock bool) (Subscription, error) {
	sql := "SELECT " + columns + " FROM subscriptions s WHERE s.id = $1"
	if lock {
		sql += " FOR UPDATE OF s"
	}
	x, err := scan(q.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Subscription{}, apperror.NotFound("subscription")
	}
	if err != nil {
		return Subscription{}, fmt.Errorf("get subscription: %w", err)
	}
	return x, nil
}

func enqueue(ctx context.Context, q database.Querier, subID, kind string, now time.Time) error {
	_, err := q.Exec(ctx, `INSERT INTO outbox_messages (subscription_id, kind, next_attempt_at, created_at) VALUES ($1, $2, $3, $3)`, subID, kind, now)
	if err != nil {
		return fmt.Errorf("enqueue %s: %w", kind, err)
	}
	return nil
}

func setComponents(ctx context.Context, tx pgx.Tx, id string, ids []string) error {
	if _, err := tx.Exec(ctx, "DELETE FROM subscription_components WHERE subscription_id = $1", id); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, "INSERT INTO subscription_components (subscription_id, component_id) SELECT $1, unnest($2::uuid[])", id, ids)
	return err
}

// --- validation ---

func normalizeEmail(v *apperror.Validator, raw string) string {
	e := strings.ToLower(strings.TrimSpace(raw))
	at := strings.IndexByte(e, '@')
	ok := len(e) >= 3 && len(e) <= 254 && at > 0 && at == strings.LastIndexByte(e, '@') && at < len(e)-1 && !strings.ContainsAny(e, " \t\r\n<>,;")
	v.Check(ok, "email", "Informe um e-mail válido.")
	return e
}

func normalizeTarget(v *apperror.Validator, channel, raw string) string {
	t := strings.TrimSpace(raw)
	u, err := url.Parse(t)
	ok := err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && len(t) <= 500
	if !ok {
		v.Add("url", "Informe um endereço https:// completo, sem credenciais, com até 500 caracteres.")
		return t
	}
	if channel == ChannelSlack && (u.Host != "hooks.slack.com" || !strings.HasPrefix(u.Path, "/services/")) {
		v.Add("url", "Use a URL de um Incoming Webhook do Slack (https://hooks.slack.com/services/...).")
	}
	return t
}

// resolveComponents validates followed components: existing, active and
// public (the public page is what subscribers follow).
func resolveComponents(ctx context.Context, q database.Querier, v *apperror.Validator, ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > maxComponents {
		v.Add("component_ids", fmt.Sprintf("Escolha no máximo %d componentes.", maxComponents))
		return nil
	}
	clean := []string{}
	for _, id := range ids {
		id = strings.ToLower(strings.TrimSpace(id))
		if !httpx.IsUUID(id) {
			v.Add("component_ids", "Contém um identificador inválido.")
			return nil
		}
		if !slices.Contains(clean, id) {
			clean = append(clean, id)
		}
	}
	var n int
	if err := q.QueryRow(ctx, "SELECT count(*) FROM components WHERE id = ANY($1::uuid[]) AND is_active AND is_public", clean).Scan(&n); err != nil || n != len(clean) {
		v.Add("component_ids", "Um ou mais componentes não existem.")
		return nil
	}
	return clean
}

// --- public self-service ---

// SubscribeInput is a public subscription request.
type SubscribeInput struct {
	Channel      string   `json:"channel"`
	Email        string   `json:"email"`
	URL          string   `json:"url"`
	ComponentIDs []string `json:"component_ids"`
}

// Subscribe registers a pending subscription and queues a confirmation
// e-mail. The outcome is never revealed to the caller (no enumeration of
// subscribers): an existing subscription gets a "manage" link e-mailed to its
// owner instead.
func (s *Service) Subscribe(ctx context.Context, in SubscribeInput) error {
	if !s.EmailEnabled() {
		return apperror.Conflict("subscriptions_unavailable", "Inscrições por e-mail, webhook e Slack não estão disponíveis nesta instalação. Use o feed RSS/Atom.")
	}
	var v apperror.Validator
	v.Check(in.Channel == ChannelEmail || in.Channel == ChannelWebhook || in.Channel == ChannelSlack, "channel", "Canal inválido. Valores aceitos: email, webhook, slack.")
	email := normalizeEmail(&v, in.Email)
	var target string
	if in.Channel == ChannelWebhook || in.Channel == ChannelSlack {
		target = normalizeTarget(&v, in.Channel, in.URL)
	}
	comps := resolveComponents(ctx, s.pool, &v, in.ComponentIDs)
	if err := v.Err(); err != nil {
		return err
	}
	now := s.now()
	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var recent int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM subscriptions WHERE status = 'pending' AND created_at > $1", now.Add(-time.Hour)).Scan(&recent); err != nil {
			return err
		}
		var existing Subscription
		var err error
		if in.Channel == ChannelEmail {
			existing, err = scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM subscriptions s WHERE s.channel = 'email' AND lower(s.email) = $1 FOR UPDATE OF s", email))
		} else {
			existing, err = scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM subscriptions s WHERE s.channel = $1 AND s.target_url = $2 FOR UPDATE OF s", in.Channel, target))
		}
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			if recent >= pendingPerHour {
				return &apperror.Error{Kind: apperror.KindUnavailable, Code: "subscriptions_busy", Message: "Muitas inscrições recentes. Tente novamente mais tarde."}
			}
			var secret *string
			if in.Channel == ChannelWebhook {
				x := newSecret()
				secret = &x
			}
			var targetPtr *string
			if target != "" {
				targetPtr = &target
			}
			var id string
			if err := tx.QueryRow(ctx, `INSERT INTO subscriptions (channel, email, target_url, signing_secret, status, all_components, created_by,
				confirmation_sent_at, created_at, updated_at) VALUES ($1, $2, $3, $4, 'pending', $5, 'public', $6, $6, $6) RETURNING id`,
				in.Channel, email, targetPtr, secret, len(comps) == 0, now).Scan(&id); err != nil {
				return fmt.Errorf("create subscription: %w", err)
			}
			if err := setComponents(ctx, tx, id, comps); err != nil {
				return err
			}
			return enqueue(ctx, tx, id, "confirm", now)
		case err != nil:
			return fmt.Errorf("find subscription: %w", err)
		}
		// Existing: resend to the address on file (never to the new one).
		if existing.confirmationSentAt != nil && now.Sub(*existing.confirmationSentAt) < resendAfter {
			return nil
		}
		if _, err := tx.Exec(ctx, "UPDATE subscriptions SET confirmation_sent_at = $2 WHERE id = $1", existing.ID, now); err != nil {
			return err
		}
		if existing.Email == nil {
			return nil
		}
		kind := "manage_link"
		if existing.Status == StatusPending {
			kind = "confirm"
		}
		return enqueue(ctx, tx, existing.ID, kind, now)
	})
}

func newSecret() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return "whsec_" + hex.EncodeToString(b)
}

// Confirm activates a pending subscription.
func (s *Service) Confirm(ctx context.Context, token string) (ManageView, error) {
	id, err := s.resolve(ctx, token, purposeConfirm)
	if err != nil {
		return ManageView{}, err
	}
	now := s.now()
	var out Subscription
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		cur, err := get(ctx, tx, id, true)
		if err != nil {
			if _, ok := apperror.As(err); ok {
				return invalidToken()
			}
			return err
		}
		if cur.Status == StatusPending {
			if _, err := tx.Exec(ctx, "UPDATE subscriptions SET status = 'active', confirmed_at = $2, updated_at = $2 WHERE id = $1", id, now); err != nil {
				return err
			}
		}
		out, err = get(ctx, tx, id, false)
		return err
	})
	return out.manageView(), err
}

// Manage returns the subscription behind a manage (or confirm) token.
func (s *Service) Manage(ctx context.Context, token string) (ManageView, error) {
	id, err := s.resolve(ctx, token, purposeManage)
	if err != nil {
		return ManageView{}, err
	}
	x, err := get(ctx, s.pool, id, false)
	if _, ok := apperror.As(err); ok {
		return ManageView{}, invalidToken()
	}
	return x.manageView(), err
}

// PreferencesInput changes which components are followed.
type PreferencesInput struct {
	Token        string   `json:"token"`
	ComponentIDs []string `json:"component_ids"`
}

// UpdatePreferences sets the followed components (empty = all).
func (s *Service) UpdatePreferences(ctx context.Context, in PreferencesInput) (ManageView, error) {
	id, err := s.resolve(ctx, in.Token, purposeManage)
	if err != nil {
		return ManageView{}, err
	}
	var v apperror.Validator
	comps := resolveComponents(ctx, s.pool, &v, in.ComponentIDs)
	if err := v.Err(); err != nil {
		return ManageView{}, err
	}
	var out Subscription
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := get(ctx, tx, id, true); err != nil {
			return invalidToken()
		}
		if _, err := tx.Exec(ctx, "UPDATE subscriptions SET all_components = $2, updated_at = $3 WHERE id = $1", id, len(comps) == 0, s.now()); err != nil {
			return err
		}
		if err := setComponents(ctx, tx, id, comps); err != nil {
			return err
		}
		out, err = get(ctx, tx, id, false)
		return err
	})
	return out.manageView(), err
}

// Unsubscribe deletes the subscription (and its queued messages). Unknown
// or already removed tokens succeed silently, so it is idempotent.
func (s *Service) Unsubscribe(ctx context.Context, token string) error {
	id, err := s.resolve(ctx, token, purposeManage)
	if err != nil {
		if id2, err2 := s.resolve(ctx, token, purposeConfirm); err2 == nil {
			id = id2
		} else {
			return err
		}
	}
	_, err = s.pool.Exec(ctx, "DELETE FROM subscriptions WHERE id = $1", id)
	return err
}

// --- admin ---

// AdminFilter narrows the admin listing.
type AdminFilter struct {
	Channel, Status, Search string
}

// List returns a page of subscriptions.
func (s *Service) List(ctx context.Context, f AdminFilter, page httpx.Page) ([]Subscription, int, map[string]int, error) {
	search := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(f.Search)
	where := `WHERE ($1 = '' OR s.channel = $1) AND ($2 = '' OR s.status = $2)
		AND ($3 = '' OR s.email ILIKE '%' || $3 || '%' OR s.target_url ILIKE '%' || $3 || '%')`
	var total int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM subscriptions s "+where, f.Channel, f.Status, search).Scan(&total); err != nil {
		return nil, 0, nil, fmt.Errorf("count subscriptions: %w", err)
	}
	rows, err := s.pool.Query(ctx, "SELECT "+columns+" FROM subscriptions s "+where+" ORDER BY s.created_at DESC, s.id LIMIT $4 OFFSET $5",
		f.Channel, f.Status, search, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("list subscriptions: %w", err)
	}
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Subscription, error) { return scan(r) })
	if err != nil {
		return nil, 0, nil, err
	}
	counts := map[string]int{StatusPending: 0, StatusActive: 0, StatusDisabled: 0}
	crow, err := s.pool.Query(ctx, "SELECT status, count(*) FROM subscriptions GROUP BY status")
	if err != nil {
		return nil, 0, nil, err
	}
	defer crow.Close()
	for crow.Next() {
		var st string
		var n int
		if err := crow.Scan(&st, &n); err != nil {
			return nil, 0, nil, err
		}
		counts[st] = n
	}
	return items, total, counts, crow.Err()
}

// AdminCreateInput creates an integration (or invites an e-mail).
type AdminCreateInput struct {
	Channel      string   `json:"channel"`
	Email        *string  `json:"email"`
	URL          *string  `json:"url"`
	ComponentIDs []string `json:"component_ids"`
}

// Created carries the signing secret, shown only once.
type Created struct {
	Subscription
	SigningSecret *string `json:"signing_secret"`
}

// AdminCreate creates a webhook or Slack integration, active at once, or
// an e-mail subscription that still needs the recipient's confirmation.
func (s *Service) AdminCreate(ctx context.Context, in AdminCreateInput) (Created, error) {
	var v apperror.Validator
	v.Check(in.Channel == ChannelEmail || in.Channel == ChannelWebhook || in.Channel == ChannelSlack, "channel", "Canal inválido. Valores aceitos: email, webhook, slack.")
	var email, target *string
	if in.Email != nil && strings.TrimSpace(*in.Email) != "" {
		e := normalizeEmail(&v, *in.Email)
		email = &e
	}
	switch in.Channel {
	case ChannelEmail:
		v.Check(email != nil, "email", "Informe o e-mail.")
		if !s.EmailEnabled() {
			v.Add("channel", "O envio de e-mail não está configurado (SMTP_HOST e PUBLIC_BASE_URL).")
		}
	case ChannelWebhook, ChannelSlack:
		t := normalizeTarget(&v, in.Channel, deref(in.URL))
		target = &t
	}
	comps := resolveComponents(ctx, s.pool, &v, in.ComponentIDs)
	if err := v.Err(); err != nil {
		return Created{}, err
	}
	now := s.now()
	var out Created
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var secret *string
		if in.Channel == ChannelWebhook {
			x := newSecret()
			secret = &x
		}
		status, confirmed := StatusActive, &now
		if in.Channel == ChannelEmail {
			status, confirmed = StatusPending, nil
		}
		var id string
		err := tx.QueryRow(ctx, `INSERT INTO subscriptions (channel, email, target_url, signing_secret, status, all_components, created_by,
			confirmed_at, confirmation_sent_at, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, 'admin', $7, $8, $8, $8) RETURNING id`,
			in.Channel, email, target, secret, status, len(comps) == 0, confirmed, now).Scan(&id)
		if name, ok := database.ConstraintError(err, database.CodeUniqueViolation); ok {
			field := "url"
			if name == "subscriptions_email_key" {
				field = "email"
			}
			return &apperror.Error{Kind: apperror.KindConflict, Code: "subscription_exists", Message: "Já existe uma inscrição para este destino.",
				Fields: []apperror.FieldError{{Field: field, Message: "Já cadastrado."}}}
		}
		if err != nil {
			return fmt.Errorf("create subscription: %w", err)
		}
		if err := setComponents(ctx, tx, id, comps); err != nil {
			return err
		}
		if in.Channel == ChannelEmail {
			if err := enqueue(ctx, tx, id, "confirm", now); err != nil {
				return err
			}
		}
		sub, err := get(ctx, tx, id, false)
		if err != nil {
			return err
		}
		out = Created{Subscription: sub, SigningSecret: secret}
		return audit.Record(ctx, tx, audit.ResourceSubscription, id, "created", audit.Changes{After: map[string]any{"channel": in.Channel, "status": status}}, now)
	})
	return out, err
}

// AdminPatchInput enables/disables or changes followed components.
type AdminPatchInput struct {
	Enabled      *bool     `json:"enabled"`
	ComponentIDs *[]string `json:"component_ids"`
}

// AdminUpdate applies AdminPatchInput.
func (s *Service) AdminUpdate(ctx context.Context, id string, in AdminPatchInput) (Subscription, error) {
	if in.Enabled == nil && in.ComponentIDs == nil {
		return Subscription{}, apperror.Invalid("empty_update", "Informe ao menos um campo para atualizar.")
	}
	var v apperror.Validator
	var comps []string
	if in.ComponentIDs != nil {
		comps = resolveComponents(ctx, s.pool, &v, *in.ComponentIDs)
	}
	if err := v.Err(); err != nil {
		return Subscription{}, err
	}
	now := s.now()
	var out Subscription
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		cur, err := get(ctx, tx, id, true)
		if err != nil {
			return err
		}
		action := "updated"
		if in.Enabled != nil {
			switch {
			case *in.Enabled && cur.Status == StatusPending:
				return apperror.Conflict("subscription_pending", "A inscrição aguarda confirmação do destinatário e não pode ser ativada por um administrador.")
			case *in.Enabled && cur.Status == StatusDisabled:
				action = "enabled"
				_, err = tx.Exec(ctx, "UPDATE subscriptions SET status = 'active', disabled_reason = NULL, failure_count = 0, updated_at = $2 WHERE id = $1", id, now)
			case !*in.Enabled && cur.Status != StatusDisabled:
				action = "disabled"
				_, err = tx.Exec(ctx, "UPDATE subscriptions SET status = 'disabled', disabled_reason = 'Desativada por um administrador.', updated_at = $2 WHERE id = $1", id, now)
			}
			if err != nil {
				return err
			}
		}
		if in.ComponentIDs != nil {
			if _, err := tx.Exec(ctx, "UPDATE subscriptions SET all_components = $2, updated_at = $3 WHERE id = $1", id, len(comps) == 0, now); err != nil {
				return err
			}
			if err := setComponents(ctx, tx, id, comps); err != nil {
				return err
			}
		}
		if out, err = get(ctx, tx, id, false); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.ResourceSubscription, id, action,
			audit.Changes{Before: map[string]any{"status": cur.Status}, After: map[string]any{"status": out.Status, "all_components": out.AllComponents}}, now)
	})
	return out, err
}

// AdminDelete removes a subscription.
func (s *Service) AdminDelete(ctx context.Context, id string) error {
	now := s.now()
	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		cur, err := get(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM subscriptions WHERE id = $1", id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.ResourceSubscription, id, "deleted", audit.Changes{Before: map[string]any{"channel": cur.Channel, "status": cur.Status}}, now)
	})
}

// AdminTest queues a test message to an active subscription.
func (s *Service) AdminTest(ctx context.Context, id string) error {
	now := s.now()
	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		cur, err := get(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if cur.Status != StatusActive {
			return apperror.Conflict("subscription_not_active", "Só é possível testar inscrições ativas.")
		}
		if err := enqueue(ctx, tx, id, "test", now); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.ResourceSubscription, id, "test_queued", audit.Changes{}, now)
	})
}
