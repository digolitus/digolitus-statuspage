// Package presence counts who is looking at the status page and the admin
// area right now (ADR 0018).
//
// Each open browser tab sends a heartbeat about once a minute while it is
// visible. A tab counts as present for Window after its last heartbeat. Public
// visitors are anonymous: the tab sends a random id that the server stores
// only as a SHA-256, with no IP address, user agent or cookie. Admin tabs are
// tied to the signed-in user so administrators can see who else is working.
package presence

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/clock"
)

const (
	// Window is how long a tab counts as present after its last heartbeat:
	// two heartbeat intervals, so one lost request does not drop a visitor.
	Window = 2 * time.Minute
	// HeartbeatInterval is what clients are expected to use.
	HeartbeatInterval = time.Minute
	// keepHeartbeats bounds how long a tab row exists after its last
	// heartbeat; keepSamples bounds the per-minute counts.
	keepHeartbeats = 10 * time.Minute
	keepSamples    = 48 * time.Hour
	// HistoryMinutes is the length of the chart returned to the admin.
	HistoryMinutes = 60

	// New public tabs are rate limited per process so a script cannot fill
	// the table or inflate the count without bound. Heartbeats of tabs
	// already known are never limited.
	newTabsPerSecond = 10
	newTabsBurst     = 600

	maxAdminArea = 120
)

// Public areas, in display order.
const (
	AreaStatus        = "status"
	AreaIncident      = "incident"
	AreaMaintenance   = "maintenance"
	AreaAnnouncements = "announcements"
	AreaSubscriptions = "subscriptions"
	AreaOther         = "other"
)

// PublicAreas lists the accepted public area codes in display order.
var PublicAreas = []string{AreaStatus, AreaIncident, AreaMaintenance, AreaAnnouncements, AreaSubscriptions, AreaOther}

var tabIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)

// Snapshot is what the admin sees.
type Snapshot struct {
	WindowSeconds int           `json:"window_seconds"`
	Public        PublicSummary `json:"public"`
	Admin         AdminSummary  `json:"admin"`
	History       []Sample      `json:"history"`
	PeakToday     *Peak         `json:"peak_today"`
	GeneratedAt   time.Time     `json:"generated_at"`
}

// PublicSummary counts anonymous visitors of the public page.
type PublicSummary struct {
	Total int         `json:"total"`
	Areas []AreaCount `json:"areas"`
}

// AreaCount is the number of public tabs on one area.
type AreaCount struct {
	Area  string `json:"area"`
	Count int    `json:"count"`
}

// AdminSummary lists signed-in users with the admin open.
type AdminSummary struct {
	Total int         `json:"total"`
	Users []AdminUser `json:"users"`
}

// AdminUser is one user with at least one admin tab present. With several
// tabs, the most recent visible one is shown.
type AdminUser struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Area       string    `json:"area"`
	Hidden     bool      `json:"hidden"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

// Sample is the count of one minute.
type Sample struct {
	Minute time.Time `json:"minute"`
	Public int       `json:"public"`
	Admin  int       `json:"admin"`
}

// Peak is the highest public count of the day.
type Peak struct {
	Count int       `json:"count"`
	At    time.Time `json:"at"`
}

// Service records heartbeats and builds snapshots.
type Service struct {
	pool    *pgxpool.Pool
	now     clock.Clock
	limiter *bucket
}

// NewService creates a Service.
func NewService(pool *pgxpool.Pool, now clock.Clock) *Service {
	return &Service{pool: pool, now: now, limiter: newBucket(newTabsPerSecond, newTabsBurst, now)}
}

// PublicBeat is the body of a public heartbeat.
type PublicBeat struct {
	TabID string `json:"tab_id"`
	Area  string `json:"area"`
}

// AdminBeat is the body of an admin heartbeat.
type AdminBeat struct {
	TabID  string `json:"tab_id"`
	Area   string `json:"area"`
	Hidden bool   `json:"hidden"`
}

func validTabID(v *apperror.Validator, id string) {
	v.Check(tabIDPattern.MatchString(id), "tab_id", "Informe de 16 a 64 caracteres entre letras, números, - e _.")
}

func hashTab(id string) []byte {
	sum := sha256.Sum256([]byte(id))
	return sum[:]
}

// Public records a heartbeat of an anonymous visitor.
func (s *Service) Public(ctx context.Context, b PublicBeat) error {
	var v apperror.Validator
	validTabID(&v, b.TabID)
	known := false
	for _, a := range PublicAreas {
		known = known || a == b.Area
	}
	v.Check(known, "area", "Área desconhecida.")
	if err := v.Err(); err != nil {
		return err
	}
	now := s.now()
	h := hashTab(b.TabID)
	// Common case: the tab is already known.
	tag, err := s.pool.Exec(ctx, `UPDATE presence_heartbeats SET area = $2, last_seen_at = $3
		WHERE tab_hash = $1 AND kind = 'public'`, h, b.Area, now)
	if err != nil {
		return fmt.Errorf("refresh public heartbeat: %w", err)
	}
	if tag.RowsAffected() > 0 || !s.limiter.allow() {
		// Over the limit the heartbeat is dropped silently: the client has
		// nothing useful to do with an error.
		return nil
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO presence_heartbeats (tab_hash, kind, area, last_seen_at)
		VALUES ($1, 'public', $2, $3) ON CONFLICT (tab_hash) DO NOTHING`, h, b.Area, now); err != nil {
		return fmt.Errorf("insert public heartbeat: %w", err)
	}
	return nil
}

