// Package api holds the versioned OpenAPI specification, the contract of the
// HTTP API. Tests check the router and responses against it.
package api

import _ "embed"

// OpenAPISpec is the OpenAPI 3.1 document served at /api/openapi.yaml.
//
//go:embed openapi.yaml
var OpenAPISpec []byte
