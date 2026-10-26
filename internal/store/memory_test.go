package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"example.com/todo-api/internal/store"
	"example.com/todo-api/internal/todo"
)

func fixedClock(ts time.Time) func() time.Time {
	return func() time.Time { return ts }
}

func TestMemoryCreateAssignsSequentialIDs(t *testing.T) {
	ts := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	m := store.NewMemory(fixedClock(ts))
	ctx := context.Background()

	first, err := m.Create(ctx, todo.Todo{Title: "first"})
	if err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	second, err := m.Create(ctx, todo.Todo{Title: "second"})
	if err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}

	if first.ID != 1 || second.ID != 2 {
		t.Errorf("IDs = %d, %d; want 1, 2", first.ID, second.ID)
	}
	if !first.CreatedAt.Equal(ts) || !first.UpdatedAt.Equal(ts) {
		t.Errorf("timestamps = %v / %v, want both %v", first.CreatedAt, first.UpdatedAt, ts)
	}
}

func TestMemoryGet(t *testing.T) {
	m := store.NewMemory(nil)
	ctx := context.Background()

	created, err := m.Create(ctx, todo.Todo{Title: "buy milk"})
	if err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}

	got, err := m.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() = %v, want nil", err)
	}
	if got.Title != "buy milk" {
		t.Errorf("Title = %q, want %q", got.Title, "buy milk")
	}

	if _, err := m.Get(ctx, 999); !errors.Is(err, todo.ErrNotFound) {
		t.Errorf("Get(999) = %v, want todo.ErrNotFound", err)
	}
}

func TestMemoryListIsOrderedAndCopied(t *testing.T) {
	m := store.NewMemory(nil)
	ctx := context.Background()

	for _, title := range []string{"a", "b", "c"} {
		if _, err := m.Create(ctx, todo.Todo{Title: title}); err != nil {
			t.Fatalf("Create() = %v, want nil", err)
		}
	}

	items, err := m.List(ctx)
	if err != nil {
		t.Fatalf("List() = %v, want nil", err)
	}
	if len(items) != 3 {
		t.Fatalf("len(List()) = %d, want 3", len(items))
	}
	for i := 1; i < len(items); i++ {
		if items[i-1].ID >= items[i].ID {
			t.Fatalf("List() not ordered by ID: %v", items)
		}
	}

	items[0].Title = "mutated by caller"
	got, err := m.Get(ctx, items[0].ID)
	if err != nil {
		t.Fatalf("Get() = %v, want nil", err)
	}
	if got.Title == "mutated by caller" {
		t.Error("mutating the slice returned by List changed stored state")
	}
}

func TestMemoryListOfEmptyStoreIsNotNil(t *testing.T) {
	m := store.NewMemory(nil)

	items, err := m.List(context.Background())
	if err != nil {
		t.Fatalf("List() = %v, want nil", err)
	}
	if items == nil {
		t.Fatal("List() = nil, want empty non-nil slice")
	}
	if len(items) != 0 {
		t.Errorf("len(List()) = %d, want 0", len(items))
	}
}

func TestMemoryUpdateRefreshesUpdatedAtOnly(t *testing.T) {
	created := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	now := created
	m := store.NewMemory(func() time.Time { return now })
	ctx := context.Background()

	todoItem, err := m.Create(ctx, todo.Todo{Title: "old"})
	if err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}

	now = created.Add(time.Minute)
	updated, err := m.Update(ctx, todoItem.ID, func(cur *todo.Todo) {
		cur.Title = "new"
		cur.Completed = true
	})
	if err != nil {
		t.Fatalf("Update() = %v, want nil", err)
	}

	if updated.Title != "new" || !updated.Completed {
		t.Errorf("Update() = %+v, want title=new completed=true", updated)
	}
	if !updated.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt = %v, want unchanged %v", updated.CreatedAt, created)
	}
	if !updated.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt = %v, want %v", updated.UpdatedAt, now)
	}
}

func TestMemoryUpdateNotFound(t *testing.T) {
	m := store.NewMemory(nil)

	_, err := m.Update(context.Background(), 999, func(*todo.Todo) {})

	if !errors.Is(err, todo.ErrNotFound) {
		t.Errorf("Update(999) = %v, want todo.ErrNotFound", err)
	}
}

func TestMemoryDelete(t *testing.T) {
	m := store.NewMemory(nil)
	ctx := context.Background()

	created, err := m.Create(ctx, todo.Todo{Title: "doomed"})
	if err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}

	if err := m.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete() = %v, want nil", err)
	}
	if _, err := m.Get(ctx, created.ID); !errors.Is(err, todo.ErrNotFound) {
		t.Errorf("Get() after Delete = %v, want todo.ErrNotFound", err)
	}
	if err := m.Delete(ctx, 999); !errors.Is(err, todo.ErrNotFound) {
		t.Errorf("Delete(999) = %v, want todo.ErrNotFound", err)
	}
}

// TestMemoryConcurrentUpdate pins the read-modify-write behaviour under
// contention: 1000 toggles are an even number, so the final value must be false.
func TestMemoryConcurrentUpdate(t *testing.T) {
	m := store.NewMemory(nil)
	ctx := context.Background()

	created, err := m.Create(ctx, todo.Todo{Title: "shared"})
	if err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}

	const goroutines, togglesEach = 50, 20

	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range togglesEach {
				_, err := m.Update(ctx, created.ID, func(cur *todo.Todo) {
					cur.Completed = !cur.Completed
				})
				if err != nil {
					t.Errorf("Update() = %v, want nil", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	got, err := m.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() = %v, want nil", err)
	}
	if got.Completed {
		t.Errorf("Completed = true after %d toggles, want false", goroutines*togglesEach)
	}
}
