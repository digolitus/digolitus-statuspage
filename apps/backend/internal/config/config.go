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

	maxConns, err := strconv.Atoi(get("DB_MAX_CONNS", "10"))
	if err != nil || maxConns < 1 || maxConns > 100 {
		errs = append(errs, errors.New("DB_MAX_CONNS must be an integer between 1 and 100"))
	}
	cfg.DBMaxConns = int32(maxConns) // #nosec G115 -- validated to 1..100 above.

	parseBool := func(key, def string, dst *bool) {
		b, err := strconv.ParseBool(get(key, def))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s must be true or false", key))
		}
		*dst = b
	}
	parseBool("DB_AUTO_MIGRATE", "true", &cfg.AutoMigrate)
	parseBool("ADMIN_API_ENABLED", "true", &cfg.AdminAPIEnabled)

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
