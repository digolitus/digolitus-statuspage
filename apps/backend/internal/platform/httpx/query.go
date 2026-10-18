package httpx

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
)

// Pagination limits.
const (
	DefaultLimit = 20
	MaxLimit     = 100
	MaxOffset    = 100_000
)

// Page is a validated limit/offset window.
type Page struct {
	Limit  int
	Offset int
}

// PageInfo is returned with every paginated list.
type PageInfo struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

// List is the envelope for paginated collections.
type List[T any] struct {
	Data       []T      `json:"data"`
	Pagination PageInfo `json:"pagination"`
}

// NewList builds a list envelope, never serialising data as null.
func NewList[T any](items []T, page Page, total int) List[T] {
	if items == nil {
		items = []T{}
	}
	return List[T]{Data: items, Pagination: PageInfo{Limit: page.Limit, Offset: page.Offset, Total: total}}
}

// Sort is a validated sort field and direction. Field is a key into the
// caller's whitelist, never raw user input in SQL.
type Sort struct {
	Field string
	Desc  bool
}

// Query parses common list parameters and accumulates validation errors.
type Query struct {
	values url.Values
	v      apperror.Validator
}

// NewQuery wraps request query values.
func NewQuery(values url.Values) *Query { return &Query{values: values} }

// Page parses limit and offset.
func (q *Query) Page() Page {
	p := Page{Limit: DefaultLimit}
	if raw := q.values.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > MaxLimit {
			q.v.Add("limit", "Informe um inteiro entre 1 e 100.")
		} else {
			p.Limit = n
		}
	}
	if raw := q.values.Get("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > MaxOffset {
			q.v.Add("offset", "Informe um inteiro entre 0 e 100000.")
		} else {
			p.Offset = n
		}
	}
	return p
}

// Sort parses "sort=field" or "sort=-field" against allowed fields.
func (q *Query) Sort(allowed []string, def Sort) Sort {
	raw := q.values.Get("sort")
	if raw == "" {
		return def
	}
	s := Sort{Field: strings.TrimPrefix(raw, "-"), Desc: strings.HasPrefix(raw, "-")}
	for _, a := range allowed {
		if a == s.Field {
			return s
		}
	}
	q.v.Add("sort", "Ordenação inválida. Valores aceitos: "+strings.Join(allowed, ", ")+" (prefixo '-' para ordem decrescente).")
	return def
}

// Enum parses an optional parameter restricted to allowed values.
func (q *Query) Enum(name string, allowed ...string) string {
	raw := q.values.Get(name)
	if raw == "" {
		return ""
	}
	for _, a := range allowed {
		if a == raw {
			return raw
		}
	}
	q.v.Add(name, "Valor inválido. Valores aceitos: "+strings.Join(allowed, ", ")+".")
	return ""
}

// Bool parses an optional boolean parameter.
func (q *Query) Bool(name string) *bool {
	raw := q.values.Get(name)
	if raw == "" {
		return nil
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		q.v.Add(name, "Informe true ou false.")
		return nil
	}
	return &b
}

// UUID parses an optional UUID parameter.
func (q *Query) UUID(name string) string {
	raw := q.values.Get(name)
	if raw == "" {
		return ""
	}
	if !IsUUID(raw) {
		q.v.Add(name, "Informe um UUID válido.")
		return ""
	}
	return strings.ToLower(raw)
}

// Err returns accumulated validation errors.
func (q *Query) Err() error { return q.v.Err() }
