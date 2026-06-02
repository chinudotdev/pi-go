package session

import (
	"context"
	"strings"
	"sync"

	"github.com/chinudotdev/pi-go/agent/harness"
)

// InMemorySessionStorage is an in-memory SessionStorage implementation.
type InMemorySessionStorage struct {
	mu         sync.RWMutex
	metadata   harness.SessionMetadata
	entries    []harness.SessionTreeEntry
	byID       map[string]*harness.SessionTreeEntry
	labelsByID map[string]string
	leafID     *string
}

// NewInMemorySessionStorage creates a new in-memory session storage.
func NewInMemorySessionStorage(opts *InMemoryStorageOptions) *InMemorySessionStorage {
	if opts == nil {
		opts = &InMemoryStorageOptions{}
	}

	entries := opts.Entries
	if entries == nil {
		entries = []harness.SessionTreeEntry{}
	}

	byID := make(map[string]*harness.SessionTreeEntry, len(entries))
	labelsByID := make(map[string]string)
	var leafID *string

	for i := range entries {
		byID[entries[i].ID] = &entries[i]
		updateLabelCache(labelsByID, &entries[i])
		lid := leafIDAfterEntry(&entries[i])
		if lid != nil {
			leafID = lid
		}
	}

	if leafID != nil {
		if _, ok := byID[*leafID]; !ok {
			panic(harness.NewSessionError(harness.SessionErrorInvalidSession, "Entry "+*leafID+" not found", nil))
		}
	}

	meta := opts.Metadata
	if meta == nil {
		id := UUIDv7()
		meta = &harness.SessionMetadata{
			ID:        id,
			CreatedAt: harness.NowISO(),
		}
	}

	return &InMemorySessionStorage{
		metadata:   *meta,
		entries:    entries,
		byID:       byID,
		labelsByID: labelsByID,
		leafID:     leafID,
	}
}

// InMemoryStorageOptions configures in-memory storage creation.
type InMemoryStorageOptions struct {
	Entries  []harness.SessionTreeEntry
	Metadata *harness.SessionMetadata
}

// GetMetadata returns the session metadata.
func (s *InMemorySessionStorage) GetMetadata(_ context.Context) (harness.SessionMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.metadata, nil
}

// GetLeafID returns the current leaf entry ID.
func (s *InMemorySessionStorage) GetLeafID(_ context.Context) (*string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.leafID != nil {
		if _, ok := s.byID[*s.leafID]; !ok {
			return nil, harness.NewSessionError(harness.SessionErrorInvalidSession, "Entry "+*s.leafID+" not found", nil)
		}
	}
	return s.leafID, nil
}

// SetLeafId persists a leaf entry.
func (s *InMemorySessionStorage) SetLeafID(_ context.Context, leafID *string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if leafID != nil {
		if _, ok := s.byID[*leafID]; !ok {
			return harness.NewSessionError(harness.SessionErrorNotFound, "Entry "+*leafID+" not found", nil)
		}
	}

	entry := harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{
			Type:      "leaf",
			ID:        generateEntryID(s.byID),
			ParentID:  s.leafID,
			Timestamp: harness.NowISO(),
		},
		TargetID: leafID,
	}
	s.entries = append(s.entries, entry)
	s.byID[entry.ID] = &s.entries[len(s.entries)-1]
	s.leafID = leafID
	return nil
}

// CreateEntryID generates a new unique entry ID.
func (s *InMemorySessionStorage) CreateEntryID(_ context.Context) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return generateEntryID(s.byID), nil
}

// AppendEntry appends an entry to the session tree.
func (s *InMemorySessionStorage) AppendEntry(_ context.Context, entry harness.SessionTreeEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries = append(s.entries, entry)
	s.byID[entry.ID] = &s.entries[len(s.entries)-1]
	updateLabelCache(s.labelsByID, &entry)
	lid := leafIDAfterEntry(&entry)
	if lid != nil {
		s.leafID = lid
	}
	return nil
}

// GetEntry returns an entry by ID.
func (s *InMemorySessionStorage) GetEntry(_ context.Context, id string) (*harness.SessionTreeEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.byID[id], nil
}

// FindEntries returns all entries of a given type.
func (s *InMemorySessionStorage) FindEntries(_ context.Context, entryType string) ([]harness.SessionTreeEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []harness.SessionTreeEntry
	for _, e := range s.entries {
		if e.Type == entryType {
			result = append(result, e)
		}
	}
	return result, nil
}

// GetLabel returns the label for an entry.
func (s *InMemorySessionStorage) GetLabel(_ context.Context, id string) (*string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if label, ok := s.labelsByID[id]; ok {
		return &label, nil
	}
	return nil, nil
}

// GetPathToRoot returns entries from the given leaf to the root.
func (s *InMemorySessionStorage) GetPathToRoot(_ context.Context, leafID *string) ([]harness.SessionTreeEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if leafID == nil {
		return nil, nil
	}

	var path []harness.SessionTreeEntry
	current := s.byID[*leafID]
	if current == nil {
		return nil, harness.NewSessionError(harness.SessionErrorNotFound, "Entry "+*leafID+" not found", nil)
	}

	for current != nil {
		path = append([]harness.SessionTreeEntry{*current}, path...)
		if current.ParentID == nil {
			break
		}
		parent := s.byID[*current.ParentID]
		if parent == nil {
			return nil, harness.NewSessionError(harness.SessionErrorInvalidSession, "Entry "+*current.ParentID+" not found", nil)
		}
		current = parent
	}
	return path, nil
}

// GetEntries returns all entries in chronological order.
func (s *InMemorySessionStorage) GetEntries(_ context.Context) ([]harness.SessionTreeEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]harness.SessionTreeEntry, len(s.entries))
	copy(result, s.entries)
	return result, nil
}

// ============================================================================
// Internal helpers
// ============================================================================

func generateEntryID(byID map[string]*harness.SessionTreeEntry) string {
	for i := 0; i < 100; i++ {
		id := ShortID()
		if _, ok := byID[id]; !ok {
			return id
		}
	}
	return UUIDv7()
}

func leafIDAfterEntry(entry *harness.SessionTreeEntry) *string {
	if entry.Type == "leaf" {
		return entry.TargetID
	}
	return &entry.ID
}

func updateLabelCache(labelsByID map[string]string, entry *harness.SessionTreeEntry) {
	if entry.Type != "label" {
		return
	}
	if entry.TargetID == nil {
		return
	}
	targetID := *entry.TargetID
	if entry.Label != nil {
		trimmed := strings.TrimSpace(*entry.Label)
		if trimmed != "" {
			labelsByID[targetID] = trimmed
			return
		}
	}
	delete(labelsByID, targetID)
}
