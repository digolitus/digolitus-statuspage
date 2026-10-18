package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/component"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/notify"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
)

// Delivery policy: up to maxAttempts tries with growing delays; the lease
// keeps a message claimed while it is being sent (other workers skip it).
const (
	batchSize   = 20
	maxAttempts = 6
	lease       = 2 * time.Minute
	sendTimeout = 20 * time.Second
)

var backoff = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, 3 * time.Hour}

// Worker turns events into deliveries. Several processes may run it: rows
// are claimed with FOR UPDATE SKIP LOCKED.
type Worker struct {
	svc *Service
	log *slog.Logger
}

// NewWorker builds the worker.
func NewWorker(svc *Service, log *slog.Logger) *Worker { return &Worker{svc: svc, log: log} }

// Run loops until ctx ends.
func (w *Worker) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	runs := 0
	for {
		w.RunOnce(ctx)
		if runs%360 == 0 {
			w.purge(ctx)
		}
		runs++
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// RunOnce performs one cycle (exported for tests).
func (w *Worker) RunOnce(ctx context.Context) {
	if err := w.announcementEvents(ctx); err != nil {
		w.log.Warn("notifications: announcement events failed", "error", err)
	}
	if err := w.fanOut(ctx); err != nil {
		w.log.Warn("notifications: fan-out failed", "error", err)
	}
	if err := w.deliver(ctx); err != nil {
		w.log.Warn("notifications: delivery failed", "error", err)
	}
}

// announcementEvents records events for announcements whose publication
// time has arrived (immediate or scheduled), once.
func (w *Worker) announcementEvents(ctx context.Context) error {
	now := w.svc.now()
	return database.InTx(ctx, w.svc.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, title, body, published_at FROM announcements
			WHERE status = 'published' AND published_at <= $1 AND notified_at IS NULL
			ORDER BY published_at FOR UPDATE SKIP LOCKED LIMIT $2`, now, batchSize)
		if err != nil {
			return err
		}
		type ann struct {
			id, title, body string
			at              time.Time
		}
		var due []ann
		for rows.Next() {
			var a ann
			if err := rows.Scan(&a.id, &a.title, &a.body, &a.at); err != nil {
				rows.Close()
				return err
			}
			due = append(due, a)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, a := range due {
			refs, err := component.RefsByOwner(ctx, tx, component.OwnerAnnouncement, []string{a.id})
			if err != nil {
				return err
			}
			comps := []notify.Component{}
			for _, r := range component.PublicRefs(refs[a.id]) {
				comps = append(comps, notify.Component{ID: r.ID, Name: r.Name})
			}
			e := notify.Event{Kind: notify.AnnouncementPosted, ResourceType: "announcement", ResourceID: a.id, Title: a.title,
				Message: plainText(a.body, 4000), Components: comps, Path: "/announcements/" + a.id, OccurredAt: a.at}
			if err := notify.Record(ctx, tx, e); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "UPDATE announcements SET notified_at = $2 WHERE id = $1", a.id, now); err != nil {
				return err
			}
		}
		return nil
	})
}

// plainText strips the light formatting of announcements for messages.
func plainText(s string, max int) string {
	r := strings.NewReplacer("**", "")
	s = r.Replace(s)
	if utf8.RuneCountInString(s) > max {
		s = string([]rune(s)[:max-1]) + "…"
	}
	return s
}

// fanOut creates one outbox message per matching active subscription.
func (w *Worker) fanOut(ctx context.Context) error {
	now := w.svc.now()
	return database.InTx(ctx, w.svc.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, component_ids::text[] FROM notification_events WHERE dispatched_at IS NULL
			ORDER BY id FOR UPDATE SKIP LOCKED LIMIT $1`, batchSize)
		if err != nil {
			return err
		}
		type ev struct {
			id    int64
			comps []string
		}
		var evs []ev
		for rows.Next() {
			var e ev
			if err := rows.Scan(&e.id, &e.comps); err != nil {
				rows.Close()
				return err
			}
			evs = append(evs, e)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, e := range evs {
			// Subscribers of all components, or of any affected one. An event
			// without public components concerns the whole platform.
			if _, err := tx.Exec(ctx, `INSERT INTO outbox_messages (subscription_id, kind, event_id, next_attempt_at, created_at)
				SELECT s.id, 'event', $1, $2, $2 FROM subscriptions s
				WHERE s.status = 'active' AND (s.all_components OR cardinality($3::uuid[]) = 0 OR EXISTS (
					SELECT 1 FROM subscription_components sc WHERE sc.subscription_id = s.id AND sc.component_id = ANY($3::uuid[])))
				ON CONFLICT (event_id, subscription_id) DO NOTHING`, e.id, now, e.comps); err != nil {
				return fmt.Errorf("fan out event %d: %w", e.id, err)
			}
			if _, err := tx.Exec(ctx, "UPDATE notification_events SET dispatched_at = $2 WHERE id = $1", e.id, now); err != nil {
				return err
			}
		}
		return nil
	})
}

