package main

import (
    "context"
    "errors"
    "log/slog"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/digolitus/digolitus-statuspage/apps/backend/internal/config"
    "github.com/digolitus/digolitus-statuspage/apps/backend/internal/handlers"
)

func main() {
    cfg := config.Load()

    server := &http.Server{
        Addr:              ":" + cfg.Port,
        Handler:           handlers.New().Routes(),
        ReadHeaderTimeout: 5 * time.Second,
        ReadTimeout:       10 * time.Second,
        WriteTimeout:      10 * time.Second,
        IdleTimeout:       60 * time.Second,
    }

    serverErrors := make(chan error, 1)

    go func() {
        slog.Info("backend listening", "port", cfg.Port)
        serverErrors <- server.ListenAndServe()
    }()

    signals := make(chan os.Signal, 1)
    signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

    select {
    case err := <-serverErrors:
        if !errors.Is(err, http.ErrServerClosed) {
            slog.Error("server failed", "error", err)
            os.Exit(1)
        }
    case signal := <-signals:
        slog.Info("shutdown signal received", "signal", signal.String())

        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()

        if err := server.Shutdown(ctx); err != nil {
            slog.Error("graceful shutdown failed", "error", err)
            os.Exit(1)
        }
    }

    slog.Info("server stopped")
}
