// Package config loads and validates configuration from the environment.
// Invalid or missing required settings fail startup with a clear message.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds runtime settings.
type Config struct {
	Port            string
	DatabaseURL     string
	DBMaxConns      int32
	AutoMigrate     bool
	AdminAPIEnabled bool
	RequestTimeout  time.Duration
	LogLevel        string
	// Third-party status pages (e.g. githubstatus.com).
	ExternalSync     bool
	ExternalInterval time.Duration
	// Allows sources on private/loopback addresses (intranet status pages).
	// Off by default: it widens SSRF risk.
	ExternalAllowPrivate bool
	// Admin sessions.
	SessionTTL          time.Duration
	SessionIdleTimeout  time.Duration
	SessionCookieSecure bool
}

// Load reads configuration from the process environment.
func Load() (Config, error) {
	return FromLookup(os.LookupEnv)
}

// FromLookup reads configuration through lookup (testable).
func FromLookup(lookup func(string) (string, bool)) (Config, error) {
	get := func(key, def string) string {
		if v, ok := lookup(key); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		return def
	}
	var errs []error
	cfg := Config{
		Port:        get("PORT", "8080"),
		DatabaseURL: get("DATABASE_URL", ""),
		LogLevel:    strings.ToLower(get("LOG_LEVEL", "info")),
	}

	if p, err := strconv.Atoi(cfg.Port); err != nil || p < 1 || p > 65535 {
		errs = append(errs, errors.New("PORT must be an integer between 1 and 65535"))
	}
	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required (e.g. postgres://user:password@host:5432/db?sslmode=disable)"))
	}

	// Parsed directly as 32 bits: no narrowing conversion is needed.
	maxConns, err := strconv.ParseInt(get("DB_MAX_CONNS", "10"), 10, 32)
	if err != nil || maxConns < 1 || maxConns > 100 {
		errs = append(errs, errors.New("DB_MAX_CONNS must be an integer between 1 and 100"))
	}
	cfg.DBMaxConns = int32(maxConns)

	parseBool := func(key, def string, dst *bool) {
		b, err := strconv.ParseBool(get(key, def))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s must be true or false", key))
		}
		*dst = b
	}
	parseBool("DB_AUTO_MIGRATE", "true", &cfg.AutoMigrate)
	parseBool("ADMIN_API_ENABLED", "true", &cfg.AdminAPIEnabled)
	parseBool("EXTERNAL_STATUS_SYNC", "true", &cfg.ExternalSync)
	parseBool("EXTERNAL_STATUS_ALLOW_PRIVATE_NETWORKS", "false", &cfg.ExternalAllowPrivate)
	parseBool("SESSION_COOKIE_SECURE", "true", &cfg.SessionCookieSecure)

	ttl, err := time.ParseDuration(get("SESSION_TTL", "12h"))
	if err != nil || ttl < 15*time.Minute || ttl > 30*24*time.Hour {
		errs = append(errs, errors.New("SESSION_TTL must be a duration between 15m and 720h"))
	}
	cfg.SessionTTL = ttl
	idle, err := time.ParseDuration(get("SESSION_IDLE_TIMEOUT", "2h"))
	if err != nil || idle < 5*time.Minute || (ttl > 0 && idle > ttl) {
		errs = append(errs, errors.New("SESSION_IDLE_TIMEOUT must be a duration between 5m and SESSION_TTL"))
	}
	cfg.SessionIdleTimeout = idle

	interval, err := time.ParseDuration(get("EXTERNAL_STATUS_INTERVAL", "2m"))
	if err != nil || interval < 30*time.Second || interval > time.Hour {
		errs = append(errs, errors.New("EXTERNAL_STATUS_INTERVAL must be a duration between 30s and 1h"))
	}
	cfg.ExternalInterval = interval

	timeout, err := time.ParseDuration(get("REQUEST_TIMEOUT", "10s"))
	if err != nil || timeout < time.Second || timeout > time.Minute {
		errs = append(errs, errors.New("REQUEST_TIMEOUT must be a duration between 1s and 1m"))
	}
	cfg.RequestTimeout = timeout

	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, errors.New("LOG_LEVEL must be one of debug, info, warn, error"))
	}
	return cfg, errors.Join(errs...)
}
