package session

import (
	"context"

	"github.com/chinudotdev/pi-go/agent/harness"
)

// SessionCreateOptions configures session creation.
type SessionCreateOptions struct {
	ID *string
}

// SessionForkOptions configures session forking.
type SessionForkOptions struct {
	EntryID  *string
	Position string // "before" or "at"
	ID       *string
}

// SessionRepo is the interface for session lifecycle management.
type SessionRepo interface {
	Create(ctx context.Context, options *SessionCreateOptions) (*Session, error)
	Open(ctx context.Context, metadata harness.SessionMetadata) (*Session, error)
	List(ctx context.Context) ([]harness.SessionMetadata, error)
	Delete(ctx context.Context, metadata harness.SessionMetadata) error
	Fork(ctx context.Context, source harness.SessionMetadata, options SessionForkOptions) (*Session, error)
}
