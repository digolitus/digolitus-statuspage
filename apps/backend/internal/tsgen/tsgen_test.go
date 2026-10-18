package tsgen

import (
	"os"
	"strings"
	"testing"

	"github.com/digolitus/digolitus-statuspage/apps/backend/api"
)

// generatedFile is the committed output, relative to this package.
const generatedFile = "../../../frontend/src/api/schema.gen.ts"

// TestGeneratedTypesAreUpToDate fails when the contract changed but the
// frontend types were not regenerated (or were edited by hand).
func TestGeneratedTypesAreUpToDate(t *testing.T) {
	want, err := Generate(api.OpenAPISpec)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(generatedFile)
	if err != nil {
		t.Fatalf("read %s: %v", generatedFile, err)
	}
	if string(got) != want {
		t.Fatalf("%s is out of date with api/openapi.yaml.\nRun: make generate-types (or: cd apps/backend && go run ./cmd/openapi-ts)", generatedFile)
	}
}

func TestGenerateShapes(t *testing.T) {
	spec := []byte(`openapi: 3.0.3
info: {title: t, version: "1"}
paths: {}
components:
  schemas:
    Status:
      type: string
      enum: [a, b]
    Item:
      type: object
      required: [id, status]
      properties:
        id: {type: string}
        status: {$ref: '#/components/schemas/Status'}
        note: {type: string, nullable: true, description: A note.}
        tags: {type: array, items: {type: string, enum: [x, y]}}
        meta: {type: object, additionalProperties: true}
`)
	out, err := Generate(spec)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`export type Status = "a" | "b";`,
		"  id: string;",
		"  status: Status;",
		"  /** A note. */\n  note?: string | null;",
		`  tags?: ("x" | "y")[];`,
		"  meta?: Record<string, unknown>;",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}
