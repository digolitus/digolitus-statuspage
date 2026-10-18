package handlers

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestHello(t *testing.T) {
    req := httptest.NewRequest(http.MethodGet, "/api/hello", nil)
    rec := httptest.NewRecorder()

    New().Routes().ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("expected status 200, got %d", rec.Code)
    }

    if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
        t.Fatalf("unexpected content type: %q", got)
    }

    var response helloResponse
    if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
        t.Fatalf("decode response: %v", err)
    }

    if response.Message != "Hello World from Digolitus Status Page" {
        t.Fatalf("unexpected message: %q", response.Message)
    }

    if response.Service != "digolitus-statuspage-backend" {
        t.Fatalf("unexpected service: %q", response.Service)
    }
}

func TestHealthz(t *testing.T) {
    req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
    rec := httptest.NewRecorder()

    New().Routes().ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("expected status 200, got %d", rec.Code)
    }
}

func TestReadyz(t *testing.T) {
    req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
    rec := httptest.NewRecorder()

    New().Routes().ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("expected status 200, got %d", rec.Code)
    }
}

func TestUnsupportedMethod(t *testing.T) {
    req := httptest.NewRequest(http.MethodPost, "/api/hello", nil)
    rec := httptest.NewRecorder()

    New().Routes().ServeHTTP(rec, req)

    if rec.Code != http.StatusMethodNotAllowed {
        t.Fatalf("expected status 405, got %d", rec.Code)
    }
}

func TestNotFound(t *testing.T) {
    req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
    rec := httptest.NewRecorder()

    New().Routes().ServeHTTP(rec, req)

    if rec.Code != http.StatusNotFound {
        t.Fatalf("expected status 404, got %d", rec.Code)
    }
}
