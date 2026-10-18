package external

import (
	"context"
	"log/slog"
	"time"
)

// Poller refreshes due sources periodically. Several backend instances may
// run it: sources are claimed with FOR UPDATE SKIP LOCKED, so each one is
// fetched by a single instance per period.
type Poller struct {
	svc      *Service
	interval time.Duration
	log      *slog.Logger
}

// NewPoller creates a poller for the service's interval.
func NewPoller(svc *Service, log *slog.Logger) *Poller {
	return &Poller{svc: svc, interval: svc.interval, log: log}
}

// Run blocks until ctx is cancelled.
func (p *Poller) Run(ctx context.Context) {
	tick := max(p.interval/4, 5*time.Second)
	t := time.NewTicker(tick)
	defer t.Stop()
	p.RunOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.RunOnce(ctx)
		}
	}
}

type due struct{ id, base string }

// RunOnce claims and refreshes the sources whose last check is older than
// the interval.
func (p *Poller) RunOnce(ctx context.Context) {
	now := p.svc.now()
	rows, err := p.svc.pool.Query(ctx, `UPDATE external_sources SET last_checked_at = $1
		WHERE id IN (
			SELECT id FROM external_sources
			WHERE is_active AND (last_checked_at IS NULL OR last_checked_at < $2)
			ORDER BY last_checked_at NULLS FIRST
			LIMIT 10
			FOR UPDATE SKIP LOCKED)
		RETURNING id, base_url`, now, now.Add(-p.interval))
	if err != nil {
		if ctx.Err() == nil {
			p.log.Warn("external status: claim failed", "error", err)
		}
		return
	}
	var list []due
	for rows.Next() {
		var d due
		if err := rows.Scan(&d.id, &d.base); err == nil {
			list = append(list, d)
		}
	}
	rows.Close()
	for _, d := range list {
		if err := p.svc.syncOne(ctx, d.id, d.base); err != nil && ctx.Err() == nil {
			p.log.Warn("external status: sync failed", "source_id", d.id, "base_url", d.base, "error", err)
		}
	}
}
