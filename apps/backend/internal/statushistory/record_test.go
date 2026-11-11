package statushistory_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/testsupport"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/statushistory"
)

// Concurrent changes must leave a history whose last entry is the status in
// force and which never repeats a status (the advisory lock serializes them).
func TestRecordUnderConcurrency(t *testing.T) {
	pool := testsupport.NewPool(t)
	ctx := context.Background()
	var ids [2]string
	for i := range ids {
		if err := pool.QueryRow(ctx, `INSERT INTO components (name, slug) VALUES ($1, $1) RETURNING id`, fmt.Sprintf("c%d", i)).Scan(&ids[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := statushistory.Baseline(ctx, pool, time.Now()); err != nil {
		t.Fatal(err)
	}
	statuses := []string{"operational", "degraded_performance", "partial_outage", "major_outage"}
	var wg sync.WaitGroup
	tx := func(fn func(pgx.Tx) error, cause statushistory.Cause) {
		err := database.InTx(ctx, pool, func(tx pgx.Tx) error {
			if err := fn(tx); err != nil {
				return err
			}
			return statushistory.Record(ctx, tx, cause, time.Now())
		})
		if err != nil {
			t.Error(err)
		}
	}
	for w := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := range 10 {
				id := ids[(w+k)%2]
				if w%2 == 0 {
					// Declared status changes lock the component row...
					tx(func(tx pgx.Tx) error {
						_, err := tx.Exec(ctx, "UPDATE components SET status = $2 WHERE id = $1", id, statuses[(w*k+k)%4])
						return err
					}, statushistory.Cause{Type: statushistory.CauseComponent, ID: id})
					continue
				}
				// ...incidents change the effective status without touching it.
				var inc string
				tx(func(tx pgx.Tx) error {
					if err := tx.QueryRow(ctx, `INSERT INTO incidents (title, impact, status, started_at) VALUES ('x', $1, 'investigating', now())
						RETURNING id`, statuses[1+k%3]).Scan(&inc); err != nil {
						return err
					}
					_, err := tx.Exec(ctx, "INSERT INTO incident_components (incident_id, component_id) VALUES ($1, $2)", inc, id)
					return err
				}, statushistory.Cause{Type: statushistory.CauseIncident})
				tx(func(tx pgx.Tx) error {
					_, err := tx.Exec(ctx, "UPDATE incidents SET status = 'resolved', resolved_at = now() WHERE id = $1", inc)
					return err
				}, statushistory.Cause{Type: statushistory.CauseIncident})
			}
		}()
	}
	wg.Wait()

	for _, id := range ids {
		rows, err := pool.Query(ctx, `SELECT status, previous_status FROM component_status_changes WHERE component_id = $1 ORDER BY changed_at, id`, id)
		if err != nil {
			t.Fatal(err)
		}
		var last *string
		for rows.Next() {
			var st, prev *string
			if err := rows.Scan(&st, &prev); err != nil {
				t.Fatal(err)
			}
			if (prev == nil) != (last == nil) || (prev != nil && *prev != *last) || (last != nil && *st == *last) {
				t.Fatalf("broken chain for %s: prev=%v last=%v status=%v", id, prev, last, *st)
			}
			last = st
		}
		rows.Close()
		var current string
		if err := pool.QueryRow(ctx, "SELECT status FROM components WHERE id = $1", id).Scan(&current); err != nil {
			t.Fatal(err)
		}
		if last == nil || *last != current {
			t.Fatalf("last recorded %v, current %s", last, current)
		}
	}
}
