package handlers

import (
    "encoding/json"
    "net/http"
)

type Handler struct{}

type helloResponse struct {
    Message string `json:"message"`
    Service string `json:"service"`
}

type statusResponse struct {
    Status string `json:"status"`
}

func New() *Handler {
    return &Handler{}
}

func (h *Handler) Routes() http.Handler {
    mux := http.NewServeMux()

    mux.HandleFunc("/api/hello", h.hello)
    mux.HandleFunc("/healthz", h.healthz)
    mux.HandleFunc("/readyz", h.readyz)

    return methodGuard(mux)
}

func (h *Handler) hello(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeMethodNotAllowed(w)
        return
    }

    writeJSON(w, http.StatusOK, helloResponse{
        Message: "Hello World from Digolitus Status Page",
        Service: "digolitus-statuspage-backend",
    })
}

func (h *Handler) healthz(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeMethodNotAllowed(w)
        return
    }

    writeJSON(w, http.StatusOK, statusResponse{Status: "ok"})
}

func (h *Handler) readyz(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeMethodNotAllowed(w)
        return
    }

    writeJSON(w, http.StatusOK, statusResponse{Status: "ready"})
}

func methodGuard(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch r.URL.Path {
        case "/api/hello", "/healthz", "/readyz":
            next.ServeHTTP(w, r)
        default:
            writeJSON(w, http.StatusNotFound, map[string]string{
                "error": "resource not found",
            })
        }
    })
}

func writeMethodNotAllowed(w http.ResponseWriter) {
    w.Header().Set("Allow", http.MethodGet)
    writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
        "error": "method not allowed",
    })
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    w.WriteHeader(status)

    if err := json.NewEncoder(w).Encode(payload); err != nil {
        return
    }
}
