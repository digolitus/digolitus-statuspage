package external

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/statushistory"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/statuspolicy"
)

// History of third-party sources (docs/adr/0014-status-history.md): status
// changes of provider components and the incidents and maintenances the
// provider reported, both detected when a snapshot is saved. Names are
// copied so the record survives the source being deleted.

// currentStatuses returns the stored status of each component of a source,
// keyed by provider id.
func currentStatuses(ctx context.Context, q database.Querier, sourceID string) (map[string]string, error) {
	rows, err := q.Query(ctx, "SELECT external_id, status FROM external_components WHERE source_id = $1", sourceID)
	if err != nil {
		return nil, fmt.Errorf("load external statuses: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, st string
		if err := rows.Scan(&id, &st); err != nil {
			return nil, err
		}
		out[id] = st
	}
	return out, rows.Err()
}

func recordComponentChange(ctx context.Context, q database.Querier, sourceID, componentID, sourceName, componentName, status string, previous *string, at time.Time) error {
	if _, err := q.Exec(ctx, `INSERT INTO external_component_status_changes
		(source_id, external_component_id, source_name, component_name, status, previous_status, changed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, sourceID, componentID, clip(sourceName, 120), clip(componentName, 255), status, previous, at); err != nil {
		return fmt.Errorf("record external status change: %w", err)
	}
	return nil
}

// eventKey identifies a provider event across snapshots: its provider id,
// or its name for events stored without one.
func eventKey(e Event) string {
	if e.ID != "" {
		return e.ID
	}
	return "name:" + clip(e.Name, 250)
}

// recordEvents opens or refreshes an external_events row for each event in
// the snapshot and closes the source's open events that are no longer
// reported.
func recordEvents(ctx context.Context, q database.Querier, sourceID, sourceName string, snap Snapshot, now time.Time) error {
	names := make(map[string]string, len(snap.Components))
	for _, c := range snap.Components {
		names[c.ExternalID] = c.Name
	}
	seen := make([]string, 0, len(snap.Events))
	for _, e := range snap.Events {
		if e.Kind != "incident" && e.Kind != "maintenance" {
			continue
		}
		key := eventKey(e)
		seen = append(seen, e.Kind+":"+key)
		comps := []string{}
		for _, id := range e.ComponentIDs {
			if n, ok := names[id]; ok {
				comps = append(comps, n)
			}
		}
		var url *string
		if e.URL != "" {
			u := e.URL
			url = &u
		}
		if _, err := q.Exec(ctx, `INSERT INTO external_events
			(source_id, source_name, kind, external_id, name, status, impact, url, component_names, first_seen_at, last_seen_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
			ON CONFLICT (source_id, kind, external_id) WHERE closed_at IS NULL DO UPDATE SET
				source_name = EXCLUDED.source_name, name = EXCLUDED.name, status = EXCLUDED.status, impact = EXCLUDED.impact,
				url = EXCLUDED.url, component_names = EXCLUDED.component_names, last_seen_at = EXCLUDED.last_seen_at`,
			sourceID, clip(sourceName, 120), e.Kind, key, clip(e.Name, 255), clip(e.Status, 32), clip(e.Impact, 32), url, comps, now); err != nil {
			return fmt.Errorf("record external event: %w", err)
		}
	}
	if _, err := q.Exec(ctx, `UPDATE external_events SET closed_at = $3
		WHERE source_id = $1 AND closed_at IS NULL AND NOT ((kind || ':' || external_id) = ANY($2::text[]))`, sourceID, seen, now); err != nil {
		return fmt.Errorf("close external events: %w", err)
	}
	return nil
}

// Outage is a resolved period in which a provider component was not
// operational, derived from its recorded status changes. Ongoing periods are
// not listed: they are shown as current status until they end.
type Outage struct {
	ExternalComponentID *string   `json:"external_component_id"`
	ComponentName       string    `json:"component_name"`
	WorstStatus         string    `json:"worst_status"`
	StartedAt           time.Time `json:"started_at"`
	EndedAt             time.Time `json:"ended_at"`
	DurationSeconds     int64     `json:"duration_seconds"`
}

type changeRow struct {
	componentKey string
	componentID  *string
	name         string
	status       string
	at           time.Time
}

// Outages derives resolved outages from ordered changes: a period starts
// when a component leaves "operational" and ends when it returns to it;
// "unknown" neither starts nor ends one. The result is newest first.
func outages(rows []changeRow) []Outage {
	cur := map[string]*Outage{}
	var out []Outage
	for _, r := range rows {
		o := cur[r.componentKey]
		switch {
		case r.status == "unknown":
		case r.status == statuspolicy.Operational:
			if o != nil {
				o.EndedAt = r.at.UTC()
				o.DurationSeconds = int64(o.EndedAt.Sub(o.StartedAt) / time.Second)
				out = append(out, *o)
				delete(cur, r.componentKey)
			}
		case o == nil:
			cur[r.componentKey] = &Outage{ExternalComponentID: r.componentID, ComponentName: r.name, WorstStatus: r.status, StartedAt: r.at.UTC()}
		default:
			o.WorstStatus = statuspolicy.Worse(o.WorstStatus, r.status)
			o.ComponentName = r.name
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].EndedAt.After(out[j].EndedAt) })
	return out
}

// EventRecord is a provider incident or maintenance as recorded.
type EventRecord struct {
	ID             string     `json:"id"`
	Kind           string     `json:"kind"`
	Name           string     `json:"name"`
	Status         string     `json:"status"`
	Impact         string     `json:"impact"`
	URL            *string    `json:"url"`
	ComponentNames []string   `json:"component_names"`
	FirstSeenAt    time.Time  `json:"first_seen_at"`
	LastSeenAt     time.Time  `json:"last_seen_at"`
	ClosedAt       *time.Time `json:"closed_at"`
}

// Outages lists a source's resolved outages, newest first; with
// onlySelected, only those of components currently published.
func (s *Service) Outages(ctx context.Context, sourceID string, onlySelected bool, page httpx.Page) ([]Outage, int, error) {
	if _, err := s.Get(ctx, s.pool, sourceID); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT coalesce(h.external_component_id::text, h.component_name), h.external_component_id, h.component_name, h.status, h.changed_at
		FROM external_component_status_changes h
		WHERE h.source_id = $1 AND ($2 = false OR EXISTS (SELECT 1 FROM external_components c
			WHERE c.id = h.external_component_id AND c.is_selected AND c.is_present))
		ORDER BY h.changed_at, h.id`, sourceID, onlySelected)
	if err != nil {
		return nil, 0, fmt.Errorf("list external status history: %w", err)
	}
	var changes []changeRow
	for rows.Next() {
		var c changeRow
		if err := rows.Scan(&c.componentKey, &c.componentID, &c.name, &c.status, &c.at); err != nil {
			rows.Close()
			return nil, 0, err
		}
		changes = append(changes, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	all := outages(changes)
	start := min(page.Offset, len(all))
	end := min(start+page.Limit, len(all))
	return all[start:end], len(all), nil
}

// EventHistory lists the incidents and maintenances reported by a source:
// resolved ones (the history, by end, newest first) or, with open, those
// still in progress.
func (s *Service) EventHistory(ctx context.Context, sourceID string, open bool, page httpx.Page) ([]EventRecord, int, error) {
	if _, err := s.Get(ctx, s.pool, sourceID); err != nil {
		return nil, 0, err
	}
	where := "WHERE source_id = $1 AND (closed_at IS NULL) = $2"
	var total int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM external_events "+where, sourceID, open).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count external events: %w", err)
	}
	rows, err := s.pool.Query(ctx, `SELECT id, kind, name, status, impact, url, component_names, first_seen_at, last_seen_at, closed_at
		FROM external_events `+where+` ORDER BY coalesce(closed_at, first_seen_at) DESC, id LIMIT $3 OFFSET $4`, sourceID, open, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list external events: %w", err)
	}
	defer rows.Close()
	var out []EventRecord
	for rows.Next() {
		var e EventRecord
		if err := rows.Scan(&e.ID, &e.Kind, &e.Name, &e.Status, &e.Impact, &e.URL, &e.ComponentNames, &e.FirstSeenAt, &e.LastSeenAt, &e.ClosedAt); err != nil {
			return nil, 0, err
		}
		e.FirstSeenAt, e.LastSeenAt, e.ClosedAt = e.FirstSeenAt.UTC(), e.LastSeenAt.UTC(), utc(e.ClosedAt)
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// ComponentUptime is the daily bar of a published provider component.
type ComponentUptime struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	UptimePercent *float64            `json:"uptime_percent"`
	Days          []statushistory.Day `json:"days"`
}

// Uptime is the response of the source uptime endpoint.
type Uptime struct {
	Days       int               `json:"days"`
	Timezone   string            `json:"timezone"`
	From       time.Time         `json:"from"`
	To         time.Time         `json:"to"`
	Components []ComponentUptime `json:"components"`
}

// Uptime builds daily bars for the published components of a source from
// the statuses the provider reported. "unknown" counts as no data; periods the poller could not read are not recorded and
// keep the last known status.
func (s *Service) Uptime(ctx context.Context, sourceID string, n int, loc *time.Location) (Uptime, error) {
	now := s.now()
	starts, end := statushistory.DayStarts(now, n, loc)
	from := starts[0]
	out := Uptime{Days: n, Timezone: loc.String(), From: from.UTC(), To: now, Components: []ComponentUptime{}}
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if _, err := s.Get(ctx, tx, sourceID); err != nil {
			return err
		}
		comps, err := Components(ctx, tx, sourceID, true)
		if err != nil {
			return err
		}
		if len(comps) == 0 {
			return nil
		}
		ids := make([]string, len(comps))
		for i, c := range comps {
			ids[i] = c.ID
		}
		rows, err := tx.Query(ctx, `SELECT external_component_id::text, status, changed_at FROM (
				SELECT external_component_id, status, changed_at, id FROM external_component_status_changes
				WHERE external_component_id = ANY($1::uuid[]) AND changed_at >= $2
				UNION ALL
				SELECT c.cid, h.status, h.changed_at, h.id FROM unnest($1::uuid[]) AS c(cid)
				JOIN LATERAL (SELECT status, changed_at, id FROM external_component_status_changes
					WHERE external_component_id = c.cid AND changed_at < $2 ORDER BY changed_at DESC, id DESC LIMIT 1) h ON true
			) x ORDER BY external_component_id, changed_at, id`, ids, from)
		if err != nil {
			return fmt.Errorf("load external status history: %w", err)
		}
		changes := map[string][]statushistory.Change{}
		for rows.Next() {
			var id, st string
			var at time.Time
			if err := rows.Scan(&id, &st, &at); err != nil {
				rows.Close()
				return err
			}
			c := statushistory.Change{Status: &st, At: at}
			if st == "unknown" {
				c.Status = nil // the provider did not say: no data
			}
			changes[id] = append(changes[id], c)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, c := range comps {
			days, total := statushistory.Daily(changes[c.ID], starts, end, now)
			out.Components = append(out.Components, ComponentUptime{ID: c.ID, Name: c.Name, UptimePercent: total, Days: days})
		}
		return nil
	})
	return out, err
}

func (h *Handler) outages(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "external_source")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	q := httpx.NewQuery(r.URL.Query())
	page := q.Page()
	selected := q.Bool("only_selected")
	if err := q.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, total, err := h.svc.Outages(r.Context(), id, selected != nil && *selected, page)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewList(items, page, total))
}

func (h *Handler) eventHistory(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "external_source")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	q := httpx.NewQuery(r.URL.Query())
	page := q.Page()
	state := q.Enum("state", "resolved", "open")
	if err := q.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, total, err := h.svc.EventHistory(r.Context(), id, state == "open", page)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewList(items, page, total))
}

func (h *Handler) uptime(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "external_source")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	n, loc, err := statushistory.ParseRange(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := h.svc.Uptime(r.Context(), id, n, loc)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
