// Package testsupport provides an isolated PostgreSQL database per test.
//
// Integration tests require TEST_DATABASE_URL pointing to a server where the
// user may create databases (e.g. postgres://postgres@127.0.0.1:5432/postgres).
// Without it they are skipped, unless REQUIRE_DB_TESTS=1 (set in CI) turns
// the absence into a failure so integration coverage cannot silently vanish.
package testsupport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
)

// AdminURL returns TEST_DATABASE_URL or skips/fails the test.
func AdminURL(t testing.TB) string {
	t.Helper()
	u := os.Getenv("TEST_DATABASE_URL")
	if u == "" {
		if os.Getenv("REQUIRE_DB_TESTS") == "1" {
			t.Fatal("TEST_DATABASE_URL is required when REQUIRE_DB_TESTS=1")
		}
		t.Skip("TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}
	return u
}

// NewDatabase creates an empty database and returns its URL. It is dropped
// when the test ends.
func NewDatabase(t testing.TB) string {
	t.Helper()
	admin := AdminURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var b [6]byte
	_, _ = rand.Read(b[:])
	name := "test_" + hex.EncodeToString(b[:])

	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatalf("connect to TEST_DATABASE_URL: %v", err)
	}
	defer conn.Close(ctx)
	// #nosec G202 -- name is generated above from random hex; identifiers cannot be bound parameters.
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		c, err := pgx.Connect(ctx, admin)
		if err != nil {
			t.Logf("cleanup connect: %v", err)
			return
		}
		defer c.Close(ctx)
		// #nosec G202 -- name is generated from random hex in this function.
		if _, err := c.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
			t.Logf("drop test database: %v", err)
		}
	})

	u, err := url.Parse(admin)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}

// NewPool creates a migrated database and returns a pool to it.
func NewPool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	pool := NewEmptyPool(t)
	if err := database.Migrate(context.Background(), pool, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

// NewEmptyPool creates an unmigrated database and returns a pool to it.
func NewEmptyPool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	pool, err := database.Open(context.Background(), database.Options{URL: NewDatabase(t), MaxConns: 8})
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
