package session

import (
	"context"

	"github.com/chinudotdev/pi-go/agent/harness"
)

// InMemorySessionRepo is an in-memory SessionRepo implementation.
type InMemorySessionRepo struct {
	sessions map[string]*Session
}

// NewInMemorySessionRepo creates a new in-memory session repository.
func NewInMemorySessionRepo() *InMemorySessionRepo {
	return &InMemorySessionRepo{
		sessions: make(map[string]*Session),
	}
}

// Create creates a new session.
func (r *InMemorySessionRepo) Create(ctx context.Context, options *SessionCreateOptions) (*Session, error) {
	var id string
	if options != nil && options.ID != nil {
		id = *options.ID
	} else {
		id = UUIDv7()
	}

	metadata := harness.SessionMetadata{
		ID:        id,
		CreatedAt: harness.NowISO(),
	}
	storage := NewInMemorySessionStorage(&InMemoryStorageOptions{
		Metadata: &metadata,
	})
	session := NewSession(storage)
	r.sessions[metadata.ID] = session
	return session, nil
}

// Open opens an existing session by metadata.
func (r *InMemorySessionRepo) Open(_ context.Context, metadata harness.SessionMetadata) (*Session, error) {
	session, ok := r.sessions[metadata.ID]
	if !ok {
		return nil, harness.NewSessionError(harness.SessionErrorNotFound, "Session not found: "+metadata.ID, nil)
	}
	return session, nil
}

// List returns metadata for all sessions.
func (r *InMemorySessionRepo) List(ctx context.Context) ([]harness.SessionMetadata, error) {
	result := make([]harness.SessionMetadata, 0, len(r.sessions))
	for _, session := range r.sessions {
		meta, err := session.GetMetadata(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, meta)
	}
	return result, nil
}

// Delete deletes a session by metadata.
func (r *InMemorySessionRepo) Delete(_ context.Context, metadata harness.SessionMetadata) error {
	delete(r.sessions, metadata.ID)
	return nil
}

// Fork forks a session at a given entry point.
func (r *InMemorySessionRepo) Fork(ctx context.Context, sourceMeta harness.SessionMetadata, options SessionForkOptions) (*Session, error) {
	source, err := r.Open(ctx, sourceMeta)
	if err != nil {
		return nil, err
	}

	forkedEntries, err := GetEntriesToFork(ctx, source.GetStorage(), options)
	if err != nil {
		return nil, err
	}

	var id string
	if options.ID != nil {
		id = *options.ID
	} else {
		id = UUIDv7()
	}

	metadata := harness.SessionMetadata{
		ID:        id,
		CreatedAt: harness.NowISO(),
	}
	storage := NewInMemorySessionStorage(&InMemoryStorageOptions{
		Entries:  forkedEntries,
		Metadata: &metadata,
	})
	session := NewSession(storage)
	r.sessions[metadata.ID] = session
	return session, nil
}

// GetEntriesToFork returns the entries that should be copied when forking.
func GetEntriesToFork(ctx context.Context, storage SessionStorage, options SessionForkOptions) ([]harness.SessionTreeEntry, error) {
	if options.EntryID == nil {
		return storage.GetEntries(ctx)
	}

	target, err := storage.GetEntry(ctx, *options.EntryID)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, harness.NewSessionError(harness.SessionErrorInvalidForkTarget, "Entry "+*options.EntryID+" not found", nil)
	}

	var effectiveLeafID *string
	position := options.Position
	if position == "" {
		position = "before"
	}

	if position == "at" {
		effectiveLeafID = &target.ID
	} else {
		// "before" — go to parent if target is a user message
		if target.Type == "message" && target.Message != nil && target.Message.Role == "user" {
			effectiveLeafID = target.ParentID
		} else {
			return nil, harness.NewSessionError(harness.SessionErrorInvalidForkTarget, "Entry "+*options.EntryID+" is not a user message", nil)
		}
	}

	return storage.GetPathToRoot(ctx, effectiveLeafID)
}
