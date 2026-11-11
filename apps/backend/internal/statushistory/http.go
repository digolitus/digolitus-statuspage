package statushistory

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/clock"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
)

// ComponentUptime is the daily bar of one public component.
type ComponentUptime struct {
	ID            string   `json:"id"`
	UptimePercent *float64 `json:"uptime_percent"`
	Days          []Day    `json:"days"`
}

// Uptime is the response of GET /api/v1/status/uptime.
type Uptime struct {
	Days        int               `json:"days"`
	Timezone    string            `json:"timezone"`
	From        time.Time         `json:"from"`
	To          time.Time         `json:"to"`
	Components  []ComponentUptime `json:"components"`
	GeneratedAt time.Time         `json:"generated_at"`
}

// Entry is a recorded change as listed in the admin area.
type Entry struct {
	ID             int64   `json:"id"`
	ComponentID    string  `json:"component_id"`
	ComponentName  string  `json:"component_name"`
	Status         *string `json:"status"`
	PreviousStatus *string `json:"previous_status"`
	CauseType      string  `json:"cause_type"`
	CauseID        *string `json:"cause_id"`
	// CauseName is the incident or maintenance title, when the cause has one.
	CauseName *string   `json:"cause_name"`
	Actor     *Actor    `json:"actor"`
	ChangedAt time.Time `json:"changed_at"`
}

// Actor identifies who made a change.
type Actor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Service reads the history.
type Service struct {
	pool *pgxpool.Pool
	now  clock.Clock
}

// NewService creates a Service.
func NewService(pool *pgxpool.Pool, now clock.Clock) *Service {
	return &Service{pool: pool, now: now}
}

// Uptime builds the daily bars of every public, active component for the
// last n days in loc, in one read-only snapshot.
func (s *Service) Uptime(ctx context.Context, n int, loc *time.Location) (Uptime, error) {
	now := s.now()
	starts, end := DayStarts(now, n, loc)
	from := starts[0]
	out := Uptime{Days: n, Timezone: loc.String(), From: from.UTC(), To: now, GeneratedAt: now, Components: []ComponentUptime{}}
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		var ids []string
		if err := tx.QueryRow(ctx, `SELECT coalesce(array_agg(id::text ORDER BY display_order, name, id), '{}')
			FROM components WHERE is_active AND is_public`).Scan(&ids); err != nil {
			return fmt.Errorf("list public components: %w", err)
		}
		if len(ids) == 0 {
			return nil
		}
		// Changes inside the range plus, per component, the last one before it.
		rows, err := tx.Query(ctx, `SELECT component_id::text, status, changed_at FROM (
				SELECT component_id, status, changed_at, id FROM component_status_changes
				WHERE component_id = ANY($1::uuid[]) AND changed_at >= $2
				UNION ALL
				SELECT c.cid, h.status, h.changed_at, h.id FROM unnest($1::uuid[]) AS c(cid)
				JOIN LATERAL (SELECT status, changed_at, id FROM component_status_changes
					WHERE component_id = c.cid AND changed_at < $2 ORDER BY changed_at DESC, id DESC LIMIT 1) h ON true
			) x ORDER BY component_id, changed_at, id`, ids, from)
		if err != nil {
			return fmt.Errorf("load status history: %w", err)
		}
		changes := map[string][]Change{}
		for rows.Next() {
			var id string
			var c Change
			if err := rows.Scan(&id, &c.Status, &c.At); err != nil {
				rows.Close()
				return err
			}
			changes[id] = append(changes[id], c)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		type incidentSpan struct {
			ref        IncidentRef
			start, end time.Time
		}
		rows, err = tx.Query(ctx, `SELECT ic.component_id::text, i.id::text, i.title, i.impact, i.started_at, i.resolved_at
			FROM incidents i JOIN incident_components ic ON ic.incident_id = i.id
			WHERE ic.component_id = ANY($1::uuid[]) AND i.started_at < $3 AND (i.resolved_at IS NULL OR i.resolved_at > $2)
			ORDER BY i.started_at, i.id`, ids, from, now)
		if err != nil {
			return fmt.Errorf("load incidents for uptime: %w", err)
		}
		incidents := map[string][]incidentSpan{}
		for rows.Next() {
			var cid string
			var sp incidentSpan
			var resolved *time.Time
			if err := rows.Scan(&cid, &sp.ref.ID, &sp.ref.Title, &sp.ref.Impact, &sp.start, &resolved); err != nil {
				rows.Close()
				return err
			}
			sp.end = now
			if resolved != nil {
				sp.end = *resolved
			}
			incidents[cid] = append(incidents[cid], sp)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, id := range ids {
			days, total := Daily(changes[id], starts, end, now)
			for i := range days {
				d := &days[i]
				for _, sp := range incidents[id] {
					if sp.start.Before(d.end) && (sp.end.After(d.start) || sp.end.Equal(sp.start) && !sp.start.Before(d.start)) {
						d.Incidents = append(d.Incidents, sp.ref)
					}
				}
			}
			out.Components = append(out.Components, ComponentUptime{ID: id, UptimePercent: total, Days: days})
		}
		return nil
	})
	return out, err
}

