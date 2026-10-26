package todo

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

func TestValidateCreate(t *testing.T) {
	tests := []struct {
		name       string
		in         CreateInput
		wantFields []string
	}{
		{
			name: "valid minimal",
			in:   CreateInput{Title: "买牛奶"},
		},
		{
			name:       "empty title",
			in:         CreateInput{Title: ""},
			wantFields: []string{"title"},
		},
		{
			name:       "whitespace-only title",
			in:         CreateInput{Title: "   "},
			wantFields: []string{"title"},
		},
		{
			name: "title of exactly 200 CJK runes",
			in:   CreateInput{Title: strings.Repeat("字", 200)},
		},
		{
			name:       "title of 201 CJK runes",
			in:         CreateInput{Title: strings.Repeat("字", 201)},
			wantFields: []string{"title"},
		},
		{
			name: "title of 200 emoji counts runes not bytes",
			in:   CreateInput{Title: strings.Repeat("🎉", 200)},
		},
		{
			name: "description of exactly 2000 chars",
			in:   CreateInput{Title: "x", Description: strings.Repeat("a", 2000)},
		},
		{
			name:       "description of 2001 chars",
			in:         CreateInput{Title: "x", Description: strings.Repeat("a", 2001)},
			wantFields: []string{"description"},
		},
		{
			name:       "both fields invalid",
			in:         CreateInput{Title: " ", Description: strings.Repeat("a", 2001)},
			wantFields: []string{"title", "description"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreate(tt.in)
			if len(tt.wantFields) == 0 {
				if err != nil {
					t.Fatalf("ValidateCreate() = %v, want nil", err)
				}
				return
			}

			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("ValidateCreate() = %v, want *ValidationError", err)
			}
			if len(ve.Fields) != len(tt.wantFields) {
				t.Fatalf("got fields %v, want %v", fieldNames(ve.Fields), tt.wantFields)
			}
			for i, want := range tt.wantFields {
				if ve.Fields[i].Field != want {
					t.Errorf("fields[%d] = %q, want %q", i, ve.Fields[i].Field, want)
				}
			}
		})
	}
}

func TestValidatePatch(t *testing.T) {
	t.Run("empty patch is rejected with a request-level message", func(t *testing.T) {
		err := ValidatePatch(Patch{})

		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("ValidatePatch() = %v, want *ValidationError", err)
		}
		if ve.Message != "at least one field must be provided" {
			t.Errorf("Message = %q, want %q", ve.Message, "at least one field must be provided")
		}
		if len(ve.Fields) != 0 {
			t.Errorf("Fields = %v, want none", fieldNames(ve.Fields))
		}
	})

	t.Run("whitespace-only title is rejected", func(t *testing.T) {
		err := ValidatePatch(Patch{Title: strPtr("   ")})

		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("ValidatePatch() = %v, want *ValidationError", err)
		}
		if ve.Fields[0].Field != "title" {
			t.Errorf("field = %q, want title", ve.Fields[0].Field)
		}
	})

	t.Run("title of 201 runes is rejected", func(t *testing.T) {
		err := ValidatePatch(Patch{Title: strPtr(strings.Repeat("字", 201))})

		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("ValidatePatch() = %v, want *ValidationError", err)
		}
		if ve.Fields[0].Field != "title" {
			t.Errorf("field = %q, want title", ve.Fields[0].Field)
		}
	})

	t.Run("completed alone is accepted", func(t *testing.T) {
		if err := ValidatePatch(Patch{Completed: boolPtr(true)}); err != nil {
			t.Fatalf("ValidatePatch() = %v, want nil", err)
		}
	})

	t.Run("description of 2001 chars is rejected", func(t *testing.T) {
		err := ValidatePatch(Patch{Description: strPtr(strings.Repeat("a", 2001))})

		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("ValidatePatch() = %v, want *ValidationError", err)
		}
		if ve.Fields[0].Field != "description" {
			t.Errorf("field = %q, want description", ve.Fields[0].Field)
		}
	})
}

func TestApplyPatch(t *testing.T) {
	created := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	base := Todo{
		ID:          1,
		Title:       "old",
		Description: "desc",
		Completed:   false,
		CreatedAt:   created,
		UpdatedAt:   created,
	}

	t.Run("only the provided field changes", func(t *testing.T) {
		got := base
		ApplyPatch(&got, Patch{Completed: boolPtr(true)})

		if !got.Completed {
			t.Error("Completed = false, want true")
		}
		if got.Title != "old" || got.Description != "desc" {
			t.Errorf("untouched fields changed: title=%q description=%q", got.Title, got.Description)
		}
		if !got.CreatedAt.Equal(created) || !got.UpdatedAt.Equal(created) {
			t.Error("timestamps were modified by ApplyPatch")
		}
	})

	t.Run("title is trimmed", func(t *testing.T) {
		got := base
		ApplyPatch(&got, Patch{Title: strPtr("  买牛奶  ")})

		if got.Title != "买牛奶" {
			t.Errorf("Title = %q, want %q", got.Title, "买牛奶")
		}
	})

	t.Run("description may be cleared", func(t *testing.T) {
		got := base
		ApplyPatch(&got, Patch{Description: strPtr("")})

		if got.Description != "" {
			t.Errorf("Description = %q, want empty", got.Description)
		}
	})
}

func TestPatchIsEmpty(t *testing.T) {
	if !(Patch{}).IsEmpty() {
		t.Error("zero Patch IsEmpty() = false, want true")
	}
	if (Patch{Completed: boolPtr(false)}).IsEmpty() {
		t.Error("Patch with Completed IsEmpty() = true, want false")
	}
}

func fieldNames(fields []FieldError) []string {
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		out = append(out, f.Field)
	}
	return out
}
