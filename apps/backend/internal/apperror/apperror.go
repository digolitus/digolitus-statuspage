// Package apperror defines the error vocabulary shared by the domain modules
// and the HTTP layer. Domain code returns *Error values; the HTTP layer maps
// their Kind to a status code without inspecting messages.
package apperror

import (
	"errors"
	"fmt"
)

// Kind classifies an error for transport mapping.
type Kind int

const (
	KindInvalid Kind = iota + 1
	KindNotFound
	KindConflict
	KindUnavailable
)

// FieldError describes a validation problem with a single input field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Error is an expected, client-facing error. Its Message must never contain
// internal details such as SQL or stack traces.
type Error struct {
	Kind    Kind
	Code    string
	Message string
	Fields  []FieldError
}

func (e *Error) Error() string {
	if len(e.Fields) == 0 {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s (%d field errors)", e.Code, e.Message, len(e.Fields))
}

// As extracts an *Error from err, if any.
func As(err error) (*Error, bool) {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}

// NotFound reports a missing resource.
func NotFound(resource string) *Error {
	return &Error{Kind: KindNotFound, Code: resource + "_not_found", Message: "Recurso não encontrado."}
}

// Conflict reports a request incompatible with the current resource state.
func Conflict(code, message string) *Error {
	return &Error{Kind: KindConflict, Code: code, Message: message}
}

// Invalid reports a request-level validation failure without field details.
func Invalid(code, message string) *Error {
	return &Error{Kind: KindInvalid, Code: code, Message: message}
}

// Validator accumulates field errors so a client sees every problem at once.
type Validator struct {
	fields []FieldError
}

// Add records a field error.
func (v *Validator) Add(field, message string) {
	v.fields = append(v.fields, FieldError{Field: field, Message: message})
}

// Check records a field error when ok is false.
func (v *Validator) Check(ok bool, field, message string) {
	if !ok {
		v.Add(field, message)
	}
}

// Has reports whether a field already has an error.
func (v *Validator) Has(field string) bool {
	for _, f := range v.fields {
		if f.Field == field {
			return true
		}
	}
	return false
}

// Err returns a validation error, or nil when no field failed.
func (v *Validator) Err() error {
	if len(v.fields) == 0 {
		return nil
	}
	return &Error{
		Kind:    KindInvalid,
		Code:    "validation_failed",
		Message: "Os dados enviados são inválidos.",
		Fields:  v.fields,
	}
}
