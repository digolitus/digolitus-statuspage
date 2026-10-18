// Package app is the composition root: it wires modules, routes and
// middleware into a single http.Handler.
package app

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digolitus/digolitus-statuspage/apps/backend/api"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/audit"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/component"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/docs"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/incident"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/maintenance"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/clock"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/httpx"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/statuspage"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/templates"
)

// Options configures the application handler.
type Options struct {
	AdminAPIEnabled bool
	RequestTimeout  time.Duration
	Clock           clock.Clock
}

// App exposes the handler and its route registry.
type App struct {
	Router  *httpx.Router
	Handler http.Handler
}

// New builds the application.
func New(pool *pgxpool.Pool, opts Options) *App {
	now := opts.Clock
	if now == nil {
		now = clock.System
	}
	if opts.RequestTimeout == 0 {
		opts.RequestTimeout = 10 * time.Second
	}
	rt := httpx.NewRouter()

	rt.Handle(http.MethodGet, "/healthz", liveness)
	rt.Handle(http.MethodGet, "/readyz", readiness(pool))
	rt.Handle(http.MethodGet, "/api/openapi.yaml", serveSpec)
	// Swagger UI is documentation, not an API route, so it is not part of
	// the contract check. It is not a security boundary either.
	rt.HandleRaw("/swagger/", docs.Handler())
	rt.HandleRaw("/swagger", docs.Handler())

	incidentHandler := incident.NewHandler(incident.NewService(pool, now), pool)
	maintenanceHandler := maintenance.NewHandler(maintenance.NewService(pool, now), pool, now)

	statuspage.NewHandler(statuspage.NewService(pool, now)).Register(rt)
	incidentHandler.RegisterPublic(rt)
	maintenanceHandler.RegisterPublic(rt)

	// Admin routes live under /api/v1/admin/ and carry an explicit audit
	// source. There is no authentication in this version: exposure is
	// controlled by deployment (see docs). This wrapper is the single place
	// where authentication and authorization will be enforced later.
	if opts.AdminAPIEnabled {
		admin := func(h http.HandlerFunc) http.HandlerFunc {
			return httpx.WithSource(httpx.SourceAdminAPI, h).ServeHTTP
		}
		component.NewHandler(component.NewService(pool, now), pool).Register(rt, admin)
		incidentHandler.RegisterAdmin(rt, admin)
		maintenanceHandler.RegisterAdmin(rt, admin)
		templates.NewHandler(templates.NewService(pool, now), pool).Register(rt, admin)
		audit.NewHandler(audit.NewStore(pool)).Register(rt, admin)
	}

	return &App{Router: rt, Handler: httpx.Chain(rt, opts.RequestTimeout)}
}

type healthBody struct {
	Status string `json:"status"`
}

// liveness reports that the process can serve HTTP. It deliberately does not
// check the database: a database outage must not restart healthy processes.
func liveness(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, healthBody{Status: "ok"})
}

// readiness reports whether requests can be served, i.e. the database is
// reachable within a short timeout.
func readiness(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			httpx.WriteError(w, r, &apperror.Error{Code: "database_unavailable", Message: "Banco de dados indisponível."}, http.StatusServiceUnavailable)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, healthBody{Status: "ready"})
	}
}

func serveSpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(api.OpenAPISpec)
}
