// Command openapi-ts writes the TypeScript types generated from the API
// contract to the frontend.
//
// Usage (from apps/backend): go run ./cmd/openapi-ts [-o path]
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/digolitus/digolitus-statuspage/apps/backend/api"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/tsgen"
)

// DefaultOutput is relative to apps/backend.
const DefaultOutput = "../frontend/src/api/schema.gen.ts"

func main() {
	out := flag.String("o", DefaultOutput, "output file")
	flag.Parse()
	code, err := tsgen.Generate(api.OpenAPISpec)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, []byte(code), 0o644); err != nil { // #nosec G306 -- source file, world-readable by design.
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote", *out)
}
