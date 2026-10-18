package app

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"

	"github.com/digolitus/digolitus-statuspage/apps/backend/api"
)

func init() {
	// Feeds are XML documents; the contract only checks status and media type.
	openapi3filter.RegisterBodyDecoder("application/atom+xml", openapi3filter.PlainBodyDecoder)
	openapi3filter.RegisterBodyDecoder("application/rss+xml", openapi3filter.PlainBodyDecoder)
}

func loadSpec(t *testing.T) *openapi3.T {
	t.Helper()
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(api.OpenAPISpec)
	if err != nil {
		t.Fatalf("load OpenAPI spec: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("OpenAPI spec is invalid: %v", err)
	}
	return doc
}

// TestSpecIsValid checks syntax and schema consistency of the contract.
func TestSpecIsValid(t *testing.T) {
	loadSpec(t)
}

// TestRoutesMatchSpec fails when an implemented route is undocumented or a
// documented route is not implemented, in either direction.
func TestRoutesMatchSpec(t *testing.T) {
	doc := loadSpec(t)
	documented := map[string]bool{}
	for path, item := range doc.Paths.Map() {
		for method := range item.Operations() {
			documented[strings.ToUpper(method)+" "+path] = true
		}
	}
	application := New(nil, Options{AdminAPIEnabled: true})
	implemented := map[string]bool{}
	for _, r := range application.Router.Routes() {
		implemented[r.Method+" "+r.Path] = true
	}

	var missingDocs, missingImpl []string
	for k := range implemented {
		if !documented[k] {
			missingDocs = append(missingDocs, k)
		}
	}
	for k := range documented {
		if !implemented[k] {
			missingImpl = append(missingImpl, k)
		}
	}
	sort.Strings(missingDocs)
	sort.Strings(missingImpl)
	if len(missingDocs) > 0 {
		t.Errorf("routes implemented but not documented in api/openapi.yaml:\n  %s", strings.Join(missingDocs, "\n  "))
	}
	if len(missingImpl) > 0 {
		t.Errorf("routes documented but not implemented:\n  %s", strings.Join(missingImpl, "\n  "))
	}
}

// TestAdminRoutesCanBeDisabled ensures ADMIN_API_ENABLED=false removes every
// admin route and every mutating route except visitors' own subscription
// self-service (subscribe, confirm, manage, unsubscribe), so a public
// read-only instance can still take subscriptions.
func TestAdminRoutesCanBeDisabled(t *testing.T) {
	application := New(nil, Options{AdminAPIEnabled: false})
	for _, r := range application.Router.Routes() {
		selfService := strings.HasPrefix(r.Path, "/api/v1/subscriptions")
		if strings.HasPrefix(r.Path, "/api/v1/admin/") || r.Method != http.MethodGet && !selfService {
			t.Errorf("route %s %s must not be registered when the admin API is disabled", r.Method, r.Path)
		}
	}
}
