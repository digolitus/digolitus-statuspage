// Package audit persists the operational audit log. Records are written in
// the same transaction as the change they describe.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
)

// Resource types recorded in the log.
const (
	ResourceComponent            = "component"
	ResourceComponentGroup       = "component_group"
	ResourceExternalSource       = "external_source"
	ResourceIncident             = "incident"
	ResourceMaintenance          = "maintenance"
	ResourceIncidentTemplate     = "incident_template"
	ResourceMaintenanceTemplate  = "maintenance_template"
	ResourceAnnouncement         = "announcement"
	ResourceAnnouncementCategory = "announcement_category"
	ResourceTeam                 = "team"
	ResourceUser                 = "user"
	ResourceRole                 = "role"
)

// ResourceTypes lists every resource type, for filter validation.
var ResourceTypes = []string{ResourceComponent, ResourceComponentGroup, ResourceIncident, ResourceMaintenance, ResourceIncidentTemplate, ResourceMaintenanceTemplate, ResourceExternalSource,
	ResourceAnnouncement, ResourceAnnouncementCategory, ResourceTeam, ResourceUser, ResourceRole}

// Changes holds the relevant before/after values of an operation.
type Changes struct {
	Before any `json:"before,omitempty"`
	After  any `json:"after,omitempty"`
}

// Diff returns the fields that differ between two JSON-serialisable values,
// ignoring bookkeeping fields. It keeps audit payloads small and relevant.
func Diff(before, after any) Changes {
	b, a := toMap(before), toMap(after)
	outB, outA := map[string]any{}, map[string]any{}
	for k, av := range a {
		if k == "updated_at" || k == "created_at" {
			continue
		}
		bv, ok := b[k]
		if !ok || !jsonEqual(av, bv) {
			outA[k] = av
			if ok {
				outB[k] = bv
			}
		}
	}
	return Changes{Before: outB, After: outA}
}

func toMap(v any) map[string]any {
	raw, _ := json.Marshal(v)
	m := map[string]any{}
	_ = json.Unmarshal(raw, &m)
	return m
}

func jsonEqual(a, b any) bool {
	ra, _ := json.Marshal(a)
	rb, _ := json.Marshal(b)
	return string(ra) == string(rb)
}

// Record writes an audit event using q, which should be the transaction of
// the change being recorded. Source and request ID come from ctx.
func Record(ctx context.Context, q database.Querier, resourceType, resourceID, action string, changes Changes, at time.Time) error {
	payload, err := json.Marshal(changes)
	if err != nil {
		return fmt.Errorf("encode audit changes: %w", err)
	}
	var requestID, actorID, actorName *string
	if id := httpx.RequestID(ctx); id != "" {
		requestID = &id
	}
	if a, ok := httpx.ActorFrom(ctx); ok {
		actorID, actorName = &a.ID, &a.Name
	}
	_, err = q.Exec(ctx, `INSERT INTO audit_events (occurred_at, resource_type, resource_id, action, source, request_id, changes, actor_id, actor_name)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		at, resourceType, resourceID, action, httpx.Source(ctx), requestID, payload, actorID, actorName)
	if err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}

// Event is an audit record as exposed by the API.
type Event struct {
	ID           int64           `json:"id"`
	OccurredAt   time.Time       `json:"occurred_at"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	Action       string          `json:"action"`
	Source       string          `json:"source"`
	RequestID    *string         `json:"request_id"`
	Actor        *ActorRef       `json:"actor"`
	Changes      json.RawMessage `json:"changes"`
}

// ActorRef identifies who made a change (null for changes made before
// authentication existed or by the system).
type ActorRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Filter narrows an audit listing.
type Filter struct {
	ResourceType string
	ResourceID   string
	Action       string
	Desc         bool
}

// Store reads audit events.
type Store struct{ pool *pgxpool.Pool }

// NewStore creates a Store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// List returns a page of events, newest first unless Desc is false.
func (s *Store) List(ctx context.Context, f Filter, page httpx.Page) ([]Event, int, error) {
	where := "WHERE ($1 = '' OR resource_type = $1) AND ($2 = '' OR resource_id = NULLIF($2, '')::uuid) AND ($3 = '' OR action = $3)"
	var total int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events "+where, f.ResourceType, f.ResourceID, f.Action).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit events: %w", err)
	}
	order := "occurred_at DESC, id DESC"
	if !f.Desc {
		order = "occurred_at ASC, id ASC"
	}
	// #nosec G202 -- order is one of two constant strings; filters are bound parameters.
	rows, err := s.pool.Query(ctx, `SELECT id, occurred_at, resource_type, resource_id, action, source, request_id, changes, actor_id, actor_name
		FROM audit_events `+where+` ORDER BY `+order+` LIMIT $4 OFFSET $5`,
		f.ResourceType, f.ResourceID, f.Action, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var changes []byte
		var actorID, actorName *string
		if err := rows.Scan(&e.ID, &e.OccurredAt, &e.ResourceType, &e.ResourceID, &e.Action, &e.Source, &e.RequestID, &changes, &actorID, &actorName); err != nil {
			return nil, 0, err
		}
		if actorID != nil {
			e.Actor = &ActorRef{ID: *actorID}
			if actorName != nil {
				e.Actor.Name = *actorName
			}
		}
		e.OccurredAt = e.OccurredAt.UTC()
		e.Changes = changes
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// Handler exposes the audit log over HTTP.
type Handler struct{ store *Store }

// NewHandler creates a Handler.
func NewHandler(store *Store) *Handler { return &Handler{store: store} }

// Register mounts admin routes.
func (h *Handler) Register(rt *httpx.Router, admin func(http.HandlerFunc) http.HandlerFunc) {
	rt.Handle(http.MethodGet, "/api/v1/admin/audit-events", admin(h.list))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := httpx.NewQuery(r.URL.Query())
	page := q.Page()
	sort := q.Sort([]string{"occurred_at"}, httpx.Sort{Field: "occurred_at", Desc: true})
	f := Filter{
		ResourceType: q.Enum("resource_type", ResourceTypes...),
		ResourceID:   q.UUID("resource_id"),
		Action:       r.URL.Query().Get("action"),
		Desc:         sort.Desc,
	}
	err := q.Err()
	if len(f.Action) > 64 {
		err = &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
			Fields: []apperror.FieldError{{Field: "action", Message: "Informe no máximo 64 caracteres."}}}
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	events, total, err := h.store.List(r.Context(), f, page)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewList(events, page, total))
}
