package todo

import (
	"context"
	"errors"
	"testing"
)

func TestValidationErrorError(t *testing.T) {
	tests := []struct {
		name string
		err  *ValidationError
		want string
	}{
		{
			name: "fields are joined",
			err:  &ValidationError{Fields: []FieldError{{Field: "title", Message: "must not be empty"}}},
			want: "validation failed: title: must not be empty",
		},
		{
			name: "request-level message without fields",
			err:  &ValidationError{Message: "at least one field must be provided"},
			want: "at least one field must be provided",
		},
		{
			name: "empty error",
			err:  &ValidationError{},
			want: "validation failed",
		},
		{
			name: "request-level message with fields",
			err: &ValidationError{
				Message: "invalid request",
				Fields:  []FieldError{{Field: "a", Message: "x"}, {Field: "b", Message: "y"}},
			},
			want: "invalid request: a: x; b: y",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

// stubStore is a Store built from function fields, so a test can inject exactly
// the failure it cares about.
type stubStore struct {
	createFn func(context.Context, Todo) (Todo, error)
	getFn    func(context.Context, int64) (Todo, error)
	listFn   func(context.Context) ([]Todo, error)
	updateFn func(context.Context, int64, func(*Todo)) (Todo, error)
	deleteFn func(context.Context, int64) error
}

func (s *stubStore) Create(ctx context.Context, t Todo) (Todo, error) {
	return s.createFn(ctx, t)
}

func (s *stubStore) Get(ctx context.Context, id int64) (Todo, error) {
	return s.getFn(ctx, id)
}

func (s *stubStore) List(ctx context.Context) ([]Todo, error) {
	return s.listFn(ctx)
}

func (s *stubStore) Update(ctx context.Context, id int64, mutate func(*Todo)) (Todo, error) {
	return s.updateFn(ctx, id, mutate)
}

func (s *stubStore) Delete(ctx context.Context, id int64) error {
	return s.deleteFn(ctx, id)
}

func TestNewServiceRejectsNilStore(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewService(nil) did not panic")
		}
	}()

	NewService(nil)
}

// TestServicePropagatesStoreFailures pins that every use case wraps its store
// error with %w, so callers can still match with errors.Is.
func TestServicePropagatesStoreFailures(t *testing.T) {
	sentinel := errors.New("backing store unavailable")
	ctx := context.Background()

	failing := &stubStore{
		createFn: func(context.Context, Todo) (Todo, error) { return Todo{}, sentinel },
		getFn:    func(context.Context, int64) (Todo, error) { return Todo{}, sentinel },
		listFn:   func(context.Context) ([]Todo, error) { return nil, sentinel },
		updateFn: func(context.Context, int64, func(*Todo)) (Todo, error) { return Todo{}, sentinel },
		deleteFn: func(context.Context, int64) error { return sentinel },
	}
	svc := NewService(failing)

	tests := []struct {
		name string
		call func() error
	}{
		{"Create", func() error { _, err := svc.Create(ctx, CreateInput{Title: "x"}); return err }},
		{"List", func() error { _, err := svc.List(ctx); return err }},
		{"Get", func() error { _, err := svc.Get(ctx, 1); return err }},
		{"Update", func() error {
			_, err := svc.Update(ctx, 1, Patch{Completed: boolPtr(true)})
			return err
		}},
		{"Delete", func() error { return svc.Delete(ctx, 1) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if !errors.Is(err, sentinel) {
				t.Errorf("%s error = %v, want it to wrap the store error", tt.name, err)
			}
		})
	}
}

func TestServiceListReturnsEmptySliceNotNil(t *testing.T) {
	svc := NewService(&stubStore{
		listFn: func(context.Context) ([]Todo, error) { return nil, nil },
	})

	items, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List() = %v, want nil", err)
	}
	if items == nil {
		t.Fatal("List() = nil, want an empty non-nil slice so it encodes as []")
	}
}

func TestServiceRejectsNonPositiveIDOnUpdateAndDelete(t *testing.T) {
	// Both methods must short-circuit before touching the store.
	svc := NewService(&stubStore{
		updateFn: func(context.Context, int64, func(*Todo)) (Todo, error) {
			t.Fatal("Update reached the store with a non-positive id")
			return Todo{}, nil
		},
		deleteFn: func(context.Context, int64) error {
			t.Fatal("Delete reached the store with a non-positive id")
			return nil
		},
	})

	for _, id := range []int64{0, -1} {
		if _, err := svc.Update(context.Background(), id, Patch{Completed: boolPtr(true)}); !errors.Is(err, ErrNotFound) {
			t.Errorf("Update(%d) = %v, want ErrNotFound", id, err)
		}
		if err := svc.Delete(context.Background(), id); !errors.Is(err, ErrNotFound) {
			t.Errorf("Delete(%d) = %v, want ErrNotFound", id, err)
		}
	}
}
