package todo

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotFound is returned when a todo does not exist.
var ErrNotFound = errors.New("todo not found")

// FieldError describes one invalid field.
type FieldError struct {
	Field   string
	Message string
}

// ValidationError reports that a request failed validation. Message carries a
// request-level explanation; Fields carries per-field explanations.
type ValidationError struct {
	Message string
	Fields  []FieldError
}

// Error implements error.
func (e *ValidationError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = "validation failed"
	}
	if len(e.Fields) == 0 {
		return msg
	}
	parts := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		parts = append(parts, fmt.Sprintf("%s: %s", f.Field, f.Message))
	}
	return msg + ": " + strings.Join(parts, "; ")
}

// Add records an invalid field.
func (e *ValidationError) Add(field, msg string) {
	e.Fields = append(e.Fields, FieldError{Field: field, Message: msg})
}
