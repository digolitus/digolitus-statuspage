// Package monitoring receives alerts pushed by monitoring tools (UptimeRobot,
// Healthchecks.io) and turns them into a status input for components.
//
// Each integration has a secret webhook token (only its SHA-256 is stored).
// Signals create or update "monitors" (the tool's checks); operators link a
// monitor to a component and choose the status it implies while down. The
// status policy then treats a down monitor like an incident: the component's
// effective status is the most severe of all inputs (see ADR 0015).
package monitoring

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
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
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/optional"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/statuspolicy"
)

// Providers.
const (
	ProviderUptimeRobot  = "uptimerobot"
	ProviderHealthchecks = "healthchecks"
)

// Providers lists the supported tools.
var Providers = []string{ProviderUptimeRobot, ProviderHealthchecks}

// Monitor states.
const (
	StateUnknown = "unknown"
	StateUp      = "up"
	StateDown    = "down"
)

// Limits.
const (
	MaxIntegrations = 50
	MaxMonitors     = 500
	// KeepEvents is how many received signals are kept per integration.
	KeepEvents = 200
)

// SourceWebhook tags audit records written by received signals.
const SourceWebhook = "monitoring_webhook"

// Integration is the admin representation of an integration.
type Integration struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Provider       string     `json:"provider"`
	IsActive       bool       `json:"is_active"`
	TokenHint      string     `json:"token_hint"`
	LastReceivedAt *time.Time `json:"last_received_at"`
	MonitorCount   int        `json:"monitor_count"`
	LinkedCount    int        `json:"linked_count"`
	DownCount      int        `json:"down_count"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// IntegrationWithToken is returned once, when a token is created.
type IntegrationWithToken struct {
	Integration
	WebhookToken string `json:"webhook_token"`
}

// ComponentRef names the component a monitor is linked to.
type ComponentRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Monitor is a tool-side check.
type Monitor struct {
	ID              string        `json:"id"`
	IntegrationID   string        `json:"integration_id"`
	IntegrationName string        `json:"integration_name"`
	Provider        string        `json:"provider"`
	ExternalID      string        `json:"external_id"`
	Name            string        `json:"name"`
	Component       *ComponentRef `json:"component"`
	DownStatus      string        `json:"down_status"`
	State           string        `json:"state"`
	StateChangedAt  *time.Time    `json:"state_changed_at"`
	LastSignalAt    *time.Time    `json:"last_signal_at"`
}

// Event is a received signal.
type Event struct {
	ID          int64     `json:"id"`
	ReceivedAt  time.Time `json:"received_at"`
	Kind        string    `json:"kind"`
	MonitorID   *string   `json:"monitor_id"`
	MonitorName string    `json:"monitor_name"`
	Detail      string    `json:"detail"`
	Applied     bool      `json:"applied"`
}

// CreateInput registers an integration.
type CreateInput struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

// PatchInput edits an integration.
type PatchInput struct {
	Name     *string `json:"name"`
	IsActive *bool   `json:"is_active"`
}

// MonitorInput adds a monitor by hand (before its first signal).
type MonitorInput struct {
	ExternalID  string  `json:"external_id"`
	Name        *string `json:"name"`
	ComponentID *string `json:"component_id"`
	DownStatus  *string `json:"down_status"`
}

// MonitorPatch links a monitor or changes its effect.
type MonitorPatch struct {
	Name        *string                   `json:"name"`
	ComponentID optional.Nullable[string] `json:"component_id"`
	DownStatus  *string                   `json:"down_status"`
}

// ChangeHook is called inside the transaction that changed a linked
// monitor's state, so other modules (status history) can react atomically.
type ChangeHook func(ctx context.Context, tx pgx.Tx, monitorID string, at time.Time) error

// Service implements the monitoring use cases.
type Service struct {
	pool     *pgxpool.Pool
	now      clock.Clock
	onChange ChangeHook
}

// NewService creates a Service. onChange may be nil.
func NewService(pool *pgxpool.Pool, now clock.Clock, onChange ChangeHook) *Service {
	return &Service{pool: pool, now: now, onChange: onChange}
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hint(token string) string { return token[len(token)-4:] }

const integrationColumns = `i.id, i.name, i.provider, i.is_active, i.token_hint, i.last_received_at, i.created_at, i.updated_at,
	(SELECT count(*) FROM monitoring_monitors m WHERE m.integration_id = i.id)::int,
	(SELECT count(*) FROM monitoring_monitors m WHERE m.integration_id = i.id AND m.component_id IS NOT NULL)::int,
	(SELECT count(*) FROM monitoring_monitors m WHERE m.integration_id = i.id AND m.state = 'down')::int`

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func scanIntegration(row pgx.Row) (Integration, error) {
	var in Integration
	err := row.Scan(&in.ID, &in.Name, &in.Provider, &in.IsActive, &in.TokenHint, &in.LastReceivedAt, &in.CreatedAt, &in.UpdatedAt,
		&in.MonitorCount, &in.LinkedCount, &in.DownCount)
	in.CreatedAt, in.UpdatedAt, in.LastReceivedAt = in.CreatedAt.UTC(), in.UpdatedAt.UTC(), utc(in.LastReceivedAt)
	return in, err
}

// Get loads an integration.
func (s *Service) Get(ctx context.Context, q database.Querier, id string) (Integration, error) {
	in, err := scanIntegration(q.QueryRow(ctx, "SELECT "+integrationColumns+" FROM monitoring_integrations i WHERE i.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Integration{}, apperror.NotFound("monitoring_integration")
	}
	if err != nil {
		return Integration{}, fmt.Errorf("get monitoring integration: %w", err)
	}
	return in, nil
}

// List returns every integration by name.
func (s *Service) List(ctx context.Context) ([]Integration, error) {
	rows, err := s.pool.Query(ctx, "SELECT "+integrationColumns+" FROM monitoring_integrations i ORDER BY lower(i.name), i.id")
	if err != nil {
		return nil, fmt.Errorf("list monitoring integrations: %w", err)
	}
	defer rows.Close()
	out := []Integration{}
	for rows.Next() {
		in, err := scanIntegration(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, rows.Err()
}

func validateName(v *apperror.Validator, name string) {
	n := utf8.RuneCountInString(name)
	v.Check(n >= 1 && n <= 120, "name", "Informe um nome com 1 a 120 caracteres.")
}

func validDownStatus(s string) bool {
	switch s {
	case statuspolicy.DegradedPerformance, statuspolicy.PartialOutage, statuspolicy.MajorOutage:
		return true
	}
	return false
}

// Create registers an integration and returns its token (shown once).
func (s *Service) Create(ctx context.Context, in CreateInput) (IntegrationWithToken, error) {
	var v apperror.Validator
	name := strings.TrimSpace(in.Name)
	validateName(&v, name)
	v.Check(in.Provider == ProviderUptimeRobot || in.Provider == ProviderHealthchecks, "provider", "Escolha uma ferramenta suportada.")
	if err := v.Err(); err != nil {
		return IntegrationWithToken{}, err
	}
	token, err := newToken()
	if err != nil {
		return IntegrationWithToken{}, err
	}
	now := s.now()
	var id string
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM monitoring_integrations").Scan(&count); err != nil {
			return err
		}
		if count >= MaxIntegrations {
			return apperror.Conflict("monitoring_integration_limit", fmt.Sprintf("Limite de %d integrações atingido.", MaxIntegrations))
		}
		if err := tx.QueryRow(ctx, `INSERT INTO monitoring_integrations (name, provider, token_hash, token_hint, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $5) RETURNING id`, name, in.Provider, hashToken(token), hint(token), now).Scan(&id); err != nil {
			return fmt.Errorf("insert monitoring integration: %w", err)
		}
		return audit.Record(ctx, tx, audit.ResourceMonitoringIntegration, id, "created",
			audit.Changes{After: map[string]any{"name": name, "provider": in.Provider}}, now)
	})
	if err != nil {
		return IntegrationWithToken{}, err
	}
	got, err := s.Get(ctx, s.pool, id)
	return IntegrationWithToken{Integration: got, WebhookToken: token}, err
}

// Patch renames or (de)activates an integration. Deactivating it removes the
// effect of its monitors on component status.
func (s *Service) Patch(ctx context.Context, id string, in PatchInput) (Integration, error) {
	if in.Name == nil && in.IsActive == nil {
		return Integration{}, apperror.Invalid("empty_update", "Informe ao menos um campo para atualizar.")
	}
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		cur, err := s.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		next := cur
		var v apperror.Validator
		if in.Name != nil {
			next.Name = strings.TrimSpace(*in.Name)
			validateName(&v, next.Name)
		}
		if in.IsActive != nil {
			next.IsActive = *in.IsActive
		}
		if err := v.Err(); err != nil {
			return err
		}
		now := s.now()
		if _, err := tx.Exec(ctx, "UPDATE monitoring_integrations SET name = $2, is_active = $3, updated_at = $4 WHERE id = $1",
			id, next.Name, next.IsActive, now); err != nil {
			return fmt.Errorf("update monitoring integration: %w", err)
		}
		type view struct {
			Name     string `json:"name"`
			IsActive bool   `json:"is_active"`
		}
		if err := audit.Record(ctx, tx, audit.ResourceMonitoringIntegration, id, "updated",
			audit.Diff(view{cur.Name, cur.IsActive}, view{next.Name, next.IsActive}), now); err != nil {
			return err
		}
		if cur.IsActive != next.IsActive {
			return s.notifyIntegration(ctx, tx, id, now)
		}
		return nil
	})
	if err != nil {
		return Integration{}, err
	}
	return s.Get(ctx, s.pool, id)
}

// notifyIntegration runs the change hook for every down, linked monitor of
// an integration whose activation changed.
func (s *Service) notifyIntegration(ctx context.Context, tx pgx.Tx, id string, at time.Time) error {
	if s.onChange == nil {
		return nil
	}
	rows, err := tx.Query(ctx, "SELECT id FROM monitoring_monitors WHERE integration_id = $1 AND state = 'down' AND component_id IS NOT NULL", id)
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, mid := range ids {
		if err := s.onChange(ctx, tx, mid, at); err != nil {
			return err
		}
	}
	return nil
}

// Delete removes an integration with its monitors and events.
func (s *Service) Delete(ctx context.Context, id string) error {
	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		cur, err := s.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		now := s.now()
		// Run the hook first: once deleted, the monitors no longer exist.
		if cur.IsActive {
			if _, err := tx.Exec(ctx, "UPDATE monitoring_integrations SET is_active = false WHERE id = $1", id); err != nil {
				return err
			}
			if err := s.notifyIntegration(ctx, tx, id, now); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, "DELETE FROM monitoring_integrations WHERE id = $1", id); err != nil {
			return fmt.Errorf("delete monitoring integration: %w", err)
		}
		return audit.Record(ctx, tx, audit.ResourceMonitoringIntegration, id, "deleted",
			audit.Changes{Before: map[string]any{"name": cur.Name, "provider": cur.Provider}}, now)
	})
}

// RotateToken replaces the webhook token; the old URL stops working.
func (s *Service) RotateToken(ctx context.Context, id string) (IntegrationWithToken, error) {
	token, err := newToken()
	if err != nil {
		return IntegrationWithToken{}, err
	}
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := s.Get(ctx, tx, id); err != nil {
			return err
		}
		now := s.now()
		if _, err := tx.Exec(ctx, "UPDATE monitoring_integrations SET token_hash = $2, token_hint = $3, updated_at = $4 WHERE id = $1",
			id, hashToken(token), hint(token), now); err != nil {
			return fmt.Errorf("rotate monitoring token: %w", err)
		}
		return audit.Record(ctx, tx, audit.ResourceMonitoringIntegration, id, "token_rotated", audit.Changes{}, now)
	})
	if err != nil {
		return IntegrationWithToken{}, err
	}
	got, err := s.Get(ctx, s.pool, id)
	return IntegrationWithToken{Integration: got, WebhookToken: token}, err
}

const monitorColumns = `m.id, m.integration_id, i.name, i.provider, m.external_id, m.name, c.id, c.name, m.down_status, m.state,
	m.state_changed_at, m.last_signal_at`

const monitorFrom = ` FROM monitoring_monitors m JOIN monitoring_integrations i ON i.id = m.integration_id
	LEFT JOIN components c ON c.id = m.component_id `

func scanMonitor(row pgx.Row) (Monitor, error) {
	var m Monitor
	var cid, cname *string
	err := row.Scan(&m.ID, &m.IntegrationID, &m.IntegrationName, &m.Provider, &m.ExternalID, &m.Name, &cid, &cname, &m.DownStatus, &m.State,
		&m.StateChangedAt, &m.LastSignalAt)
	if cid != nil && cname != nil {
		m.Component = &ComponentRef{ID: *cid, Name: *cname}
	}
	m.StateChangedAt, m.LastSignalAt = utc(m.StateChangedAt), utc(m.LastSignalAt)
	return m, err
}

// MonitorFilter narrows a monitor listing.
type MonitorFilter struct {
	IntegrationID string
	ComponentID   string
	State         string
}

// Monitors lists monitors: linked ones first, then by name.
func (s *Service) Monitors(ctx context.Context, f MonitorFilter) ([]Monitor, error) {
	rows, err := s.pool.Query(ctx, "SELECT "+monitorColumns+monitorFrom+`
		WHERE ($1 = '' OR m.integration_id = NULLIF($1, '')::uuid) AND ($2 = '' OR m.component_id = NULLIF($2, '')::uuid)
		AND ($3 = '' OR m.state = $3)
		ORDER BY (m.component_id IS NULL), lower(i.name), lower(m.name), m.external_id LIMIT 1000`, f.IntegrationID, f.ComponentID, f.State)
	if err != nil {
		return nil, fmt.Errorf("list monitors: %w", err)
	}
	defer rows.Close()
	out := []Monitor{}
	for rows.Next() {
		m, err := scanMonitor(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Service) getMonitor(ctx context.Context, q database.Querier, id string, forUpdate bool) (Monitor, error) {
	lock := ""
	if forUpdate {
		lock = " FOR UPDATE OF m"
	}
	m, err := scanMonitor(q.QueryRow(ctx, "SELECT "+monitorColumns+monitorFrom+"WHERE m.id = $1"+lock, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Monitor{}, apperror.NotFound("monitor")
	}
	if err != nil {
		return Monitor{}, fmt.Errorf("get monitor: %w", err)
	}
	return m, nil
}

// resolveComponent checks that id is an active component. (Package
// component is not imported: it depends on statushistory, which reads the
// monitoring input from this package.)
func resolveComponent(ctx context.Context, q database.Querier, id string) error {
	invalid := func(msg string) error {
		return &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
			Fields: []apperror.FieldError{{Field: "component_id", Message: msg}}}
	}
	if !httpx.IsUUID(id) {
		return invalid("Identificador inválido.")
	}
	var active bool
	err := q.QueryRow(ctx, "SELECT is_active FROM components WHERE id = $1", id).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return invalid("Componente não encontrado.")
	}
	if err != nil {
		return fmt.Errorf("check component: %w", err)
	}
	if !active {
		return invalid("Componente desativado.")
	}
	return nil
}

func (s *Service) countMonitors(ctx context.Context, q database.Querier, integrationID string) error {
	var n int
	if err := q.QueryRow(ctx, "SELECT count(*) FROM monitoring_monitors WHERE integration_id = $1", integrationID).Scan(&n); err != nil {
		return err
	}
	if n >= MaxMonitors {
		return apperror.Conflict("monitor_limit", fmt.Sprintf("Limite de %d monitores por integração atingido.", MaxMonitors))
	}
	return nil
}

// AddMonitor registers a monitor by its tool identifier.
func (s *Service) AddMonitor(ctx context.Context, integrationID string, in MonitorInput) (Monitor, error) {
	var v apperror.Validator
	ext := strings.TrimSpace(in.ExternalID)
	v.Check(utf8.RuneCountInString(ext) >= 1 && utf8.RuneCountInString(ext) <= 128, "external_id", "Informe o identificador do monitor (até 128 caracteres).")
	name := ""
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
		v.Check(utf8.RuneCountInString(name) <= 255, "name", "Use no máximo 255 caracteres.")
	}
	down := statuspolicy.MajorOutage
	if in.DownStatus != nil {
		down = *in.DownStatus
		v.Check(validDownStatus(down), "down_status", "Escolha um status de indisponibilidade.")
	}
	if err := v.Err(); err != nil {
		return Monitor{}, err
	}
	var id string
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := s.Get(ctx, tx, integrationID); err != nil {
			return err
		}
		if err := s.countMonitors(ctx, tx, integrationID); err != nil {
			return err
		}
		var cid *string
		if in.ComponentID != nil {
			c := strings.ToLower(*in.ComponentID)
			if err := resolveComponent(ctx, tx, c); err != nil {
				return err
			}
			cid = &c
		}
		now := s.now()
		err := tx.QueryRow(ctx, `INSERT INTO monitoring_monitors (integration_id, external_id, name, component_id, down_status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $6) RETURNING id`, integrationID, ext, name, cid, down, now).Scan(&id)
		if c, ok := database.ConstraintError(err, database.CodeUniqueViolation); ok && c == "monitoring_monitors_integration_external_key" {
			return &apperror.Error{Kind: apperror.KindConflict, Code: "monitor_conflict", Message: "Este monitor já está cadastrado.",
				Fields: []apperror.FieldError{{Field: "external_id", Message: "Identificador já cadastrado nesta integração."}}}
		}
		if err != nil {
			return fmt.Errorf("insert monitor: %w", err)
		}
		return audit.Record(ctx, tx, audit.ResourceMonitoringIntegration, integrationID, "monitor_added",
			audit.Changes{After: map[string]any{"monitor_id": id, "external_id": ext, "component_id": cid, "down_status": down}}, now)
	})
	if err != nil {
		return Monitor{}, err
	}
	return s.getMonitor(ctx, s.pool, id, false)
}

// PatchMonitor links/unlinks a component or changes the implied status.
func (s *Service) PatchMonitor(ctx context.Context, id string, in MonitorPatch) (Monitor, error) {
	if in.Name == nil && !in.ComponentID.Set && in.DownStatus == nil {
		return Monitor{}, apperror.Invalid("empty_update", "Informe ao menos um campo para atualizar.")
	}
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		cur, err := s.getMonitor(ctx, tx, id, true)
		if err != nil {
			return err
		}
		var v apperror.Validator
		name := cur.Name
		if in.Name != nil {
			name = strings.TrimSpace(*in.Name)
			v.Check(utf8.RuneCountInString(name) <= 255, "name", "Use no máximo 255 caracteres.")
		}
		down := cur.DownStatus
		if in.DownStatus != nil {
			down = *in.DownStatus
			v.Check(validDownStatus(down), "down_status", "Escolha um status de indisponibilidade.")
		}
		if err := v.Err(); err != nil {
			return err
		}
		var curCID, nextCID *string
		if cur.Component != nil {
			curCID = &cur.Component.ID
		}
		nextCID = curCID
		if in.ComponentID.Set {
			nextCID = nil
			if in.ComponentID.Value != nil {
				c := strings.ToLower(*in.ComponentID.Value)
				if curCID == nil || *curCID != c {
					if err := resolveComponent(ctx, tx, c); err != nil {
						return err
					}
				}
				nextCID = &c
			}
		}
		now := s.now()
		if _, err := tx.Exec(ctx, "UPDATE monitoring_monitors SET name = $2, component_id = $3, down_status = $4, updated_at = $5 WHERE id = $1",
			id, name, nextCID, down, now); err != nil {
			return fmt.Errorf("update monitor: %w", err)
		}
		type view struct {
			Name        string  `json:"name"`
			ComponentID *string `json:"component_id"`
			DownStatus  string  `json:"down_status"`
		}
		changes := audit.Diff(view{cur.Name, curCID, cur.DownStatus}, view{name, nextCID, down})
		changes.After.(map[string]any)["monitor_id"] = id
		if err := audit.Record(ctx, tx, audit.ResourceMonitoringIntegration, cur.IntegrationID, "monitor_updated", changes, now); err != nil {
			return err
		}
		// A down monitor that changed link or effect changes a status input.
		if cur.State == StateDown && s.onChange != nil && (!sameRef(curCID, nextCID) || down != cur.DownStatus) {
			return s.onChange(ctx, tx, id, now)
		}
		return nil
	})
	if err != nil {
		return Monitor{}, err
	}
	return s.getMonitor(ctx, s.pool, id, false)
}

func sameRef(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// DeleteMonitor removes a monitor (it is recreated if the tool keeps sending).
func (s *Service) DeleteMonitor(ctx context.Context, id string) error {
	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		cur, err := s.getMonitor(ctx, tx, id, true)
		if err != nil {
			return err
		}
		now := s.now()
		if cur.State == StateDown && cur.Component != nil && s.onChange != nil {
			// Clear the effect first so the hook sees the new situation.
			if _, err := tx.Exec(ctx, "UPDATE monitoring_monitors SET component_id = NULL WHERE id = $1", id); err != nil {
				return err
			}
			if err := s.onChange(ctx, tx, id, now); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, "DELETE FROM monitoring_monitors WHERE id = $1", id); err != nil {
			return fmt.Errorf("delete monitor: %w", err)
		}
		return audit.Record(ctx, tx, audit.ResourceMonitoringIntegration, cur.IntegrationID, "monitor_deleted",
			audit.Changes{Before: map[string]any{"monitor_id": id, "external_id": cur.ExternalID, "name": cur.Name}}, now)
	})
}

// Events lists the latest signals of an integration.
func (s *Service) Events(ctx context.Context, integrationID string) ([]Event, error) {
	if _, err := s.Get(ctx, s.pool, integrationID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id, received_at, kind, monitor_id, monitor_name, detail, applied FROM monitoring_events
		WHERE integration_id = $1 ORDER BY received_at DESC, id DESC LIMIT 50`, integrationID)
	if err != nil {
		return nil, fmt.Errorf("list monitoring events: %w", err)
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.ReceivedAt, &e.Kind, &e.MonitorID, &e.MonitorName, &e.Detail, &e.Applied); err != nil {
			return nil, err
		}
		e.ReceivedAt = e.ReceivedAt.UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}

