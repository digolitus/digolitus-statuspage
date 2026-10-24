// Command api runs the Digolitus Status Page HTTP API.
//
// Usage:
//
//	api               start the server (applies migrations when DB_AUTO_MIGRATE=true)
//	api migrate       apply pending migrations and exit
//	api setup-token   print a new one-time token for the initial admin setup
//	                  (only while no user exists)
//	api reset-password <e-mail>
//	                  recovery: print a temporary password for the user (who
//	                  must replace it at sign-in), unlocking and reactivating
//	                  the account and ending its sessions
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	// Embedded IANA database: maintenance timezones must not depend on the
	// container image providing tzdata.
	_ "time/tzdata"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/app"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/auth"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/auth/social"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/config"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/external"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/clock"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/subscription"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

// printSetupToken writes a fresh setup token to stdout, for when the token
// logged at startup is no longer available.
// socialProviders builds the configured social sign-in providers, in the
// order shown on the login page.
func socialProviders(o config.OAuth) ([]social.Provider, error) {
	var out []social.Provider
	if o.GoogleEnabled() {
		out = append(out, social.NewGoogle(o.GoogleClientID, o.GoogleClientSecret))
	}
	if o.AppleEnabled() {
		data, err := os.ReadFile(o.AppleKeyFile)
		if err != nil {
			return nil, fmt.Errorf("OAUTH_APPLE_PRIVATE_KEY_FILE: %w", err)
		}
		key, err := social.ParseAppleKey(data)
		if err != nil {
			return nil, fmt.Errorf("OAUTH_APPLE_PRIVATE_KEY_FILE: %w", err)
		}
		out = append(out, social.NewApple(o.AppleClientID, o.AppleTeamID, o.AppleKeyID, key))
	}
	if o.FacebookEnabled() {
		out = append(out, social.NewFacebook(o.FacebookAppID, o.FacebookAppSecret))
	}
	return out, nil
}

func printSetupToken(ctx context.Context, svc *auth.Service) error {
	token, err := svc.EnsureSetupToken(ctx)
	if err != nil {
		return err
	}
	if token == "" {
		return errors.New("setup already completed: an administrator exists; sign in or ask an administrator to reset your password")
	}
	fmt.Println(token)
	return nil
}

func run(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("invalid configuration:\n%w", err)
	}
	var level slog.Level
	_ = level.UnmarshalText([]byte(cfg.LogLevel))
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	command := "serve"
	if len(args) > 0 {
		command = args[0]
	}
	switch command {
	case "serve", "migrate", "setup-token":
	case "reset-password":
		if len(args) != 2 {
			return errors.New("usage: api reset-password <e-mail>")
		}
	default:
		return fmt.Errorf("unknown command %q (use: serve, migrate, setup-token, reset-password)", command)
	}

	pool, err := database.Open(ctx, database.Options{
		URL:             cfg.DatabaseURL,
		MaxConns:        cfg.DBMaxConns,
		MaxConnLifetime: 30 * time.Minute,
		ConnectTimeout:  5 * time.Second,
	})
	if err != nil {
		return err
	}
	defer pool.Close()

	if command == "migrate" {
		return database.Migrate(ctx, pool, log)
	}
	providers, err := socialProviders(cfg.OAuth)
	if err != nil {
		return err
	}
	authCfg := auth.Config{SessionTTL: cfg.SessionTTL, IdleTimeout: cfg.SessionIdleTimeout, CookieSecure: cfg.SessionCookieSecure,
		Providers: providers, BaseURL: cfg.PublicBaseURL}
	if command == "setup-token" {
		return printSetupToken(ctx, auth.NewService(pool, clock.System, authCfg))
	}
	if command == "reset-password" {
		password, err := auth.NewService(pool, clock.System, authCfg).RecoverAccess(ctx, args[1])
		if err != nil {
			return err
		}
		fmt.Printf("temporary password for %s (must be changed at sign-in): %s\n", args[1], password)
		return nil
	}
	if cfg.AutoMigrate {
		if err := database.Migrate(ctx, pool, log); err != nil {
			return err
		}
	}

	var mailer subscription.Mailer
	if cfg.EmailEnabled() {
		m, err := subscription.NewSMTPMailer(subscription.SMTPConfig{Host: cfg.SMTP.Host, Port: cfg.SMTP.Port, Username: cfg.SMTP.Username,
			Password: cfg.SMTP.Password, From: cfg.SMTP.From, TLS: cfg.SMTP.TLS})
		if err != nil {
			return err
		}
		mailer = m
		if cfg.SMTP.TLS == "none" {
			log.Warn("SMTP without TLS (SMTP_TLS=none): use only with a local development server")
		}
	} else {
		log.Info("e-mail notifications disabled (set SMTP_HOST and PUBLIC_BASE_URL to enable): only feeds and admin-created webhooks/Slack are available")
	}
	application := app.New(pool, app.Options{
		AdminAPIEnabled:  cfg.AdminAPIEnabled,
		RequestTimeout:   cfg.RequestTimeout,
		ExternalInterval: cfg.ExternalInterval,
		ExternalFetcher:  external.NewHTTPFetcher(10*time.Second, cfg.ExternalAllowPrivate),
		Auth:             authCfg,
		Mailer:           mailer,
		Poster:           subscription.NewHTTPPoster(10*time.Second, cfg.NotifyAllowPrivate),
		PublicBaseURL:    cfg.PublicBaseURL,
		SiteName:         cfg.SiteName,
	})
	if !cfg.AdminAPIEnabled {
		log.Info("admin API disabled (ADMIN_API_ENABLED=false): serving public endpoints only")
	} else {
		if !cfg.SessionCookieSecure {
			log.Warn("session cookie without Secure flag (SESSION_COOKIE_SECURE=false): use only for local HTTP access")
		}
		for _, p := range providers {
			log.Info("social sign-in enabled", "provider", p.ID(), "redirect_uri", cfg.PublicBaseURL+"/api/v1/auth/oauth/"+p.ID()+"/callback")
		}
		// First run: no user yet. Print a one-time token for /admin/setup.
		token, err := application.Auth.EnsureSetupToken(ctx)
		if err != nil {
			return err
		}
		if token != "" {
			log.Warn("initial setup required: open /admin/setup and enter this one-time setup token to create the first administrator",
				"setup_token", token)
		}
	}
	if cfg.ExternalAllowPrivate {
		log.Warn("third-party sources may target private networks (EXTERNAL_STATUS_ALLOW_PRIVATE_NETWORKS=true)")
	}
	if cfg.NotifyAllowPrivate {
		log.Warn("webhooks may target private networks (NOTIFY_ALLOW_PRIVATE_NETWORKS=true)")
	}
	if cfg.NotifyWorker {
		go subscription.NewWorker(application.Subscriptions, log).Run(ctx, cfg.NotifyInterval)
		log.Info("notification worker enabled", "interval", cfg.NotifyInterval.String())
	}
	if cfg.ExternalSync {
		go external.NewPoller(application.External, log).Run(ctx)
		log.Info("third-party status sync enabled", "interval", cfg.ExternalInterval.String())
	}
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           application.Handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      cfg.RequestTimeout + 5*time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}

	errs := make(chan error, 1)
	go func() {
		log.Info("backend listening", "port", cfg.Port)
		errs <- server.ListenAndServe()
	}()

	select {
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
	}
	log.Info("server stopped")
	return nil
}
