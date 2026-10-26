// Package todo holds the Todo entity, its validation rules and the use cases
// that operate on it. It knows nothing about HTTP or about any particular
// storage backend.
package todo

import (
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// MaxTitleRunes is the maximum length of a todo title, in runes.
	MaxTitleRunes = 200
	// MaxDescriptionRunes is the maximum length of a todo description, in runes.
	MaxDescriptionRunes = 2000
)

// Todo is a single task.
type Todo struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Completed   bool      `json:"completed"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// CreateInput carries the caller-supplied fields for a new todo.
type CreateInput struct {
	Title       string
	Description string
	Completed   bool
}

// Patch carries the fields to change. A nil field is left untouched.
type Patch struct {
	Title       *string
	Description *string
	Completed   *bool
}

// IsEmpty reports whether the patch changes nothing at all.
func (p Patch) IsEmpty() bool {
	return p.Title == nil && p.Description == nil && p.Completed == nil
}

// ValidateCreate checks in and returns a *ValidationError listing every invalid
// field, or nil when in is acceptable.
func ValidateCreate(in CreateInput) error {
	var errs ValidationError

	title := strings.TrimSpace(in.Title)
	switch {
	case title == "":
		errs.Add("title", "must not be empty")
	case utf8.RuneCountInString(title) > MaxTitleRunes:
		errs.Add("title", "must be between 1 and 200 characters")
	}
	if utf8.RuneCountInString(in.Description) > MaxDescriptionRunes {
		errs.Add("description", "must be at most 2000 characters")
	}

	if len(errs.Fields) > 0 {
		return &errs
	}
	return nil
}

// ValidatePatch checks p and returns a *ValidationError listing every invalid
// field, or nil when p may be applied. A patch with no fields is rejected.
func ValidatePatch(p Patch) error {
	var errs ValidationError

	if p.IsEmpty() {
		errs.Message = "at least one field must be provided"
		return &errs
	}

	if p.Title != nil {
		title := strings.TrimSpace(*p.Title)
		switch {
		case title == "":
			errs.Add("title", "must not be empty")
		case utf8.RuneCountInString(title) > MaxTitleRunes:
			errs.Add("title", "must be between 1 and 200 characters")
		}
	}
	if p.Description != nil && utf8.RuneCountInString(*p.Description) > MaxDescriptionRunes {
		errs.Add("description", "must be at most 2000 characters")
	}

	if len(errs.Fields) > 0 {
		return &errs
	}
	return nil
}

// ApplyPatch changes the non-nil fields of t. Title is trimmed. It does not
// validate: callers must run ValidatePatch first.
func ApplyPatch(t *Todo, p Patch) {
	if p.Title != nil {
		t.Title = strings.TrimSpace(*p.Title)
	}
	if p.Description != nil {
		t.Description = *p.Description
	}
	if p.Completed != nil {
		t.Completed = *p.Completed
	}
}
