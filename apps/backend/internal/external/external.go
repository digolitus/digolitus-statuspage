// Package external integrates third-party status pages (currently Atlassian
// Statuspage, e.g. githubstatus.com, status.veracode.com).
//
// Operators register a source by URL and choose which of its components to
// publish. A background poller stores a snapshot of the provider's status.
// Third-party data is reported by the provider: it is shown in its own
// section, marked stale when old, and never changes the platform's overall
// status (see docs/adr/0006-third-party-status.md).
package external

import (
	"context"
	"encoding/json"
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
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/statuspolicy"
)

// ProviderStatuspage is the only supported provider for now.
const ProviderStatuspage = "statuspage"

// MaxSources bounds the number of registered sources.
const MaxSources = 50

// Source is the admin representation of a third-party source.
type Source struct {
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	Provider             string     `json:"provider"`
	BaseURL              string     `json:"base_url"`
	PageName             string     `json:"page_name"`
	IsActive             bool       `json:"is_active"`
	DisplayOrder         int        `json:"display_order"`
	Indicator            *string    `json:"indicator"`
	IndicatorDescription string     `json:"indicator_description"`
	LastCheckedAt        *time.Time `json:"last_checked_at"`
	LastSuccessAt        *time.Time `json:"last_success_at"`
	LastError            *string    `json:"last_error"`
	Stale                bool       `json:"stale"`
	ComponentCount       int        `json:"component_count"`
	SelectedCount        int        `json:"selected_count"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`

	events []Event
}

