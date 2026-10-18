// Command api runs the Digolitus Status Page HTTP API.
//
// Usage:
//
//	api           start the server (applies migrations when DB_AUTO_MIGRATE=true)
//	api migrate   apply pending migrations and exit
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
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/config"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/external"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
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
	if command != "serve" && command != "migrate" {
		return fmt.Errorf("unknown command %q (use: serve, migrate)", command)
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
	if cfg.AutoMigrate {
		if err := database.Migrate(ctx, pool, log); err != nil {
			return err
		}
	}
	if !cfg.AdminAPIEnabled {
		log.Info("admin API disabled (ADMIN_API_ENABLED=false): serving public endpoints only")
	} else {
		log.Warn("admin API enabled without authentication: do not expose it beyond a trusted network")
	}

	application := app.New(pool, app.Options{
		AdminAPIEnabled:  cfg.AdminAPIEnabled,
		RequestTimeout:   cfg.RequestTimeout,
		ExternalInterval: cfg.ExternalInterval,
		ExternalFetcher:  external.NewHTTPFetcher(10*time.Second, cfg.ExternalAllowPrivate),
	})
	if cfg.ExternalAllowPrivate {
		log.Warn("third-party sources may target private networks (EXTERNAL_STATUS_ALLOW_PRIVATE_NETWORKS=true)")
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
