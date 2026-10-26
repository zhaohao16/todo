package todo

import "context"

// Store persists todos. Implementations must be safe for concurrent use, and
// Create must treat the ID, CreatedAt and UpdatedAt of its argument as inputs
// to be replaced.
type Store interface {
	// Create stores t and returns it with an assigned ID and timestamps.
	Create(ctx context.Context, t Todo) (Todo, error)
	// Get returns the todo with the given ID, or ErrNotFound.
	Get(ctx context.Context, id int64) (Todo, error)
	// List returns every todo ordered by ascending ID.
	List(ctx context.Context) ([]Todo, error)
	// Update applies mutate to the stored todo under a lock and refreshes
	// UpdatedAt. It returns ErrNotFound when the ID is unknown.
	Update(ctx context.Context, id int64, mutate func(*Todo)) (Todo, error)
	// Delete removes the todo with the given ID, or returns ErrNotFound.
	Delete(ctx context.Context, id int64) error
}