// Component is a provider component as seen by operators.
type Component struct {
	ID          string  `json:"id"`
	ExternalID  string  `json:"external_id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	GroupName   *string `json:"group_name"`
	Status      string  `json:"status"`
	IsSelected  bool    `json:"is_selected"`
	IsPresent   bool    `json:"is_present"`
}

// CreateInput registers a source.
type CreateInput struct {
	URL          string  `json:"url"`
	Name         *string `json:"name"`
	DisplayOrder *int    `json:"display_order"`
}

// PatchInput edits a source; the URL is immutable (register a new source).
type PatchInput struct {
	Name         *string `json:"name"`
	IsActive     *bool   `json:"is_active"`
	DisplayOrder *int    `json:"display_order"`
}

// SelectionInput replaces the set of published components.
type SelectionInput struct {
	ComponentIDs []string `json:"component_ids"`
}

// Service implements third-party source use cases.
type Service struct {
	pool     *pgxpool.Pool
	now      clock.Clock
	fetcher  Fetcher
	interval time.Duration
}

// NewService creates a Service. interval is the polling period; data older
// than StaleAfter(interval) is reported as stale.
func NewService(pool *pgxpool.Pool, now clock.Clock, fetcher Fetcher, interval time.Duration) *Service {
	return &Service{pool: pool, now: now, fetcher: fetcher, interval: interval}
}

// StaleAfter is how old a successful snapshot may be before it is stale.
func StaleAfter(interval time.Duration) time.Duration {
	return max(3*interval, 10*time.Minute)
}

const sourceColumns = `id, name, provider, base_url, page_name, is_active, display_order, indicator, indicator_description,
	events, last_checked_at, last_success_at, last_error, created_at, updated_at,
	(SELECT count(*) FROM external_components c WHERE c.source_id = external_sources.id AND c.is_present)::int,
	(SELECT count(*) FROM external_components c WHERE c.source_id = external_sources.id AND c.is_present AND c.is_selected)::int`

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func (s *Service) scanSource(row pgx.Row) (Source, error) {
	var src Source
	var events []byte
	err := row.Scan(&src.ID, &src.Name, &src.Provider, &src.BaseURL, &src.PageName, &src.IsActive, &src.DisplayOrder, &src.Indicator,
		&src.IndicatorDescription, &events, &src.LastCheckedAt, &src.LastSuccessAt, &src.LastError, &src.CreatedAt, &src.UpdatedAt,
		&src.ComponentCount, &src.SelectedCount)
	if err != nil {
		return src, err
	}
	src.CreatedAt, src.UpdatedAt = src.CreatedAt.UTC(), src.UpdatedAt.UTC()
	src.LastCheckedAt, src.LastSuccessAt = utc(src.LastCheckedAt), utc(src.LastSuccessAt)
	src.Stale = src.LastSuccessAt == nil || s.now().Sub(*src.LastSuccessAt) > StaleAfter(s.interval)
	_ = json.Unmarshal(events, &src.events)
	return src, nil
}

// Get loads a source.
func (s *Service) Get(ctx context.Context, q database.Querier, id string) (Source, error) {
	src, err := s.scanSource(q.QueryRow(ctx, "SELECT "+sourceColumns+" FROM external_sources WHERE id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Source{}, apperror.NotFound("external_source")
	}
	if err != nil {
		return Source{}, fmt.Errorf("get external source: %w", err)
	}
	return src, nil
}

// List returns every source in display order (bounded by MaxSources).
func (s *Service) List(ctx context.Context, q database.Querier, onlyActive bool) ([]Source, error) {
	rows, err := q.Query(ctx, "SELECT "+sourceColumns+" FROM external_sources WHERE ($1 = false OR is_active) ORDER BY display_order, lower(name), id", onlyActive)
	if err != nil {
		return nil, fmt.Errorf("list external sources: %w", err)
	}
	defer rows.Close()
	var out []Source
	for rows.Next() {
		src, err := s.scanSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, rows.Err()
}

// Components lists a source's components, grouped as the provider does.
func Components(ctx context.Context, q database.Querier, sourceID string, onlySelected bool) ([]Component, error) {
	rows, err := q.Query(ctx, `SELECT id, external_id, name, description, group_name, status, is_selected, is_present
		FROM external_components WHERE source_id = $1 AND ($2 = false OR (is_selected AND is_present))
		ORDER BY is_present DESC, group_name NULLS FIRST, position, name, id`, sourceID, onlySelected)
	if err != nil {
		return nil, fmt.Errorf("list external components: %w", err)
	}
	defer rows.Close()
	out := []Component{}
	for rows.Next() {
		var c Component
		if err := rows.Scan(&c.ID, &c.ExternalID, &c.Name, &c.Description, &c.GroupName, &c.Status, &c.IsSelected, &c.IsPresent); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func validateName(v *apperror.Validator, name string) {
	n := utf8.RuneCountInString(name)
	v.Check(n >= 1 && n <= 120, "name", "Informe um nome com 1 a 120 caracteres.")
}

func validateOrder(v *apperror.Validator, o int) {
	v.Check(o >= 0 && o <= 100000, "display_order", "Informe um inteiro entre 0 e 100000.")
}

func fetchFailure(err error) error {
	var fe *FetchError
	msg := "Não foi possível consultar o provedor."
	if errors.As(err, &fe) {
		msg = "Não foi possível ler a página de status: " + fe.Message + "."
	}
	return &apperror.Error{Kind: apperror.KindUnavailable, Code: "external_fetch_failed", Message: msg}
}

// Create validates the URL, reads the provider once (so operators get
// immediate feedback and the component list) and stores the source with no
// component selected.
func (s *Service) Create(ctx context.Context, in CreateInput) (Source, error) {
	var v apperror.Validator
	base, err := NormalizeBaseURL(in.URL)
	if err != nil {
		v.Add("url", strings.ToUpper(err.Error()[:1])+err.Error()[1:]+".")
	}
	name := ""
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
		validateName(&v, name)
	}
	order := 0
	if in.DisplayOrder != nil {
		order = *in.DisplayOrder
		validateOrder(&v, order)
	}
	if err := v.Err(); err != nil {
		return Source{}, err
	}

	var count int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM external_sources").Scan(&count); err != nil {
		return Source{}, err
	}
	if count >= MaxSources {
		return Source{}, apperror.Conflict("external_source_limit", fmt.Sprintf("Limite de %d provedores atingido.", MaxSources))
	}

	snap, err := s.fetcher.Fetch(ctx, base)
	if err != nil {
		return Source{}, &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
			Fields: []apperror.FieldError{{Field: "url", Message: fetchFailure(err).(*apperror.Error).Message}}}
	}
	if name == "" {
		name = clip(snap.PageName, 120)
		if name == "" {
			name = strings.TrimPrefix(base, "https://")
		}
	}
	now := s.now()
	var id string
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `INSERT INTO external_sources (name, provider, base_url, display_order, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $5) RETURNING id`, name, ProviderStatuspage, base, order, now).Scan(&id)
		if err != nil {
			if c, ok := database.ConstraintError(err, database.CodeUniqueViolation); ok && c == "external_sources_base_url_key" {
				return &apperror.Error{Kind: apperror.KindConflict, Code: "external_source_conflict", Message: "Este provedor já está cadastrado.",
					Fields: []apperror.FieldError{{Field: "url", Message: "URL já cadastrada."}}}
			}
			return fmt.Errorf("insert external source: %w", err)
		}
		if err := saveSnapshot(ctx, tx, id, snap, now); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.ResourceExternalSource, id, "created",
			audit.Changes{After: map[string]any{"name": name, "base_url": base, "provider": ProviderStatuspage}}, now)
	})
	if err != nil {
		return Source{}, err
	}
	return s.Get(ctx, s.pool, id)
}

// Patch edits name, activation and order.
func (s *Service) Patch(ctx context.Context, id string, in PatchInput) (Source, error) {
	if in.Name == nil && in.IsActive == nil && in.DisplayOrder == nil {
		return Source{}, apperror.Invalid("empty_update", "Informe ao menos um campo para atualizar.")
	}
	var v apperror.Validator
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		cur, err := s.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		next := cur
		if in.Name != nil {
			next.Name = strings.TrimSpace(*in.Name)
			validateName(&v, next.Name)
		}
		if in.IsActive != nil {
			next.IsActive = *in.IsActive
		}
		if in.DisplayOrder != nil {
			next.DisplayOrder = *in.DisplayOrder
			validateOrder(&v, next.DisplayOrder)
		}
		if err := v.Err(); err != nil {
			return err
		}
		now := s.now()
		if _, err := tx.Exec(ctx, `UPDATE external_sources SET name = $2, is_active = $3, display_order = $4, updated_at = $5 WHERE id = $1`,
			id, next.Name, next.IsActive, next.DisplayOrder, now); err != nil {
			return fmt.Errorf("update external source: %w", err)
		}
		type view struct {
			Name         string `json:"name"`
			IsActive     bool   `json:"is_active"`
			DisplayOrder int    `json:"display_order"`
		}
		return audit.Record(ctx, tx, audit.ResourceExternalSource, id, "updated",
			audit.Diff(view{cur.Name, cur.IsActive, cur.DisplayOrder}, view{next.Name, next.IsActive, next.DisplayOrder}), now)
	})
	if err != nil {
		return Source{}, err
	}
	return s.Get(ctx, s.pool, id)
}

// Delete removes a source and its cached components.
func (s *Service) Delete(ctx context.Context, id string) error {
	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		cur, err := s.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM external_sources WHERE id = $1", id); err != nil {
			return fmt.Errorf("delete external source: %w", err)
		}
		return audit.Record(ctx, tx, audit.ResourceExternalSource, id, "deleted",
			audit.Changes{Before: map[string]any{"name": cur.Name, "base_url": cur.BaseURL}}, s.now())
	})
}

// SetSelection replaces the published components of a source.
func (s *Service) SetSelection(ctx context.Context, id string, in SelectionInput) ([]Component, error) {
	if in.ComponentIDs == nil {
		return nil, &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
			Fields: []apperror.FieldError{{Field: "component_ids", Message: "Informe a lista (pode ser vazia)."}}}
	}
	seen := map[string]bool{}
	ids := []string{}
	for _, raw := range in.ComponentIDs {
		cid := strings.ToLower(raw)
		if !httpx.IsUUID(cid) {
			return nil, &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
				Fields: []apperror.FieldError{{Field: "component_ids", Message: "Contém um identificador inválido."}}}
		}
		if !seen[cid] {
			seen[cid] = true
			ids = append(ids, cid)
		}
	}
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := s.Get(ctx, tx, id); err != nil {
			return err
		}
		var known int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM external_components WHERE source_id = $1 AND id = ANY($2::uuid[])", id, ids).Scan(&known); err != nil {
			return err
		}
		if known != len(ids) {
			return &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
				Fields: []apperror.FieldError{{Field: "component_ids", Message: "Um ou mais componentes não pertencem a este provedor."}}}
		}
		before, err := Components(ctx, tx, id, true)
		if err != nil {
			return err
		}
		now := s.now()
		if _, err := tx.Exec(ctx, `UPDATE external_components SET is_selected = (id = ANY($2::uuid[])), updated_at = $3
			WHERE source_id = $1 AND is_selected <> (id = ANY($2::uuid[]))`, id, ids, now); err != nil {
			return fmt.Errorf("update selection: %w", err)
		}
		names := func(cs []Component) []string {
			out := []string{}
			for _, c := range cs {
				out = append(out, c.Name)
			}
			return out
		}
		after, err := Components(ctx, tx, id, true)
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.ResourceExternalSource, id, "selection_changed",
			audit.Changes{Before: map[string]any{"components": names(before)}, After: map[string]any{"components": names(after)}}, now)
	})
	if err != nil {
		return nil, err
	}
	return Components(ctx, s.pool, id, false)
}

// Sync reads the provider now and stores the result (success or error).
func (s *Service) Sync(ctx context.Context, id string) (Source, error) {
	src, err := s.Get(ctx, s.pool, id)
	if err != nil {
		return Source{}, err
	}
	if err := s.syncOne(ctx, src.ID, src.BaseURL); err != nil {
		return Source{}, fetchFailure(err)
	}
	return s.Get(ctx, s.pool, id)
}

// syncOne fetches and persists one source; fetch errors are recorded on the
// source and returned.
func (s *Service) syncOne(ctx context.Context, id, base string) error {
	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	snap, fetchErr := s.fetcher.Fetch(fetchCtx, base)
	now := s.now()
	if fetchErr != nil {
		msg := "falha ao consultar o provedor"
		var fe *FetchError
		if errors.As(fetchErr, &fe) {
			msg = fe.Message
		}
		if _, err := s.pool.Exec(ctx, "UPDATE external_sources SET last_checked_at = $2, last_error = $3 WHERE id = $1", id, now, clip(msg, 500)); err != nil {
			return err
		}
		return fetchErr
	}
	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error { return saveSnapshot(ctx, tx, id, snap, now) })
}

func saveSnapshot(ctx context.Context, tx pgx.Tx, id string, snap Snapshot, now time.Time) error {
	events, err := json.Marshal(snap.Events)
	if err != nil {
		return err
	}
	if snap.Events == nil {
		events = []byte("[]")
	}
	var indicator *string
	if snap.Indicator != "" {
		indicator = &snap.Indicator
	}
	if _, err := tx.Exec(ctx, `UPDATE external_sources SET page_name = $2, indicator = $3, indicator_description = $4, events = $5,
		last_checked_at = $6, last_success_at = $6, last_error = NULL WHERE id = $1`,
		id, snap.PageName, indicator, snap.IndicatorDescription, events, now); err != nil {
		return fmt.Errorf("update external source snapshot: %w", err)
	}
	present := make([]string, 0, len(snap.Components))
	for _, c := range snap.Components {
		present = append(present, c.ExternalID)
		if _, err := tx.Exec(ctx, `INSERT INTO external_components
			(source_id, external_id, name, description, group_name, status, position, provider_updated_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
			ON CONFLICT (source_id, external_id) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description,
				group_name = EXCLUDED.group_name, status = EXCLUDED.status, position = EXCLUDED.position,
				provider_updated_at = EXCLUDED.provider_updated_at, is_present = true, updated_at = EXCLUDED.updated_at`,
			id, c.ExternalID, c.Name, c.Description, c.GroupName, c.Status, c.Position, c.UpdatedAt, now); err != nil {
			return fmt.Errorf("upsert external component: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE external_components SET is_present = false, updated_at = $3
		WHERE source_id = $1 AND is_present AND NOT (external_id = ANY($2::text[]))`, id, present, now); err != nil {
		return fmt.Errorf("mark missing external components: %w", err)
	}
	return nil
}

