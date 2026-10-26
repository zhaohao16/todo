package todo

import (
	"context"
	"fmt"
	"strings"
)

// Service implements the todo use cases on top of a Store.
type Service struct {
	store Store
}

// NewService returns a Service backed by s.
func NewService(s Store) *Service {
	if s == nil {
		panic("todo: NewService called with a nil Store")
	}
	return &Service{store: s}
}

// Create validates in and stores a new todo.
func (s *Service) Create(ctx context.Context, in CreateInput) (Todo, error) {
	in.Title = strings.TrimSpace(in.Title)
	if err := ValidateCreate(in); err != nil {
		return Todo{}, err
	}

	t, err := s.store.Create(ctx, Todo{
		Title:       in.Title,
		Description: in.Description,
		Completed:   in.Completed,
	})
	if err != nil {
		return Todo{}, fmt.Errorf("create todo: %w", err)
	}
	return t, nil
}

// List returns every todo ordered by ascending ID.
func (s *Service) List(ctx context.Context) ([]Todo, error) {
	items, err := s.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list todos: %w", err)
	}
	if items == nil {
		items = []Todo{}
	}
	return items, nil
}

// Get returns one todo, or ErrNotFound.
func (s *Service) Get(ctx context.Context, id int64) (Todo, error) {
	if id <= 0 {
		return Todo{}, ErrNotFound
	}

	t, err := s.store.Get(ctx, id)
	if err != nil {
		return Todo{}, fmt.Errorf("get todo %d: %w", id, err)
	}
	return t, nil
}

// Update applies p to an existing todo.
func (s *Service) Update(ctx context.Context, id int64, p Patch) (Todo, error) {
	if id <= 0 {
		return Todo{}, ErrNotFound
	}
	if err := ValidatePatch(p); err != nil {
		return Todo{}, err
	}

	t, err := s.store.Update(ctx, id, func(cur *Todo) {
		ApplyPatch(cur, p)
	})
	if err != nil {
		return Todo{}, fmt.Errorf("update todo %d: %w", id, err)
	}
	return t, nil
}

// Delete removes a todo, or returns ErrNotFound.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrNotFound
	}
	if err := s.store.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete todo %d: %w", id, err)
	}
	return nil
}
