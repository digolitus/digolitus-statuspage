package suspicion

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/subscription"
)

// E-mails to reporters go through an outbox written in the transaction of
// the change and delivered by RunOnce, so a rolled-back change sends nothing.

const (
	maxMailAttempts = 8
	mailBatch       = 20
)

func (s *Service) mailText(ctx context.Context, body string) string {
	return body + "\n\nAcompanhe e responda em: " + s.baseURL + "/suspeitas/minhas\n\n— " + s.siteName() + "\n"
}

// mail queues an e-mail to one reporter about a suspicion. Without e-mail
// configured nothing is queued.
func (s *Service) mail(ctx context.Context, tx pgx.Tx, reporterID, suspicionID, subject, body string, now time.Time) error {
	if !s.EmailEnabled() {
		return nil
	}
	var number int64
	var component string
	if err := tx.QueryRow(ctx, "SELECT s.number, c.name FROM suspicions s JOIN components c ON c.id = s.component_id WHERE s.id = $1", suspicionID).
		Scan(&number, &component); err != nil {
		return fmt.Errorf("mail subject: %w", err)
	}
	subj := fmt.Sprintf("%s · %s (#S-%04d)", subject, component, number)
	if r := []rune(subj); len(r) > 200 {
		subj = string(r[:200])
	}
	text := s.mailText(ctx, body)
	if r := []rune(text); len(r) > 6000 {
		text = string(r[:6000])
	}
	_, err := tx.Exec(ctx, `INSERT INTO reporter_mails (reporter_id, subject, body, created_at, next_attempt_at) VALUES ($1, $2, $3, $4, $4)`,
		reporterID, subj, text, now)
	if err != nil {
		return fmt.Errorf("queue reporter mail: %w", err)
	}
	return nil
}

// mailAll queues the same e-mail to everyone who reported the suspicion.
func (s *Service) mailAll(ctx context.Context, tx pgx.Tx, suspicionID, subject, body string, now time.Time) error {
	if !s.EmailEnabled() {
		return nil
	}
	rows, err := tx.Query(ctx, "SELECT reporter_id FROM suspicion_reports WHERE suspicion_id = $1", suspicionID)
	if err != nil {
		return fmt.Errorf("list reporters: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.mail(ctx, tx, id, suspicionID, subject, body, now); err != nil {
			return err
		}
	}
	return nil
}

// notifyResolved tells reporters that the incident opened from their
// suspicion was resolved (once per suspicion).
func (s *Service) notifyResolved(ctx context.Context) error {
	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT s.id, i.title FROM suspicions s JOIN incidents i ON i.id = s.incident_id
			WHERE s.status = 'incident' AND i.status = 'resolved' AND s.resolved_notified_at IS NULL
			ORDER BY s.id LIMIT 50 FOR UPDATE OF s SKIP LOCKED`)
		if err != nil {
			return fmt.Errorf("find resolved incidents: %w", err)
		}
		type item struct{ id, title string }
		var items []item
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.id, &it.title); err != nil {
				rows.Close()
				return err
			}
			items = append(items, it)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		now := s.now()
		for _, it := range items {
			if err := s.mailAll(ctx, tx, it.id, "Incidente resolvido", "O incidente aberto a partir da sua suspeita foi resolvido: "+it.title+".", now); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "UPDATE suspicions SET resolved_notified_at = $2 WHERE id = $1", it.id, now); err != nil {
				return fmt.Errorf("mark resolved notified: %w", err)
			}
		}
		return nil
	})
}

type queued struct {
	id       int64
	to       string
	subject  string
	body     string
	attempts int
}

// deliver sends due e-mails, retrying failures with a growing delay.
func (s *Service) deliver(ctx context.Context) error {
	if s.mailer == nil {
		return nil
	}
	for range 10 {
		var batch []queued
		err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
			now := s.now()
			rows, err := tx.Query(ctx, `SELECT m.id, r.email, m.subject, m.body, m.attempts FROM reporter_mails m JOIN reporters r ON r.id = m.reporter_id
				WHERE m.sent_at IS NULL AND m.attempts < $1 AND m.next_attempt_at <= $2 ORDER BY m.id LIMIT $3 FOR UPDATE OF m SKIP LOCKED`,
				maxMailAttempts, now, mailBatch)
			if err != nil {
				return fmt.Errorf("claim reporter mails: %w", err)
			}
			for rows.Next() {
				var q queued
				if err := rows.Scan(&q.id, &q.to, &q.subject, &q.body, &q.attempts); err != nil {
					rows.Close()
					return err
				}
				batch = append(batch, q)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
			// Push the claimed rows ahead so another replica does not send
			// them while this one does.
			for _, q := range batch {
				if _, err := tx.Exec(ctx, "UPDATE reporter_mails SET next_attempt_at = $2 WHERE id = $1", q.id, now.Add(5*time.Minute)); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		for _, q := range batch {
			sendErr := s.mailer.Send(ctx, subscription.Mail{To: q.to, Subject: q.subject, Text: q.body})
			now := s.now()
			if sendErr == nil {
				_, err = s.pool.Exec(ctx, "UPDATE reporter_mails SET sent_at = $2, attempts = attempts + 1, last_error = NULL WHERE id = $1", q.id, now)
			} else {
				msg := sendErr.Error()
				if len(msg) > 300 {
					msg = msg[:300]
				}
				delay := time.Duration((q.attempts+1)*(q.attempts+1)) * time.Minute
				_, err = s.pool.Exec(ctx, "UPDATE reporter_mails SET attempts = attempts + 1, last_error = $2, next_attempt_at = $3 WHERE id = $1",
					q.id, msg, now.Add(delay))
			}
			if err != nil {
				return fmt.Errorf("record reporter mail: %w", err)
			}
		}
	}
	return nil
}

// purge removes old e-mails, expired sessions and used sign-in links.
func (s *Service) purge(ctx context.Context) error {
	now := s.now()
	for _, q := range []string{
		"DELETE FROM reporter_mails WHERE created_at < $1 - interval '30 days'",
		"DELETE FROM reporter_sessions WHERE expires_at < $1",
		"DELETE FROM reporter_login_tokens WHERE expires_at < $1 - interval '1 day'",
	} {
		if _, err := s.pool.Exec(ctx, q, now); err != nil {
			return fmt.Errorf("purge: %w", err)
		}
	}
	return nil
}

// RunOnce runs one cycle of the reporter e-mail worker.
func (s *Service) RunOnce(ctx context.Context, log *slog.Logger) {
	if err := s.notifyResolved(ctx); err != nil && ctx.Err() == nil {
		log.Warn("suspicion worker step failed", "step", "resolved", "error", err)
	}
	if err := s.deliver(ctx); err != nil && ctx.Err() == nil {
		log.Warn("suspicion worker step failed", "step", "deliver", "error", err)
	}
	if err := s.purge(ctx); err != nil && ctx.Err() == nil {
		log.Warn("suspicion worker step failed", "step", "purge", "error", err)
	}
}

// Run calls RunOnce every interval until ctx ends.
func (s *Service) Run(ctx context.Context, log *slog.Logger, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		s.RunOnce(ctx, log)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
