// Package config loads and validates configuration from the environment.
// Invalid or missing required settings fail startup with a clear message.
package config

import (
	"errors"
	"fmt"
	"os"
	"slices"
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
	// Notifications to subscribers.
	SiteName       string
	PublicBaseURL  string // absolute URL of the public page, used in links
	SMTP           SMTP
	NotifyWorker   bool
	NotifyInterval time.Duration
	// Allows webhook targets on private/loopback addresses (tests or
	// intranet integrations). Off by default.
	NotifyAllowPrivate bool
	// Social sign-in for existing admin users.
	OAuth OAuth
}

// OAuth configures social sign-in. Each provider is enabled when all of its
// settings are present; it needs PUBLIC_BASE_URL for the redirect URI.
type OAuth struct {
	GoogleClientID     string
	GoogleClientSecret string
	FacebookAppID      string
	FacebookAppSecret  string
	// Apple: Services ID, team ID, key ID and the .p8 private key file.
	AppleClientID string
	AppleTeamID   string
	AppleKeyID    string
	AppleKeyFile  string
}

// GoogleEnabled reports whether Google sign-in is configured.
func (o OAuth) GoogleEnabled() bool { return o.GoogleClientID != "" }

// FacebookEnabled reports whether Facebook sign-in is configured.
func (o OAuth) FacebookEnabled() bool { return o.FacebookAppID != "" }

// AppleEnabled reports whether Apple sign-in is configured.
func (o OAuth) AppleEnabled() bool { return o.AppleClientID != "" }

// SMTP configures e-mail delivery. E-mail subscriptions are offered only
// when Host is set (and PUBLIC_BASE_URL, for the links).
type SMTP struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	// TLS: "starttls" (default), "tls" (implicit, port 465) or "none"
	// (local development servers only).
	TLS string
}

// EmailEnabled reports whether e-mail notifications can be sent.
func (c Config) EmailEnabled() bool { return c.SMTP.Host != "" && c.PublicBaseURL != "" }

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

	cfg.SiteName = get("SITE_NAME", "Digolitus Status Page")
	cfg.PublicBaseURL = strings.TrimRight(get("PUBLIC_BASE_URL", ""), "/")
	if cfg.PublicBaseURL != "" && !strings.HasPrefix(cfg.PublicBaseURL, "https://") && !strings.HasPrefix(cfg.PublicBaseURL, "http://") {
		errs = append(errs, errors.New("PUBLIC_BASE_URL must be an absolute http(s) URL, e.g. https://status.example.com"))
	}
	parseBool("NOTIFY_WORKER", "true", &cfg.NotifyWorker)
	parseBool("NOTIFY_ALLOW_PRIVATE_NETWORKS", "false", &cfg.NotifyAllowPrivate)
	notifyInterval, err := time.ParseDuration(get("NOTIFY_INTERVAL", "10s"))
	if err != nil || notifyInterval < time.Second || notifyInterval > 5*time.Minute {
		errs = append(errs, errors.New("NOTIFY_INTERVAL must be a duration between 1s and 5m"))
	}
	cfg.NotifyInterval = notifyInterval
	cfg.SMTP = SMTP{Host: get("SMTP_HOST", ""), Username: get("SMTP_USERNAME", ""), Password: get("SMTP_PASSWORD", ""),
		From: get("SMTP_FROM", ""), TLS: get("SMTP_TLS", "starttls")}
	if cfg.SMTP.Host != "" {
		port, err := strconv.ParseInt(get("SMTP_PORT", "587"), 10, 32)
		if err != nil || port < 1 || port > 65535 {
			errs = append(errs, errors.New("SMTP_PORT must be an integer between 1 and 65535"))
		}
		cfg.SMTP.Port = int(port)
		if !strings.Contains(cfg.SMTP.From, "@") || strings.ContainsAny(cfg.SMTP.From, "\r\n") {
			errs = append(errs, errors.New("SMTP_FROM is required with SMTP_HOST (e.g. Status <status@example.com>)"))
		}
		switch cfg.SMTP.TLS {
		case "starttls", "tls", "none":
		default:
			errs = append(errs, errors.New("SMTP_TLS must be starttls, tls or none"))
		}
		if cfg.PublicBaseURL == "" {
			errs = append(errs, errors.New("PUBLIC_BASE_URL is required with SMTP_HOST (links in e-mails)"))
		}
	}

	cfg.OAuth = OAuth{
		GoogleClientID: get("OAUTH_GOOGLE_CLIENT_ID", ""), GoogleClientSecret: get("OAUTH_GOOGLE_CLIENT_SECRET", ""),
		FacebookAppID: get("OAUTH_FACEBOOK_APP_ID", ""), FacebookAppSecret: get("OAUTH_FACEBOOK_APP_SECRET", ""),
		AppleClientID: get("OAUTH_APPLE_CLIENT_ID", ""), AppleTeamID: get("OAUTH_APPLE_TEAM_ID", ""),
		AppleKeyID: get("OAUTH_APPLE_KEY_ID", ""), AppleKeyFile: get("OAUTH_APPLE_PRIVATE_KEY_FILE", ""),
	}
	requireAll := func(provider string, settings map[string]string) bool {
		var set, missing []string
		for k, v := range settings {
			if v != "" {
				set = append(set, k)
			} else {
				missing = append(missing, k)
			}
		}
		if len(set) > 0 && len(missing) > 0 {
			slices.Sort(missing)
			errs = append(errs, fmt.Errorf("%s sign-in needs %s", provider, strings.Join(missing, ", ")))
		}
		return len(set) > 0
	}
	o := cfg.OAuth
	google := requireAll("Google", map[string]string{"OAUTH_GOOGLE_CLIENT_ID": o.GoogleClientID, "OAUTH_GOOGLE_CLIENT_SECRET": o.GoogleClientSecret})
	facebook := requireAll("Facebook", map[string]string{"OAUTH_FACEBOOK_APP_ID": o.FacebookAppID, "OAUTH_FACEBOOK_APP_SECRET": o.FacebookAppSecret})
	apple := requireAll("Apple", map[string]string{"OAUTH_APPLE_CLIENT_ID": o.AppleClientID, "OAUTH_APPLE_TEAM_ID": o.AppleTeamID,
		"OAUTH_APPLE_KEY_ID": o.AppleKeyID, "OAUTH_APPLE_PRIVATE_KEY_FILE": o.AppleKeyFile})
	if (google || facebook || apple) && cfg.PublicBaseURL == "" {
		errs = append(errs, errors.New("PUBLIC_BASE_URL is required with social sign-in (OAUTH_*): it builds the redirect URI"))
	}

	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, errors.New("LOG_LEVEL must be one of debug, info, warn, error"))
	}
	return cfg, errors.Join(errs...)
}