// ErrUnknownToken is returned for webhook calls with a missing or unknown
// token; callers answer 404 without detail.
var ErrUnknownToken = apperror.NotFound("webhook")

// Receive handles a webhook call; fieldsErr is a failure to read the body
// (see Fields). It returns ErrUnknownToken for an unknown
// token and an Invalid error when the payload cannot be read (the event is
// still recorded so operators can see what arrived).
func (s *Service) Receive(ctx context.Context, token string, fields map[string]string, fieldsErr error) error {
	if token == "" || len(token) > 128 {
		return ErrUnknownToken
	}
	ctx = httpx.ContextWithSource(ctx, SourceWebhook)
	var rejected *apperror.Error
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var id, provider string
		var active bool
		err := tx.QueryRow(ctx, "SELECT id, provider, is_active FROM monitoring_integrations WHERE token_hash = $1 FOR UPDATE", hashToken(token)).
			Scan(&id, &provider, &active)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnknownToken
		}
		if err != nil {
			return fmt.Errorf("find monitoring integration: %w", err)
		}
		now := s.now()
		if _, err := tx.Exec(ctx, "UPDATE monitoring_integrations SET last_received_at = $2 WHERE id = $1", id, now); err != nil {
			return err
		}
		var sig Signal
		perr := fieldsErr
		if perr == nil {
			sig, perr = Parse(provider, fields)
		}
		if perr != nil {
			var pe *errPayload
			if !errors.As(perr, &pe) {
				return perr
			}
			if err := s.recordEvent(ctx, tx, id, nil, KindInvalid, sig.Name, pe.msg, false, now); err != nil {
				return err
			}
			// Commit the event, then report the problem to the tool.
			rejected = &apperror.Error{Kind: apperror.KindInvalid, Code: "invalid_signal", Message: "Não foi possível ler o alerta: " + pe.msg + "."}
			return nil
		}
		if !active {
			return s.recordEvent(ctx, tx, id, nil, KindIgnored, sig.Name, "integração desativada", false, now)
		}
		monitorID, applied, err := s.apply(ctx, tx, id, sig, now)
		if err != nil {
			return err
		}
		kind, detail := sig.Kind, sig.Detail
		if monitorID == "" {
			kind, detail = KindIgnored, "limite de monitores atingido"
		}
		var mid *string
		if monitorID != "" {
			mid = &monitorID
		}
		return s.recordEvent(ctx, tx, id, mid, kind, sig.Name, detail, applied, now)
	})
	if err == nil && rejected != nil {
		return rejected
	}
	return err
}

