package session

import (
	"context"

	"github.com/chinudotdev/pi-go/agent/harness"
)

// SessionStorage is the interface for persisting session tree entries.
// Implementations must be safe for concurrent use.
type SessionStorage interface {
	// GetMetadata returns the session metadata.
	GetMetadata(ctx context.Context) (harness.SessionMetadata, error)
	// GetLeafID returns the current leaf entry ID, or nil if empty.
	GetLeafID(ctx context.Context) (*string, error)
	// SetLeafID persists a leaf entry recording the active leaf.
	SetLeafID(ctx context.Context, leafID *string) error
	// CreateEntryID generates a new unique entry ID.
	CreateEntryID(ctx context.Context) (string, error)
	// AppendEntry appends an entry to the session tree.
	AppendEntry(ctx context.Context, entry harness.SessionTreeEntry) error
	// GetEntry returns an entry by ID, or nil if not found.
	GetEntry(ctx context.Context, id string) (*harness.SessionTreeEntry, error)
	// FindEntries returns all entries of a given type.
	FindEntries(ctx context.Context, entryType string) ([]harness.SessionTreeEntry, error)
	// GetLabel returns the label for an entry, if any.
	GetLabel(ctx context.Context, id string) (*string, error)
	// GetPathToRoot returns the entries from the given leaf to the root.
	GetPathToRoot(ctx context.Context, leafID *string) ([]harness.SessionTreeEntry, error)
	// GetEntries returns all entries in chronological order.
	GetEntries(ctx context.Context) ([]harness.SessionTreeEntry, error)
}
