// Package store provides storage implementations for todos.
package store

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"

	"example.com/todo-api/internal/todo"
)

// Memory is an in-memory todo.Store safe for concurrent use. The zero value is
// not usable; call NewMemory.
type Memory struct {
	mu     sync.RWMutex
	items  map[int64]todo.Todo
	nextID int64
	now    func() time.Time
}

var _ todo.Store = (*Memory)(nil)

// NewMemory returns an empty store. now supplies timestamps and falls back to
// time.Now when nil.
func NewMemory(now func() time.Time) *Memory {
	if now == nil {
		now = time.Now
	}
	return &Memory{
		items: make(map[int64]todo.Todo),
		now:   now,
	}
}

// Create stores t, assigning the next ID and setting both timestamps.
func (m *Memory) Create(_ context.Context, t todo.Todo) (todo.Todo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextID++
	ts := m.now()

	t.ID = m.nextID
	t.CreatedAt = ts
	t.UpdatedAt = ts
	m.items[t.ID] = t

	return t, nil
}

// Get returns the todo with the given ID, or todo.ErrNotFound.
func (m *Memory) Get(_ context.Context, id int64) (todo.Todo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	t, ok := m.items[id]
	if !ok {
		return todo.Todo{}, todo.ErrNotFound
	}
	return t, nil
}

// List returns every todo ordered by ascending ID. The returned slice is a copy
// and never nil.
func (m *Memory) List(_ context.Context) ([]todo.Todo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]todo.Todo, 0, len(m.items))
	for _, t := range m.items {
		out = append(out, t)
	}
	slices.SortFunc(out, func(a, b todo.Todo) int { return cmp.Compare(a.ID, b.ID) })

	return out, nil
}

// Update applies mutate to the stored todo under the write lock and refreshes
// UpdatedAt. mutate runs while the lock is held, so read-modify-write is atomic.
func (m *Memory) Update(_ context.Context, id int64, mutate func(*todo.Todo)) (todo.Todo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, ok := m.items[id]
	if !ok {
		return todo.Todo{}, todo.ErrNotFound
	}

	mutate(&t)
	t.UpdatedAt = m.now()
	m.items[id] = t

	return t, nil
}

// Delete removes the todo with the given ID, or returns todo.ErrNotFound.
func (m *Memory) Delete(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.items[id]; !ok {
		return todo.ErrNotFound
	}
	delete(m.items, id)

	return nil
}