// apply upserts the monitor and moves its state. It returns the monitor ID
// ("" when the monitor limit is reached) and whether a linked component's
// status input changed.
func (s *Service) apply(ctx context.Context, tx pgx.Tx, integrationID string, sig Signal, now time.Time) (string, bool, error) {
	var id, state string
	var cid *string
	err := tx.QueryRow(ctx, "SELECT id, state, component_id FROM monitoring_monitors WHERE integration_id = $1 AND external_id = $2 FOR UPDATE",
		integrationID, sig.ExternalID).Scan(&id, &state, &cid)
	if errors.Is(err, pgx.ErrNoRows) {
		if s.countMonitors(ctx, tx, integrationID) != nil {
			return "", false, nil
		}
		// The integration row is locked, so no concurrent insert can race.
		if err := tx.QueryRow(ctx, `INSERT INTO monitoring_monitors (integration_id, external_id, name, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $4) RETURNING id, state`, integrationID, sig.ExternalID, sig.Name, now).Scan(&id, &state); err != nil {
			return "", false, fmt.Errorf("insert monitor: %w", err)
		}
	} else if err != nil {
		return "", false, fmt.Errorf("find monitor: %w", err)
	}
	if sig.Name != "" {
		if _, err := tx.Exec(ctx, "UPDATE monitoring_monitors SET name = $2 WHERE id = $1 AND name <> $2", id, sig.Name); err != nil {
			return "", false, err
		}
	}
	if _, err := tx.Exec(ctx, "UPDATE monitoring_monitors SET last_signal_at = $2 WHERE id = $1", id, now); err != nil {
		return "", false, err
	}
	if sig.Kind != KindDown && sig.Kind != KindUp {
		return id, false, nil
	}
	if state == sig.Kind {
		return id, false, nil
	}
	if _, err := tx.Exec(ctx, "UPDATE monitoring_monitors SET state = $2, state_changed_at = $3, updated_at = $3 WHERE id = $1", id, sig.Kind, now); err != nil {
		return "", false, fmt.Errorf("update monitor state: %w", err)
	}
	// up after unknown changes nothing visible; down/up of a linked monitor does.
	applied := cid != nil && (sig.Kind == StateDown || state == StateDown)
	if !applied {
		return id, false, nil
	}
	if err := audit.Record(ctx, tx, audit.ResourceMonitoringIntegration, integrationID, "monitor_"+sig.Kind,
		audit.Changes{Before: map[string]any{"state": state}, After: map[string]any{"state": sig.Kind, "monitor_id": id, "component_id": *cid}}, now); err != nil {
		return "", false, err
	}
	if s.onChange != nil {
		if err := s.onChange(ctx, tx, id, now); err != nil {
			return "", false, err
		}
	}
	return id, true, nil
}

