package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"time"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
)

type ctxKey int

const (
	requestIDKey ctxKey = iota
	sourceKey
	actorKey
)

// SourceAdminAPI identifies changes made through the admin API. The person
// is recorded separately as the actor.
const SourceAdminAPI = "admin_api"

// Actor identifies the authenticated user behind a request, for auditing.
type Actor struct {
	ID   string
	Name string
}

// WithActor attaches the authenticated actor to ctx.
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey, a)
}

// ActorFrom returns the authenticated actor, if any.
func ActorFrom(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey).(Actor)
	return a, ok
}

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// RequestID returns the request correlation ID, if any.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// Source returns the origin of a mutating operation for audit purposes.
func Source(ctx context.Context) string {
	if s, ok := ctx.Value(sourceKey).(string); ok {
		return s
	}
	return "unknown"
}

// SourceCLI identifies changes made with server-side commands (recovery).
const SourceCLI = "cli"

// ContextWithSource tags ctx with an audit source (for non-HTTP callers).
func ContextWithSource(ctx context.Context, source string) context.Context {
	return context.WithValue(ctx, sourceKey, source)
}

// WithSource tags requests handled by next with an audit source.
func WithSource(source string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sourceKey, source)))
	})
}

// Chain applies the standard middleware stack, outermost first.
func Chain(h http.Handler, timeout time.Duration) http.Handler {
	h = withTimeout(timeout, h)
	h = withSecurityHeaders(h)
	h = withRecover(h)
	h = withAccessLog(h)
	h = withRequestID(h)
	return h
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !validRequestID.MatchString(id) {
			var b [12]byte
			_, _ = rand.Read(b[:])
			id = hex.EncodeToString(b[:])
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

func withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		// Health probes run every few seconds; logging them adds noise only.
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			return
		}
		// Query strings are not logged: they may carry search terms.
		slog.InfoContext(r.Context(), "http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", RequestID(r.Context()),
		)
	})
}

func withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				if p == http.ErrAbortHandler {
					panic(p)
				}
				slog.ErrorContext(r.Context(), "panic recovered", "panic", p, "stack", string(debug.Stack()), "request_id", RequestID(r.Context()))
				WriteError(w, r, &apperror.Error{Code: "internal_error", Message: "Erro interno inesperado."}, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "no-store")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// withTimeout bounds request processing through the context; database calls
// and other blocking work observe it.
func withTimeout(d time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), d)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