// Admin records a heartbeat of a signed-in user.
func (s *Service) Admin(ctx context.Context, userID string, b AdminBeat) error {
	var v apperror.Validator
	validTabID(&v, b.TabID)
	area := cleanLabel(b.Area)
	v.Check(area != "" && utf8.RuneCountInString(area) <= maxAdminArea, "area", fmt.Sprintf("Informe de 1 a %d caracteres.", maxAdminArea))
	if err := v.Err(); err != nil {
		return err
	}
	// The WHERE keeps a tab id from moving between users.
	if _, err := s.pool.Exec(ctx, `INSERT INTO presence_heartbeats (tab_hash, kind, area, user_id, hidden, last_seen_at)
		VALUES ($1, 'admin', $2, $3, $4, $5)
		ON CONFLICT (tab_hash) DO UPDATE SET area = EXCLUDED.area, hidden = EXCLUDED.hidden, last_seen_at = EXCLUDED.last_seen_at
		WHERE presence_heartbeats.user_id = EXCLUDED.user_id`,
		hashTab(b.TabID), area, userID, b.Hidden, s.now()); err != nil {
		return fmt.Errorf("record admin heartbeat: %w", err)
	}
	return nil
}

// cleanLabel trims the label and folds control characters and runs of
// whitespace into single spaces.
func cleanLabel(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }), " ")
}