func (s *Service) recordEvent(ctx context.Context, tx pgx.Tx, integrationID string, monitorID *string, kind, name, detail string, applied bool, now time.Time) error {
	if _, err := tx.Exec(ctx, `INSERT INTO monitoring_events (integration_id, monitor_id, received_at, kind, monitor_name, detail, applied)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, integrationID, monitorID, now, kind, name, detail, applied); err != nil {
		return fmt.Errorf("insert monitoring event: %w", err)
	}
	_, err := tx.Exec(ctx, `DELETE FROM monitoring_events WHERE integration_id = $1 AND id NOT IN
		(SELECT id FROM monitoring_events WHERE integration_id = $1 ORDER BY received_at DESC, id DESC LIMIT $2)`, integrationID, KeepEvents)
	return err
}

// ComponentSignal is the monitoring input for one component.
type ComponentSignal struct {
	// Status is the most severe down_status of its down monitors, or ""
	// when none is down.
	Status string
	// Monitored is true when an active integration has reported on at least
	// one linked monitor (a monitor that never reported proves nothing).
	Monitored bool
}

// ComponentSignals returns the monitoring input of every component that has
// linked monitors in active integrations.
func ComponentSignals(ctx context.Context, q database.Querier) (map[string]ComponentSignal, error) {
	rows, err := q.Query(ctx, `SELECT m.component_id, m.state, m.down_status FROM monitoring_monitors m
		JOIN monitoring_integrations i ON i.id = m.integration_id
		WHERE i.is_active AND m.component_id IS NOT NULL AND m.state <> 'unknown'`)
	if err != nil {
		return nil, fmt.Errorf("monitoring signals: %w", err)
	}
	defer rows.Close()
	out := map[string]ComponentSignal{}
	for rows.Next() {
		var cid, state, down string
		if err := rows.Scan(&cid, &state, &down); err != nil {
			return nil, err
		}
		cs := out[cid]
		cs.Monitored = true
		if state == StateDown {
			cs.Status = statuspolicy.Worse(cs.Status, down)
		}
		out[cid] = cs
	}
	return out, rows.Err()
}

// ComponentStatuses maps component IDs to the status implied by their down
// monitors (components without a down monitor are absent).
func ComponentStatuses(ctx context.Context, q database.Querier) (map[string]string, error) {
	sigs, err := ComponentSignals(ctx, q)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for id, s := range sigs {
		if s.Status != "" {
			out[id] = s.Status
		}
	}
	return out, nil
}