// Filter narrows the admin listing.
type Filter struct {
	ComponentID string
	CauseType   string
}

// List returns recorded changes, newest first.
func (s *Service) List(ctx context.Context, f Filter, page httpx.Page) ([]Entry, int, error) {
	where := "WHERE ($1 = '' OR h.component_id = NULLIF($1, '')::uuid) AND ($2 = '' OR h.cause_type = $2)"
	var total int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM component_status_changes h "+where, f.ComponentID, f.CauseType).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count status history: %w", err)
	}
	rows, err := s.pool.Query(ctx, `SELECT h.id, h.component_id, c.name, h.status, h.previous_status, h.cause_type, h.cause_id,
			CASE h.cause_type WHEN 'incident' THEN (SELECT title FROM incidents WHERE id = h.cause_id)
				WHEN 'maintenance' THEN (SELECT title FROM maintenances WHERE id = h.cause_id) END,
			h.actor_id, h.actor_name, h.changed_at
		FROM component_status_changes h JOIN components c ON c.id = h.component_id `+where+`
		ORDER BY h.changed_at DESC, h.id DESC LIMIT $3 OFFSET $4`, f.ComponentID, f.CauseType, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list status history: %w", err)
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		var actorID, actorName *string
		if err := rows.Scan(&e.ID, &e.ComponentID, &e.ComponentName, &e.Status, &e.PreviousStatus, &e.CauseType, &e.CauseID, &e.CauseName,
			&actorID, &actorName, &e.ChangedAt); err != nil {
			return nil, 0, err
		}
		if actorID != nil {
			e.Actor = &Actor{ID: *actorID}
			if actorName != nil {
				e.Actor.Name = *actorName
			}
		}
		e.ChangedAt = e.ChangedAt.UTC()
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// Handler exposes the history over HTTP.
type Handler struct{ svc *Service }

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterPublic mounts the public uptime endpoint.
func (h *Handler) RegisterPublic(rt *httpx.Router) {
	rt.Handle(http.MethodGet, "/api/v1/status/uptime", h.uptime)
}

// RegisterAdmin mounts admin routes.
func (h *Handler) RegisterAdmin(rt *httpx.Router, admin func(http.HandlerFunc) http.HandlerFunc) {
	rt.Handle(http.MethodGet, "/api/v1/admin/status-history", admin(h.list))
}

func invalid(field, msg string) error {
	return &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
		Fields: []apperror.FieldError{{Field: field, Message: msg}}}
}

// ParseRange reads ?days (30, 60 or 90; default 90) and ?timezone (IANA;
// default UTC) of an uptime request.
func ParseRange(r *http.Request) (int, *time.Location, error) {
	n := 90
	if raw := r.URL.Query().Get("days"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || (v != 30 && v != 60 && v != 90) {
			return 0, nil, invalid("days", "Informe 30, 60 ou 90.")
		}
		n = v
	}
	loc := time.UTC
	if raw := r.URL.Query().Get("timezone"); raw != "" {
		l, err := clock.LoadZone(raw)
		if err != nil {
			return 0, nil, invalid("timezone", "Informe um timezone IANA válido (ex.: America/Sao_Paulo).")
		}
		loc = l
	}
	return n, loc, nil
}

func (h *Handler) uptime(w http.ResponseWriter, r *http.Request) {
	n, loc, err := ParseRange(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := h.svc.Uptime(r.Context(), n, loc)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := httpx.NewQuery(r.URL.Query())
	page := q.Page()
	f := Filter{ComponentID: q.UUID("component_id"), CauseType: q.Enum("cause_type", CauseTypes...)}
	if err := q.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, total, err := h.svc.List(r.Context(), f, page)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewList(items, page, total))
}