// Snapshot returns who is present now, the last hour of samples and the peak
// of the current day in loc.
func (s *Service) Snapshot(ctx context.Context, loc *time.Location) (Snapshot, error) {
	now := s.now()
	since := now.Add(-Window)
	out := Snapshot{WindowSeconds: int(Window / time.Second), GeneratedAt: now,
		Public: PublicSummary{Areas: make([]AreaCount, 0, len(PublicAreas))},
		Admin:  AdminSummary{Users: []AdminUser{}}, History: []Sample{}}
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		counts := map[string]int{}
		rows, err := tx.Query(ctx, `SELECT area, count(*) FROM presence_heartbeats
			WHERE kind = 'public' AND last_seen_at >= $1 GROUP BY area`, since)
		if err != nil {
			return fmt.Errorf("count public: %w", err)
		}
		for rows.Next() {
			var area string
			var n int
			if err := rows.Scan(&area, &n); err != nil {
				rows.Close()
				return err
			}
			counts[area] = n
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, a := range PublicAreas {
			out.Public.Areas = append(out.Public.Areas, AreaCount{Area: a, Count: counts[a]})
			out.Public.Total += counts[a]
		}

		rows, err = tx.Query(ctx, `SELECT DISTINCT ON (u.id) u.id::text, u.name, h.area, h.hidden, h.last_seen_at
			FROM presence_heartbeats h JOIN users u ON u.id = h.user_id
			WHERE h.kind = 'admin' AND h.last_seen_at >= $1 AND u.is_active
			ORDER BY u.id, h.hidden, h.last_seen_at DESC`, since)
		if err != nil {
			return fmt.Errorf("list admin users: %w", err)
		}
		users, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (AdminUser, error) {
			var u AdminUser
			err := r.Scan(&u.ID, &u.Name, &u.Area, &u.Hidden, &u.LastSeenAt)
			return u, err
		})
		if err != nil {
			return err
		}
		// Visible first, then by name.
		sort.SliceStable(users, func(i, j int) bool {
			if users[i].Hidden != users[j].Hidden {
				return !users[i].Hidden
			}
			return strings.ToLower(users[i].Name) < strings.ToLower(users[j].Name)
		})
		out.Admin.Users = users
		out.Admin.Total = len(users)

		rows, err = tx.Query(ctx, `SELECT minute, public_count, admin_count FROM presence_samples
			WHERE minute > $1 ORDER BY minute`, now.Add(-HistoryMinutes*time.Minute))
		if err != nil {
			return fmt.Errorf("list samples: %w", err)
		}
		out.History, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (Sample, error) {
			var x Sample
			err := r.Scan(&x.Minute, &x.Public, &x.Admin)
			return x, err
		})
		if err != nil {
			return err
		}

		y, m, d := now.In(loc).Date()
		dayStart := time.Date(y, m, d, 0, 0, 0, 0, loc)
		var p Peak
		err = tx.QueryRow(ctx, `SELECT public_count, minute FROM presence_samples WHERE minute >= $1
			ORDER BY public_count DESC, minute DESC LIMIT 1`, dayStart).Scan(&p.Count, &p.At)
		switch {
		case err == nil:
			out.PeakToday = &p
		case err != pgx.ErrNoRows:
			return fmt.Errorf("peak of the day: %w", err)
		}
		return nil
	})
	if err != nil {
		return Snapshot{}, err
	}
	// The current count may be above the last sample.
	if out.Public.Total > 0 && (out.PeakToday == nil || out.Public.Total > out.PeakToday.Count) {
		out.PeakToday = &Peak{Count: out.Public.Total, At: now}
	}
	for i := range out.History {
		out.History[i].Minute = out.History[i].Minute.UTC()
	}
	if out.PeakToday != nil {
		out.PeakToday.At = out.PeakToday.At.UTC()
	}
	return out, nil
}

// Tick writes the sample of the current minute and deletes expired rows. It
// is idempotent, so every backend replica may run it: replicas share the
// table and keep the highest count seen in the minute.
func (s *Service) Tick(ctx context.Context) error {
	now := s.now()
	since := now.Add(-Window)
	if _, err := s.pool.Exec(ctx, `INSERT INTO presence_samples (minute, public_count, admin_count)
		SELECT $1,
			count(*) FILTER (WHERE kind = 'public'),
			count(DISTINCT user_id) FILTER (WHERE kind = 'admin')
		FROM presence_heartbeats WHERE last_seen_at >= $2
		ON CONFLICT (minute) DO UPDATE SET
			public_count = GREATEST(presence_samples.public_count, EXCLUDED.public_count),
			admin_count = GREATEST(presence_samples.admin_count, EXCLUDED.admin_count)`,
		now.Truncate(time.Minute), since); err != nil {
		return fmt.Errorf("write presence sample: %w", err)
	}
	if _, err := s.pool.Exec(ctx, "DELETE FROM presence_heartbeats WHERE last_seen_at < $1", now.Add(-keepHeartbeats)); err != nil {
		return fmt.Errorf("prune heartbeats: %w", err)
	}
	if _, err := s.pool.Exec(ctx, "DELETE FROM presence_samples WHERE minute < $1", now.Add(-keepSamples)); err != nil {
		return fmt.Errorf("prune samples: %w", err)
	}
	return nil
}

// bucket is a small token bucket; it is per process on purpose (no shared
// state, no stored IP addresses).
type bucket struct {
	mu     sync.Mutex
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
	now    clock.Clock
}

func newBucket(rate, burst float64, now clock.Clock) *bucket {
	return &bucket{rate: rate, burst: burst, tokens: burst, now: now}
}

func (b *bucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	if !b.last.IsZero() {
		if dt := now.Sub(b.last).Seconds(); dt > 0 {
			b.tokens = min(b.burst, b.tokens+dt*b.rate)
		}
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
