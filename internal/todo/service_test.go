package todo

import (
	"context"
	"errors"
	"testing"
)

// fakeStore is a scriptable in-memory Store used to observe how Service calls
// its port. It never touches real state.
type fakeStore struct {
	item      Todo
	getErr    error
	deleteErr error
	createErr error

	createCalls int
	getCalls    int
	updateCalls int
	deleteCalls int
	createdTodo Todo
}

func (f *fakeStore) Create(_ context.Context, t Todo) (Todo, error) {
	f.createCalls++
	if f.createErr != nil {
		return Todo{}, f.createErr
	}
	f.createdTodo = t
	t.ID = 1
	return t, nil
}

func (f *fakeStore) Get(_ context.Context, _ int64) (Todo, error) {
	f.getCalls++
	if f.getErr != nil {
		return Todo{}, f.getErr
	}
	return f.item, nil
}

func (f *fakeStore) List(_ context.Context) ([]Todo, error) {
	return []Todo{f.item}, nil
}

func (f *fakeStore) Update(_ context.Context, _ int64, mutate func(*Todo)) (Todo, error) {
	f.updateCalls++
	item := f.item
	mutate(&item)
	f.item = item
	return item, nil
}

func (f *fakeStore) Delete(_ context.Context, _ int64) error {
	f.deleteCalls++
	return f.deleteErr
}

func TestServiceCreateRejectsInvalidInputBeforeStore(t *testing.T) {
	fs := &fakeStore{}
	svc := NewService(fs)

	_, err := svc.Create(context.Background(), CreateInput{Title: "  "})

	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Create() = %v, want *ValidationError", err)
	}
	if fs.createCalls != 0 {
		t.Errorf("store.Create called %d times, want 0", fs.createCalls)
	}
}

func TestServiceCreateTrimsTitle(t *testing.T) {
	fs := &fakeStore{}
	svc := NewService(fs)

	_, err := svc.Create(context.Background(), CreateInput{Title: "  买牛奶  "})
	if err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	if fs.createdTodo.Title != "买牛奶" {
		t.Errorf("stored Title = %q, want %q", fs.createdTodo.Title, "买牛奶")
	}
}

func TestServiceGetPropagatesNotFound(t *testing.T) {
	fs := &fakeStore{getErr: ErrNotFound}
	svc := NewService(fs)

	_, err := svc.Get(context.Background(), 7)

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() = %v, want ErrNotFound", err)
	}
}

func TestServiceGetRejectsNonPositiveID(t *testing.T) {
	for _, id := range []int64{0, -1} {
		fs := &fakeStore{}
		svc := NewService(fs)

		_, err := svc.Get(context.Background(), id)

		if !errors.Is(err, ErrNotFound) {
			t.Errorf("Get(%d) = %v, want ErrNotFound", id, err)
		}
		if fs.getCalls != 0 {
			t.Errorf("Get(%d) reached the store, want it to short-circuit", id)
		}
	}
}

func TestServiceUpdateAppliesOnlyProvidedFields(t *testing.T) {
	fs := &fakeStore{item: Todo{ID: 1, Title: "old", Description: "desc"}}
	svc := NewService(fs)

	got, err := svc.Update(context.Background(), 1, Patch{Completed: boolPtr(true)})
	if err != nil {
		t.Fatalf("Update() = %v, want nil", err)
	}
	if !got.Completed {
		t.Error("Completed = false, want true")
	}
	if got.Title != "old" || got.Description != "desc" {
		t.Errorf("untouched fields changed: title=%q description=%q", got.Title, got.Description)
	}
}

func TestServiceUpdateRejectsEmptyPatchBeforeStore(t *testing.T) {
	fs := &fakeStore{}
	svc := NewService(fs)

	_, err := svc.Update(context.Background(), 1, Patch{})

	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Update() = %v, want *ValidationError", err)
	}
	if fs.updateCalls != 0 {
		t.Errorf("store.Update called %d times, want 0", fs.updateCalls)
	}
}

func TestServiceDeletePropagatesNotFound(t *testing.T) {
	fs := &fakeStore{deleteErr: ErrNotFound}
	svc := NewService(fs)

	err := svc.Delete(context.Background(), 7)

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete() = %v, want ErrNotFound", err)
	}
}