type claimed struct {
	id       int64
	subID    string
	kind     string
	eventID  *int64
	attempts int
}

// deliver claims due messages, sends them outside any transaction and
// records the outcome.
func (w *Worker) deliver(ctx context.Context) error {
	now := w.svc.now()
	rows, err := w.svc.pool.Query(ctx, `UPDATE outbox_messages SET next_attempt_at = $1, attempts = attempts + 1
		WHERE id IN (SELECT id FROM outbox_messages WHERE status = 'pending' AND next_attempt_at <= $2
			ORDER BY next_attempt_at, id FOR UPDATE SKIP LOCKED LIMIT $3)
		RETURNING id, subscription_id, kind, event_id, attempts`, now.Add(lease), now, batchSize)
	if err != nil {
		return err
	}
	var batch []claimed
	for rows.Next() {
		var c claimed
		if err := rows.Scan(&c.id, &c.subID, &c.kind, &c.eventID, &c.attempts); err != nil {
			rows.Close()
			return err
		}
		batch = append(batch, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, c := range batch {
		sendErr := w.send(ctx, c)
		if err := w.record(ctx, c, sendErr); err != nil {
			return err
		}
	}
	return nil
}

var errSkip = errors.New("skip")

func (w *Worker) send(ctx context.Context, c claimed) error {
	sub, err := get(ctx, w.svc.pool, c.subID, false)
	if err != nil {
		return errSkip
	}
	// Event and test messages go only to active subscriptions.
	if (c.kind == "event" || c.kind == "test") && sub.Status != StatusActive {
		return errSkip
	}
	var e notify.Event
	if c.eventID != nil {
		var raw []byte
		if err := w.svc.pool.QueryRow(ctx, "SELECT payload FROM notification_events WHERE id = $1", *c.eventID).Scan(&raw); err != nil {
			return errSkip
		}
		if err := json.Unmarshal(raw, &e); err != nil {
			return errSkip
		}
	}
	if c.kind == "test" {
		e = notify.Event{Kind: notify.Test, Title: "Integração configurada", Message: "Mensagem de teste enviada por um administrador.", Components: []notify.Component{}, OccurredAt: w.svc.now()}
	}
	sctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	switch {
	case c.kind == "confirm":
		if sub.Email == nil || w.svc.mailer == nil {
			return errSkip
		}
		link, err := w.svc.link(ctx, "/subscriptions/confirm", sub.ID, purposeConfirm)
		if err != nil {
			return err
		}
		return w.svc.mailer.Send(sctx, w.svc.confirmMail(*sub.Email, sub, link))
	case c.kind == "manage_link":
		if sub.Email == nil || w.svc.mailer == nil {
			return errSkip
		}
		link, err := w.svc.link(ctx, "/subscriptions/manage", sub.ID, purposeManage)
		if err != nil {
			return err
		}
		return w.svc.mailer.Send(sctx, w.svc.manageMail(*sub.Email, link))
	case sub.Channel == ChannelEmail:
		if w.svc.mailer == nil {
			return errors.New("e-mail não configurado")
		}
		link, err := w.svc.link(ctx, "/subscriptions/manage", sub.ID, purposeManage)
		if err != nil {
			return err
		}
		if c.kind == "test" {
			return w.svc.mailer.Send(sctx, w.svc.testMail(*sub.Email, link))
		}
		return w.svc.mailer.Send(sctx, w.svc.eventMail(*sub.Email, e, link))
	case sub.Channel == ChannelSlack:
		return w.svc.poster.Post(sctx, deref(sub.TargetURL), w.svc.slackBody(e), nil)
	case sub.Channel == ChannelWebhook:
		body, headers := w.svc.webhookRequest(deref(sub.signingSecret), c.id, e, w.svc.now())
		return w.svc.poster.Post(sctx, deref(sub.TargetURL), body, headers)
	}
	return errSkip
}

func shortError(err error) string {
	s := err.Error()
	if utf8.RuneCountInString(s) > 280 {
		s = string([]rune(s)[:280])
	}
	return s
}

func (w *Worker) record(ctx context.Context, c claimed, sendErr error) error {
	now := w.svc.now()
	switch {
	case sendErr == nil:
		if _, err := w.svc.pool.Exec(ctx, "UPDATE outbox_messages SET status = 'sent', sent_at = $2, last_error = NULL WHERE id = $1", c.id, now); err != nil {
			return err
		}
		_, err := w.svc.pool.Exec(ctx, `UPDATE subscriptions SET last_delivery_at = $2, failure_count = 0, last_error = NULL WHERE id = $1`, c.subID, now)
		return err
	case errors.Is(sendErr, errSkip):
		_, err := w.svc.pool.Exec(ctx, "UPDATE outbox_messages SET status = 'failed', last_error = 'Descartada: inscrição inativa ou removida.' WHERE id = $1", c.id)
		return err
	}
	msg := shortError(sendErr)
	w.log.Warn("notifications: delivery attempt failed", "message_id", c.id, "subscription_id", c.subID, "attempt", c.attempts, "error", msg)
	if c.attempts >= maxAttempts {
		if _, err := w.svc.pool.Exec(ctx, "UPDATE outbox_messages SET status = 'failed', last_error = $2 WHERE id = $1", c.id, msg); err != nil {
			return err
		}
	} else {
		next := now.Add(backoff[min(c.attempts-1, len(backoff)-1)])
		if _, err := w.svc.pool.Exec(ctx, "UPDATE outbox_messages SET next_attempt_at = $2, last_error = $3 WHERE id = $1", c.id, next, msg); err != nil {
			return err
		}
	}
	// Endpoints that keep failing are disabled (not e-mail: SMTP failures
	// are on our side).
	_, err := w.svc.pool.Exec(ctx, `UPDATE subscriptions SET failure_count = failure_count + 1, last_error = $2,
		status = CASE WHEN channel <> 'email' AND failure_count + 1 >= $3 THEN 'disabled' ELSE status END,
		disabled_reason = CASE WHEN channel <> 'email' AND failure_count + 1 >= $3 THEN 'Desativada após falhas consecutivas de entrega.' ELSE disabled_reason END
		WHERE id = $1`, c.subID, msg, disableAfterFailed)
	return err
}

// purge removes stale data: unconfirmed subscriptions after 7 days, old
// delivered or failed messages after 30 days, old events after 180 days.
func (w *Worker) purge(ctx context.Context) {
	now := w.svc.now()
	for _, q := range []struct {
		sql string
		at  time.Time
	}{
		{"DELETE FROM subscriptions WHERE status = 'pending' AND created_at < $1", now.Add(-7 * 24 * time.Hour)},
		{"DELETE FROM outbox_messages WHERE status <> 'pending' AND created_at < $1", now.Add(-30 * 24 * time.Hour)},
		{"DELETE FROM notification_events WHERE dispatched_at IS NOT NULL AND created_at < $1", now.Add(-180 * 24 * time.Hour)},
	} {
		if _, err := w.svc.pool.Exec(ctx, q.sql, q.at); err != nil {
			w.log.Warn("notifications: purge failed", "error", err)
		}
	}
}
