package config

import (
	"strings"
	"testing"
	"time"
)

func lookup(env map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := env[k]
		return v, ok
	}
}

func TestDefaults(t *testing.T) {
	cfg, err := FromLookup(lookup(map[string]string{"DATABASE_URL": "postgres://x"}))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SessionCookieSecure || cfg.SessionTTL != 12*time.Hour || cfg.SessionIdleTimeout != 2*time.Hour {
		t.Fatalf("unexpected session defaults: %+v", cfg)
	}
	if !cfg.ExternalSync || cfg.ExternalInterval != 2*time.Minute {
		t.Fatalf("unexpected external defaults: %+v", cfg)
	}
	if cfg.Port != "8080" || cfg.DBMaxConns != 10 || !cfg.AutoMigrate || !cfg.AdminAPIEnabled || cfg.RequestTimeout != 10*time.Second || cfg.LogLevel != "info" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestRequiresDatabaseURL(t *testing.T) {
	_, err := FromLookup(lookup(map[string]string{}))
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("expected DATABASE_URL error, got %v", err)
	}
}

func TestRejectsInvalidValues(t *testing.T) {
	_, err := FromLookup(lookup(map[string]string{
		"DATABASE_URL":         "postgres://x",
		"PORT":                 "http",
		"DB_MAX_CONNS":         "0",
		"ADMIN_API_ENABLED":    "maybe",
		"REQUEST_TIMEOUT":      "1h",
		"LOG_LEVEL":            "loud",
		"SESSION_TTL":          "1m",
		"SESSION_IDLE_TIMEOUT": "1s",
	}))
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, key := range []string{"PORT", "DB_MAX_CONNS", "ADMIN_API_ENABLED", "REQUEST_TIMEOUT", "LOG_LEVEL", "SESSION_TTL", "SESSION_IDLE_TIMEOUT"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error should mention %s: %v", key, err)
		}
	}
}

func TestErrorDoesNotLeakDatabaseURL(t *testing.T) {
	_, err := FromLookup(lookup(map[string]string{"DATABASE_URL": "postgres://user:secret@db/x", "PORT": "0"}))
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error must not contain credentials: %v", err)
	}
}

func TestOAuthProvidersNeedAllSettingsAndBaseURL(t *testing.T) {
	_, err := FromLookup(lookup(map[string]string{
		"DATABASE_URL":           "postgres://x",
		"OAUTH_GOOGLE_CLIENT_ID": "id",
		"OAUTH_APPLE_TEAM_ID":    "team",
	}))
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, want := range []string{"OAUTH_GOOGLE_CLIENT_SECRET", "OAUTH_APPLE_CLIENT_ID", "OAUTH_APPLE_KEY_ID", "OAUTH_APPLE_PRIVATE_KEY_FILE", "PUBLIC_BASE_URL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %s: %v", want, err)
		}
	}

	cfg, err := FromLookup(lookup(map[string]string{
		"DATABASE_URL":              "postgres://x",
		"PUBLIC_BASE_URL":           "https://status.example.com",
		"OAUTH_FACEBOOK_APP_ID":     "app",
		"OAUTH_FACEBOOK_APP_SECRET": "secret",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.OAuth.FacebookEnabled() || cfg.OAuth.GoogleEnabled() || cfg.OAuth.AppleEnabled() {
		t.Fatalf("unexpected providers: %+v", cfg.OAuth)
	}
}
