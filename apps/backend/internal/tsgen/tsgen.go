// Package tsgen generates TypeScript type declarations from the schemas of
// the OpenAPI contract (api/openapi.yaml). The frontend imports the result
// (apps/frontend/src/api/schema.gen.ts) instead of hand-written copies, and a
// test fails when the committed file is out of date.
//
// It supports the OpenAPI 3.0 subset used by the contract: $ref, objects
// with properties/required/additionalProperties, arrays, enums, nullable,
// allOf/oneOf/anyOf and primitive types. Unsupported constructs fail loudly.
package tsgen

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

const header = `// Code generated from apps/backend/api/openapi.yaml. DO NOT EDIT.
// Regenerate with: make generate-types (go run ./cmd/openapi-ts in apps/backend).
`

// Generate returns the TypeScript declarations for all component schemas.
func Generate(spec []byte) (string, error) {
	doc, err := openapi3.NewLoader().LoadFromData(spec)
	if err != nil {
		return "", fmt.Errorf("load spec: %w", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		return "", fmt.Errorf("invalid spec: %w", err)
	}
	names := make([]string, 0, len(doc.Components.Schemas))
	for name := range doc.Components.Schemas {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString(header)
	for _, name := range names {
		ref := doc.Components.Schemas[name]
		t, err := typeOf(ref.Value, "")
		if err != nil {
			return "", fmt.Errorf("schema %s: %w", name, err)
		}
		b.WriteString("\n")
		writeDoc(&b, ref.Value.Description, "")
		fmt.Fprintf(&b, "export type %s = %s;\n", name, t)
	}
	return b.String(), nil
}

func refName(ref string) string {
	return ref[strings.LastIndex(ref, "/")+1:]
}

func schemaType(s *openapi3.Schema) string {
	if s.Type == nil || len(*s.Type) == 0 {
		return ""
	}
	return (*s.Type)[0]
}

// typeOf renders a schema; indent is the indentation of the enclosing line.
func typeOf(s *openapi3.Schema, indent string) (string, error) {
	t, err := baseType(s, indent)
	if err != nil {
		return "", err
	}
	if s.Nullable {
		t += " | null"
	}
	return t, nil
}

func refOrInline(r *openapi3.SchemaRef, indent string) (string, error) {
	if r.Ref != "" {
		t := refName(r.Ref)
		if r.Value != nil && r.Value.Nullable && !strings.HasPrefix(r.Ref, "#/components/schemas/") {
			t += " | null"
		}
		return t, nil
	}
	return typeOf(r.Value, indent)
}

func combine(refs openapi3.SchemaRefs, sep, indent string) (string, error) {
	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		t, err := refOrInline(r, indent)
		if err != nil {
			return "", err
		}
		parts = append(parts, paren(t))
	}
	return strings.Join(parts, sep), nil
}

func baseType(s *openapi3.Schema, indent string) (string, error) {
	switch {
	case len(s.AllOf) > 0:
		return combine(s.AllOf, " & ", indent)
	case len(s.OneOf) > 0:
		return combine(s.OneOf, " | ", indent)
	case len(s.AnyOf) > 0:
		return combine(s.AnyOf, " | ", indent)
	case len(s.Enum) > 0:
		lits := make([]string, 0, len(s.Enum))
		for _, e := range s.Enum {
			switch v := e.(type) {
			case string:
				lits = append(lits, strconv.Quote(v))
			case float64, int, bool:
				lits = append(lits, fmt.Sprint(v))
			default:
				return "", fmt.Errorf("unsupported enum value %v", e)
			}
		}
		return strings.Join(lits, " | "), nil
	}
	switch schemaType(s) {
	case "string":
		return "string", nil
	case "integer", "number":
		return "number", nil
	case "boolean":
		return "boolean", nil
	case "array":
		if s.Items == nil {
			return "unknown[]", nil
		}
		t, err := refOrInline(s.Items, indent)
		if err != nil {
			return "", err
		}
		return paren(t) + "[]", nil
	case "object", "":
		return objectType(s, indent)
	default:
		return "", fmt.Errorf("unsupported type %q", schemaType(s))
	}
}

func objectType(s *openapi3.Schema, indent string) (string, error) {
	if len(s.Properties) == 0 {
		if s.AdditionalProperties.Schema != nil {
			t, err := refOrInline(s.AdditionalProperties.Schema, indent)
			if err != nil {
				return "", err
			}
			return "Record<string, " + t + ">", nil
		}
		return "Record<string, unknown>", nil
	}
	required := map[string]bool{}
	for _, r := range s.Required {
		required[r] = true
	}
	names := make([]string, 0, len(s.Properties))
	for name := range s.Properties {
		names = append(names, name)
	}
	// Keep the declaration order stable and readable: required first, then
	// alphabetical. (YAML key order is not preserved by the loader.)
	sort.SliceStable(names, func(i, j int) bool {
		if required[names[i]] != required[names[j]] {
			return required[names[i]]
		}
		return names[i] < names[j]
	})
	inner := indent + "  "
	var b strings.Builder
	b.WriteString("{\n")
	for _, name := range names {
		prop := s.Properties[name]
		t, err := refOrInline(prop, inner)
		if err != nil {
			return "", fmt.Errorf("property %s: %w", name, err)
		}
		if prop.Ref == "" && prop.Value != nil {
			writeDoc(&b, prop.Value.Description, inner)
		}
		opt := "?"
		if required[name] {
			opt = ""
		}
		fmt.Fprintf(&b, "%s%s%s: %s;\n", inner, name, opt, t)
	}
	b.WriteString(indent + "}")
	return b.String(), nil
}

func paren(t string) string {
	if strings.Contains(t, " | ") || strings.Contains(t, " & ") {
		return "(" + t + ")"
	}
	return t
}

func writeDoc(b *strings.Builder, desc, indent string) {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return
	}
	desc = strings.ReplaceAll(desc, "*/", "* /")
	lines := strings.Split(desc, "\n")
	if len(lines) == 1 {
		fmt.Fprintf(b, "%s/** %s */\n", indent, lines[0])
		return
	}
	fmt.Fprintf(b, "%s/**\n", indent)
	for _, l := range lines {
		fmt.Fprintf(b, "%s * %s\n", indent, strings.TrimRight(l, " "))
	}
	fmt.Fprintf(b, "%s */\n", indent)
}
