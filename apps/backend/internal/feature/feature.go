// Package feature holds optional features that administrators switch on and
// off at runtime (ADR 0019). The switch lives in the database so it takes
// effect without a restart; an environment variable can still lock a feature
// off for the whole deployment ("locked").
package feature

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/audit"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/auth"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/clock"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
)

// Keys of the known features.
const (
	Suspicions = "suspicions"
)

type definition struct {
	key, label, auditID string
}

// catalogue lists the features in display order. auditID is the fixed
// resource ID of the switch in the audit log.
var catalogue = []definition{
	{Suspicions, "Reportar suspeita", "00000000-0000-4000-8000-00000000f001"},
}

func lookup(key string) (definition, bool) {
	for _, d := range catalogue {
		if d.key == key {
			return d, true
		}
	}
	return definition{}, false
}

// Feature is a switch as shown in the admin area.
type Feature struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Enabled bool   `json:"enabled"`
	// Locked: the deployment turned the feature off (environment); the
	// switch cannot turn it on.
	Locked    bool       `json:"locked"`
	UpdatedAt *time.Time `json:"updated_at"`
	UpdatedBy *string    `json:"updated_by"`
}

// cacheTTL bounds how long a replica may serve a stale switch.
const cacheTTL = 5 * time.Second

// Service reads and changes switches.
type Service struct {
	pool   *pgxpool.Pool
	now    clock.Clock
	locked map[string]bool

	mu       sync.Mutex
	cache    map[string]bool
	cachedAt time.Time
}

// NewService builds the service. locked lists features the deployment
// turned off.
func NewService(pool *pgxpool.Pool, now clock.Clock, locked map[string]bool) *Service {
	return &Service{pool: pool, now: now, locked: locked}
}

// Enabled reports whether key is on. Database errors count as off.
func (s *Service) Enabled(ctx context.Context, key string) bool {
	if s.locked[key] {
		return false
	}
	s.mu.Lock()
	if s.cache != nil && time.Since(s.cachedAt) < cacheTTL {
		on := s.cache[key]
		s.mu.Unlock()
		return on
	}
	s.mu.Unlock()
	states, err := s.states(ctx)
	if err != nil {
		return false
	}
	s.mu.Lock()
	s.cache, s.cachedAt = states, time.Now()
	s.mu.Unlock()
	return states[key]
}

func (s *Service) states(ctx context.Context) (map[string]bool, error) {
	rows, err := s.pool.Query(ctx, "SELECT key, enabled FROM feature_flags")
	if err != nil {
		return nil, fmt.Errorf("read feature flags: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var k string
		var on bool
		if err := rows.Scan(&k, &on); err != nil {
			return nil, err
		}
		out[k] = on
	}
	return out, rows.Err()
}

func (s *Service) invalidate() {
	s.mu.Lock()
	s.cache = nil
	s.mu.Unlock()
}

// List returns every feature with its state.
func (s *Service) List(ctx context.Context) ([]Feature, error) {
	type row struct {
		enabled   bool
		updatedAt time.Time
		updatedBy *string
	}
	rows, err := s.pool.Query(ctx, `SELECT f.key, f.enabled, f.updated_at, u.name FROM feature_flags f
		LEFT JOIN users u ON u.id = f.updated_by`)
	if err != nil {
		return nil, fmt.Errorf("list feature flags: %w", err)
	}
	defer rows.Close()
	found := map[string]row{}
	for rows.Next() {
		var k string
		var r row
		if err := rows.Scan(&k, &r.enabled, &r.updatedAt, &r.updatedBy); err != nil {
			return nil, err
		}
		found[k] = r
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Feature, 0, len(catalogue))
	for _, d := range catalogue {
		f := Feature{Key: d.key, Label: d.label, Locked: s.locked[d.key]}
		if r, ok := found[d.key]; ok {
			at := r.updatedAt.UTC()
			f.Enabled, f.UpdatedAt, f.UpdatedBy = r.enabled && !f.Locked, &at, r.updatedBy
		}
		out = append(out, f)
	}
	return out, nil
}

// Set turns key on or off.
func (s *Service) Set(ctx context.Context, key string, enabled bool) (Feature, error) {
	d, ok := lookup(key)
	if !ok {
		return Feature{}, apperror.NotFound("feature")
	}
	if enabled && s.locked[key] {
		return Feature{}, apperror.Conflict("feature_locked", "Esta funcionalidade foi desligada na configuração do servidor e não pode ser ligada aqui.")
	}
	var userID *string
	if p := auth.FromContext(ctx); p != nil {
		userID = &p.UserID
	}
	now := s.now()
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var before bool
		err := tx.QueryRow(ctx, "SELECT enabled FROM feature_flags WHERE key = $1 FOR UPDATE", key).Scan(&before)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read feature flag: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO feature_flags (key, enabled, updated_at, updated_by) VALUES ($1, $2, $3, $4)
			ON CONFLICT (key) DO UPDATE SET enabled = $2, updated_at = $3, updated_by = $4`, key, enabled, now, userID); err != nil {
			return fmt.Errorf("save feature flag: %w", err)
		}
		return audit.Record(ctx, tx, audit.ResourceFeatureFlag, d.auditID, "updated", audit.Changes{
			Before: map[string]any{"key": key, "enabled": before}, After: map[string]any{"key": key, "enabled": enabled}}, now)
	})
	if err != nil {
		return Feature{}, err
	}
	s.invalidate()
	list, err := s.List(ctx)
	if err != nil {
		return Feature{}, err
	}
	for _, f := range list {
		if f.Key == key {
			return f, nil
		}
	}
	return Feature{}, apperror.NotFound("feature")
}

// Handler exposes the switches.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// PublicFeatures tells the public page which optional features are on.
type PublicFeatures struct {
	Suspicions bool `json:"suspicions"`
}

// RegisterPublic mounts the public read.
func (h *Handler) RegisterPublic(rt *httpx.Router) {
	rt.Handle(http.MethodGet, "/api/v1/features", h.public)
}

// RegisterAdmin mounts the admin routes.
func (h *Handler) RegisterAdmin(rt *httpx.Router, admin func(http.HandlerFunc) http.HandlerFunc) {
	rt.Handle(http.MethodGet, "/api/v1/admin/features", admin(h.list))
	rt.Handle(http.MethodPut, "/api/v1/admin/features/{key}", admin(h.set))
}

func (h *Handler) public(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	httpx.WriteJSON(w, http.StatusOK, PublicFeatures{Suspicions: h.svc.Enabled(r.Context(), Suspicions)})
}

type featureList struct {
	Data []Feature `json:"data"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.List(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, featureList{Data: list})
}

// SetInput turns a feature on or off.
type SetInput struct {
	Enabled *bool `json:"enabled"`
}

func (h *Handler) set(w http.ResponseWriter, r *http.Request) {
	var in SetInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.Enabled == nil {
		var v apperror.Validator
		v.Add("enabled", "Informe true ou false.")
		httpx.Fail(w, r, v.Err())
		return
	}
	f, err := h.svc.Set(r.Context(), r.PathValue("key"), *in.Enabled)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, f)
}

// Gate answers 404 while key is off, as if the routes did not exist.
func (s *Service) Gate(key string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.Enabled(r.Context(), key) {
			httpx.Fail(w, r, &apperror.Error{Kind: apperror.KindNotFound, Code: "feature_disabled", Message: "Funcionalidade desligada."})
			return
		}
		next(w, r)
	}
}