// PublicSource is a provider as shown on the public page.
type PublicSource struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	PageURL       string            `json:"page_url"`
	Status        string            `json:"status"`
	Stale         bool              `json:"stale"`
	LastSuccessAt *time.Time        `json:"last_success_at"`
	Components    []PublicComponent `json:"components"`
	Events        []PublicEvent     `json:"events"`
}

// PublicComponent is a selected provider component.
type PublicComponent struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	GroupName *string `json:"group_name"`
	Status    string  `json:"status"`
}

// PublicEvent is a provider event affecting selected components.
type PublicEvent struct {
	Kind   string  `json:"kind"`
	Name   string  `json:"name"`
	Status string  `json:"status"`
	Impact string  `json:"impact"`
	URL    *string `json:"url"`
}

// Public returns active sources with at least one selected component. When
// a snapshot is stale its statuses are reported as unknown rather than the
// last known value, so outdated data is never presented as current.
func (s *Service) Public(ctx context.Context, q database.Querier) ([]PublicSource, error) {
	sources, err := s.List(ctx, q, true)
	if err != nil {
		return nil, err
	}
	out := []PublicSource{}
	for _, src := range sources {
		comps, err := Components(ctx, q, src.ID, true)
		if err != nil {
			return nil, err
		}
		if len(comps) == 0 {
			continue
		}
		selected := map[string]bool{}
		ps := PublicSource{ID: src.ID, Name: src.Name, PageURL: src.BaseURL, Stale: src.Stale, LastSuccessAt: src.LastSuccessAt,
			Components: make([]PublicComponent, 0, len(comps)), Events: []PublicEvent{}}
		statuses := make([]string, 0, len(comps))
		for _, c := range comps {
			selected[c.ExternalID] = true
			st := c.Status
			if src.Stale {
				st = "unknown"
			}
			ps.Components = append(ps.Components, PublicComponent{ID: c.ID, Name: c.Name, GroupName: c.GroupName, Status: st})
			statuses = append(statuses, st)
		}
		ps.Status = worst(statuses)
		if !src.Stale {
			for _, e := range src.events {
				if !affectsSelection(e, selected) {
					continue
				}
				pe := PublicEvent{Kind: e.Kind, Name: e.Name, Status: e.Status, Impact: e.Impact}
				if e.URL != "" {
					u := e.URL
					pe.URL = &u
				}
				ps.Events = append(ps.Events, pe)
			}
		}
		out = append(out, ps)
	}
	return out, nil
}

// affectsSelection keeps events tied to a selected component, and events
// that list no components (provider-wide).
func affectsSelection(e Event, selected map[string]bool) bool {
	if len(e.ComponentIDs) == 0 {
		return true
	}
	for _, id := range e.ComponentIDs {
		if selected[id] {
			return true
		}
	}
	return false
}

// worst returns the most severe status; any unknown makes the result unknown
// unless something is known to be worse than operational.
func worst(statuses []string) string {
	result := statuspolicy.Operational
	unknown := false
	for _, st := range statuses {
		if st == "unknown" {
			unknown = true
			continue
		}
		result = statuspolicy.Worse(result, st)
	}
	if unknown && result == statuspolicy.Operational {
		return statuspolicy.Unknown
	}
	return result
}
