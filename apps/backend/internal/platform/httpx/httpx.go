// Package httpx contains the HTTP plumbing shared by all modules: routing
// with a route registry, JSON encoding/decoding with limits, the error
// envelope, pagination/sorting parameters and middleware.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"regexp"
	"strings"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
)

// MaxBodyBytes bounds every request body.
const MaxBodyBytes = 64 << 10

// Route identifies a registered endpoint, used to check the router against
// the OpenAPI specification.
type Route struct {
	Method string
	Path   string
}

// Router wraps http.ServeMux, records registered routes and renders JSON
// 404/405 responses.
type Router struct {
	mux    *http.ServeMux
	routes []Route
}

// NewRouter creates an empty router.
func NewRouter() *Router {
	return &Router{mux: http.NewServeMux()}
}

// Handle registers h for method and path (ServeMux pattern syntax).
func (rt *Router) Handle(method, path string, h http.HandlerFunc) {
	rt.mux.HandleFunc(method+" "+path, h)
	rt.routes = append(rt.routes, Route{Method: method, Path: path})
}

// HandleRaw registers h without recording it as an API route (e.g. static
// documentation assets).
func (rt *Router) HandleRaw(pattern string, h http.Handler) {
	rt.mux.Handle(pattern, h)
}

// Routes returns the registered API routes.
func (rt *Router) Routes() []Route {
	return append([]Route(nil), rt.routes...)
}

// ServeHTTP dispatches and converts the mux's plain-text 404/405 into the
// JSON error envelope.
func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h, pattern := rt.mux.Handler(r)
	if pattern != "" {
		// Dispatch through the mux (not h) so path values like {id} are set.
		rt.mux.ServeHTTP(w, r)
		return
	}
	probe := &statusProbe{header: http.Header{}}
	h.ServeHTTP(probe, r)
	switch probe.status {
	case http.StatusMethodNotAllowed:
		if allow := probe.header.Get("Allow"); allow != "" {
			w.Header().Set("Allow", allow)
		}
		WriteError(w, r, &apperror.Error{Code: "method_not_allowed", Message: "Método não permitido para este recurso."}, http.StatusMethodNotAllowed)
	default:
		WriteError(w, r, &apperror.Error{Code: "route_not_found", Message: "Recurso não encontrado."}, http.StatusNotFound)
	}
}

type statusProbe struct {
	header http.Header
	status int
}

func (p *statusProbe) Header() http.Header         { return p.header }
func (p *statusProbe) Write(b []byte) (int, error) { return len(b), nil }
func (p *statusProbe) WriteHeader(status int)      { p.status = status }

// ErrorBody is the error envelope returned by every endpoint.
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail carries a stable machine code and a human message.
type ErrorDetail struct {
	Code      string                `json:"code"`
	Message   string                `json:"message"`
	Details   []apperror.FieldError `json:"details,omitempty"`
	RequestID string                `json:"request_id,omitempty"`
}

// WriteJSON writes payload with the given status.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// WriteError writes the error envelope. status 0 derives it from the kind.
func WriteError(w http.ResponseWriter, r *http.Request, e *apperror.Error, status int) {
	if status == 0 {
		switch e.Code {
		case "payload_too_large":
			status = http.StatusRequestEntityTooLarge
		case "unsupported_media_type":
			status = http.StatusUnsupportedMediaType
		case "external_fetch_failed":
			status = http.StatusBadGateway
		default:
			status = statusFor(e.Kind)
		}
	}
	WriteJSON(w, status, ErrorBody{Error: ErrorDetail{
		Code:      e.Code,
		Message:   e.Message,
		Details:   e.Fields,
		RequestID: RequestID(r.Context()),
	}})
}

func statusFor(k apperror.Kind) int {
	switch k {
	case apperror.KindInvalid:
		return http.StatusUnprocessableEntity
	case apperror.KindNotFound:
		return http.StatusNotFound
	case apperror.KindConflict:
		return http.StatusConflict
	case apperror.KindUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// Fail writes err: expected errors keep their code; anything else is logged
// and returned as an opaque 500 (or 503 when the request context expired).
func Fail(w http.ResponseWriter, r *http.Request, err error) {
	if appErr, ok := apperror.As(err); ok {
		WriteError(w, r, appErr, 0)
		return
	}
	if r.Context().Err() != nil {
		slog.WarnContext(r.Context(), "request aborted", "error", err, "request_id", RequestID(r.Context()))
		WriteError(w, r, &apperror.Error{Code: "request_timeout", Message: "A requisição excedeu o tempo limite."}, http.StatusServiceUnavailable)
		return
	}
	slog.ErrorContext(r.Context(), "request failed", "error", err, "request_id", RequestID(r.Context()), "method", r.Method, "path", r.URL.Path)
	WriteError(w, r, &apperror.Error{Code: "internal_error", Message: "Erro interno inesperado."}, http.StatusInternalServerError)
}

// DecodeJSON decodes a single JSON object into dst, rejecting unknown fields,
// trailing data, oversized bodies and non-JSON content types.
func DecodeJSON(r *http.Request, dst any) error {
	ct := r.Header.Get("Content-Type")
	if ct != "" {
		mt, _, err := mime.ParseMediaType(ct)
		if err != nil || mt != "application/json" {
			return &apperror.Error{Kind: apperror.KindInvalid, Code: "unsupported_media_type", Message: "Envie o corpo como application/json."}
		}
	}
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, MaxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return decodeError(err)
	}
	if dec.More() {
		return apperror.Invalid("invalid_json", "O corpo deve conter um único objeto JSON.")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return apperror.Invalid("invalid_json", "O corpo deve conter um único objeto JSON.")
	}
	return nil
}

func decodeError(err error) error {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	switch {
	case isTooLarge(err):
		return &apperror.Error{Kind: apperror.KindInvalid, Code: "payload_too_large", Message: "O corpo da requisição excede o limite permitido."}
	case errors.Is(err, io.EOF):
		return apperror.Invalid("invalid_json", "O corpo da requisição está vazio.")
	case errors.As(err, &syntaxErr), errors.Is(err, io.ErrUnexpectedEOF):
		return apperror.Invalid("invalid_json", "O corpo não é um JSON válido.")
	case errors.As(err, &typeErr):
		field := typeErr.Field
		if field == "" {
			return apperror.Invalid("invalid_json", "O corpo deve ser um objeto JSON.")
		}
		return &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
			Fields: []apperror.FieldError{{Field: field, Message: fmt.Sprintf("Tipo inválido: esperado %s.", typeErr.Type.String())}}}
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		field := strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`)
		return &apperror.Error{Kind: apperror.KindInvalid, Code: "validation_failed", Message: "Os dados enviados são inválidos.",
			Fields: []apperror.FieldError{{Field: field, Message: "Campo desconhecido."}}}
	default:
		return apperror.Invalid("invalid_json", "O corpo não é um JSON válido.")
	}
}

func isTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// IsUUID reports whether s is a canonical UUID string.
func IsUUID(s string) bool { return uuidPattern.MatchString(s) }

// PathID returns the {id} path value, or a not-found error for malformed IDs
// so callers never send invalid UUIDs to the database.
func PathID(r *http.Request, resource string) (string, error) {
	id := r.PathValue("id")
	if !IsUUID(id) {
		return "", apperror.NotFound(resource)
	}
	return strings.ToLower(id), nil
}
