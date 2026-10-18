package database_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/testsupport"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestMigrationsAreIdempotent(t *testing.T) {
	pool := testsupport.NewEmptyPool(t)
	ctx := context.Background()
	for range 2 {
		if err := database.Migrate(ctx, pool, quiet); err != nil {
			t.Fatalf("migrate: %v", err)
		}
	}
	migrations, err := database.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	var applied int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != len(migrations) {
		t.Fatalf("applied %d migrations, want %d", applied, len(migrations))
	}
}

func TestMigrateRejectsModifiedHistory(t *testing.T) {
	pool := testsupport.NewPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "UPDATE schema_migrations SET checksum = 'tampered' WHERE version = 1"); err != nil {
		t.Fatal(err)
	}
	err := database.Migrate(ctx, pool, quiet)
	if err == nil || !strings.Contains(err.Error(), "modified after being applied") {
		t.Fatalf("expected checksum error, got %v", err)
	}
}

func TestMigrateRejectsUnknownNewerSchema(t *testing.T) {
	pool := testsupport.NewPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "INSERT INTO schema_migrations (version, name, checksum) VALUES (9999, 'future', 'x')"); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, pool, quiet); err == nil {
		t.Fatal("expected error for unknown applied migration")
	}
}

func TestInTxRollsBackOnError(t *testing.T) {
	pool := testsupport.NewPool(t)
	ctx := context.Background()
	boom := errors.New("boom")
	err := database.InTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO components (name, slug) VALUES ('x', 'x')"); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM components").Scan(&n); err != nil || n != 0 {
		t.Fatalf("rollback failed: %d rows (%v)", n, err)
	}
}

func TestSchemaConstraints(t *testing.T) {
	pool := testsupport.NewPool(t)
	ctx := context.Background()
	bad := []string{
		"INSERT INTO components (name, slug) VALUES ('x', 'Not A Slug')",
		"INSERT INTO components (name, slug, status) VALUES ('x', 'x', 'fine')",
		"INSERT INTO incidents (title, impact, status, started_at) VALUES ('x', 'minor', 'resolved', now())",
		"INSERT INTO maintenances (title, expected_impact, status, timezone, scheduled_start, scheduled_end) VALUES ('x', 'none', 'scheduled', 'UTC', now(), now() - interval '1 hour')",
		"INSERT INTO maintenances (title, expected_impact, status, timezone, scheduled_start, scheduled_end) VALUES ('x', 'none', 'completed', 'UTC', now(), now() + interval '1 hour')",
	}
	for _, sql := range bad {
		if _, err := pool.Exec(ctx, sql); err == nil {
			t.Errorf("constraint not enforced: %s", sql)
		}
	}
}
