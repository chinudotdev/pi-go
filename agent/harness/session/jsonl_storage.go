package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/chinudotdev/pi-go/agent/harness"
)

// jsonlSessionHeader is the first line of a JSONL session file.
type jsonlSessionHeader struct {
	Type          string  `json:"type"`
	Version       int     `json:"version"`
	ID            string  `json:"id"`
	Timestamp     string  `json:"timestamp"`
	Cwd           string  `json:"cwd"`
	ParentSession *string `json:"parentSession,omitempty"`
}

// JsonlFileSystem is the subset of harness.FileSystem needed by JSONL storage.
type JsonlFileSystem interface {
	ReadTextFile(ctx context.Context, path string) harness.Result[string]
	ReadTextLines(ctx context.Context, path string, maxLines int) harness.Result[[]string]
	WriteFile(ctx context.Context, path string, content []byte) harness.Result[struct{}]
	AppendFile(ctx context.Context, path string, content []byte) harness.Result[struct{}]
}

// JsonlRepoFileSystem is the subset of harness.FileSystem needed by JSONL repo.
type JsonlRepoFileSystem interface {
	JsonlFileSystem
	Cwd() string
	AbsolutePath(ctx context.Context, path string) harness.Result[string]
	JoinPath(ctx context.Context, parts ...string) harness.Result[string]
	ListDir(ctx context.Context, path string) harness.Result[[]harness.FileInfo]
	Exists(ctx context.Context, path string) harness.Result[bool]
	CreateDir(ctx context.Context, path string, recursive bool) harness.Result[struct{}]
	Remove(ctx context.Context, path string, recursive bool, force bool) harness.Result[struct{}]
}

// JsonlSessionStorage implements SessionStorage backed by a JSONL file.
type JsonlSessionStorage struct {
	mu         sync.Mutex
	fs         JsonlFileSystem
	filePath   string
	metadata   harness.JsonlSessionMetadata
	entries    []harness.SessionTreeEntry
	byID       map[string]*harness.SessionTreeEntry
	labelsByID map[string]string
	leafID     *string
}

// NewJsonlSessionStorage creates (creates the file) or opens an existing JSONL session.
// Use OpenJsonlSession or CreateJsonlSession instead.
func newJsonlSessionStorage(
	fs JsonlFileSystem,
	filePath string,
	header jsonlSessionHeader,
	entries []harness.SessionTreeEntry,
	leafID *string,
) *JsonlSessionStorage {
	meta := harness.JsonlSessionMetadata{
		SessionMetadata: harness.SessionMetadata{
			ID:        header.ID,
			CreatedAt: header.Timestamp,
		},
		Cwd:               header.Cwd,
		Path:              filePath,
		ParentSessionPath: header.ParentSession,
	}

	byID := make(map[string]*harness.SessionTreeEntry, len(entries))
	labelsByID := make(map[string]string)
	for i := range entries {
		byID[entries[i].ID] = &entries[i]
		updateLabelCache(labelsByID, &entries[i])
	}

	return &JsonlSessionStorage{
		fs:         fs,
		filePath:   filePath,
		metadata:   meta,
		entries:    entries,
		byID:       byID,
		labelsByID: labelsByID,
		leafID:     leafID,
	}
}

// OpenJsonlSession opens an existing JSONL session file.
func OpenJsonlSession(ctx context.Context, fs JsonlFileSystem, filePath string) (*JsonlSessionStorage, error) {
	result := fs.ReadTextFile(ctx, filePath)
	if !result.OK {
		return nil, harness.NewSessionError(harness.SessionErrorStorage,
			fmt.Sprintf("Failed to read session %s: %s", filePath, result.Err.Error()), result.Err)
	}

	lines := nonEmptyLines(result.Value)
	if len(lines) == 0 {
		return nil, harness.NewSessionError(harness.SessionErrorInvalidSession,
			fmt.Sprintf("Invalid JSONL session file %s: missing session header", filePath), nil)
	}

	header, err := parseHeaderLine(lines[0], filePath)
	if err != nil {
		return nil, err
	}

	var entries []harness.SessionTreeEntry
	var leafID *string
	for i := 1; i < len(lines); i++ {
		entry, entryErr := parseEntryLine(lines[i], filePath, i+1)
		if entryErr != nil {
			return nil, entryErr
		}
		entries = append(entries, entry)
		lid := leafIDAfterEntryFromJSON(&entry)
		if lid != nil {
			leafID = lid
		}
	}

	return newJsonlSessionStorage(fs, filePath, header, entries, leafID), nil
}

// CreateJsonlSession creates a new JSONL session file.
func CreateJsonlSession(ctx context.Context, fs JsonlFileSystem, filePath string, cwd, sessionID string, parentSessionPath *string) (*JsonlSessionStorage, error) {
	header := jsonlSessionHeader{
		Type:          "session",
		Version:       3,
		ID:            sessionID,
		Timestamp:     harness.NowISO(),
		Cwd:           cwd,
		ParentSession: parentSessionPath,
	}

	data, _ := json.Marshal(header)
	result := fs.WriteFile(ctx, filePath, append(data, '\n'))
	if !result.OK {
		return nil, harness.NewSessionError(harness.SessionErrorStorage,
			fmt.Sprintf("Failed to create session %s: %s", filePath, result.Err.Error()), result.Err)
	}

	return newJsonlSessionStorage(fs, filePath, header, nil, nil), nil
}

// LoadJsonlSessionMetadata reads only the header line from a JSONL session file.
func LoadJsonlSessionMetadata(ctx context.Context, fs JsonlFileSystem, filePath string) (harness.JsonlSessionMetadata, error) {
	result := fs.ReadTextLines(ctx, filePath, 1)
	if !result.OK {
		return harness.JsonlSessionMetadata{}, harness.NewSessionError(harness.SessionErrorStorage,
			fmt.Sprintf("Failed to read session header %s: %s", filePath, result.Err.Error()), result.Err)
	}
	if len(result.Value) == 0 || strings.TrimSpace(result.Value[0]) == "" {
		return harness.JsonlSessionMetadata{}, harness.NewSessionError(harness.SessionErrorInvalidSession,
			fmt.Sprintf("Invalid JSONL session file %s: missing session header", filePath), nil)
	}

	header, err := parseHeaderLine(result.Value[0], filePath)
	if err != nil {
		return harness.JsonlSessionMetadata{}, err
	}

	return harness.JsonlSessionMetadata{
		SessionMetadata: harness.SessionMetadata{
			ID:        header.ID,
			CreatedAt: header.Timestamp,
		},
		Cwd:               header.Cwd,
		Path:              filePath,
		ParentSessionPath: header.ParentSession,
	}, nil
}

// ============================================================================
// SessionStorage interface
// ============================================================================

// GetMetadata returns the JSONL session metadata.
func (s *JsonlSessionStorage) GetMetadata(_ context.Context) (harness.SessionMetadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.metadata.SessionMetadata, nil
}

// GetJsonlMetadata returns the full JSONL-specific metadata.
func (s *JsonlSessionStorage) GetJsonlMetadata(_ context.Context) (harness.JsonlSessionMetadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.metadata, nil
}

// GetLeafID returns the current leaf entry ID.
func (s *JsonlSessionStorage) GetLeafID(_ context.Context) (*string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.leafID != nil {
		if _, ok := s.byID[*s.leafID]; !ok {
			return nil, harness.NewSessionError(harness.SessionErrorInvalidSession,
				"Entry "+*s.leafID+" not found", nil)
		}
	}
	return s.leafID, nil
}

// SetLeafID persists a leaf entry to the JSONL file.
func (s *JsonlSessionStorage) SetLeafID(ctx context.Context, leafID *string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if leafID != nil {
		if _, ok := s.byID[*leafID]; !ok {
			return harness.NewSessionError(harness.SessionErrorNotFound,
				"Entry "+*leafID+" not found", nil)
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

	if err := s.appendToFile(ctx, &entry); err != nil {
		return err
	}

	s.entries = append(s.entries, entry)
	s.byID[entry.ID] = &s.entries[len(s.entries)-1]
	s.leafID = leafID
	return nil
}

// CreateEntryID generates a new unique entry ID.
func (s *JsonlSessionStorage) CreateEntryID(_ context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return generateEntryID(s.byID), nil
}

// AppendEntry appends an entry to the JSONL file.
func (s *JsonlSessionStorage) AppendEntry(ctx context.Context, entry harness.SessionTreeEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.appendToFile(ctx, &entry); err != nil {
		return err
	}

	s.entries = append(s.entries, entry)
	s.byID[entry.ID] = &s.entries[len(s.entries)-1]
	updateLabelCache(s.labelsByID, &entry)
	lid := leafIDAfterEntryFromJSON(&entry)
	if lid != nil {
		s.leafID = lid
	}
	return nil
}

// GetEntry returns an entry by ID.
func (s *JsonlSessionStorage) GetEntry(_ context.Context, id string) (*harness.SessionTreeEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.byID[id], nil
}

// FindEntries returns all entries of a given type.
func (s *JsonlSessionStorage) FindEntries(_ context.Context, entryType string) ([]harness.SessionTreeEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []harness.SessionTreeEntry
	for _, e := range s.entries {
		if e.Type == entryType {
			result = append(result, e)
		}
	}
	return result, nil
}

// GetLabel returns the label for an entry.
func (s *JsonlSessionStorage) GetLabel(_ context.Context, id string) (*string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if label, ok := s.labelsByID[id]; ok {
		return &label, nil
	}
	return nil, nil
}

// GetPathToRoot returns entries from the given leaf to the root.
func (s *JsonlSessionStorage) GetPathToRoot(_ context.Context, leafID *string) ([]harness.SessionTreeEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if leafID == nil {
		return nil, nil
	}

	var path []harness.SessionTreeEntry
	current := s.byID[*leafID]
	if current == nil {
		return nil, harness.NewSessionError(harness.SessionErrorNotFound,
			"Entry "+*leafID+" not found", nil)
	}

	for current != nil {
		path = append([]harness.SessionTreeEntry{*current}, path...)
		if current.ParentID == nil {
			break
		}
		parent := s.byID[*current.ParentID]
		if parent == nil {
			return nil, harness.NewSessionError(harness.SessionErrorInvalidSession,
				"Entry "+*current.ParentID+" not found", nil)
		}
		current = parent
	}
	return path, nil
}

// GetEntries returns all entries in chronological order.
func (s *JsonlSessionStorage) GetEntries(_ context.Context) ([]harness.SessionTreeEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]harness.SessionTreeEntry, len(s.entries))
	copy(result, s.entries)
	return result, nil
}

// ============================================================================
// Internal helpers
// ============================================================================

func (s *JsonlSessionStorage) appendToFile(_ context.Context, entry *harness.SessionTreeEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return harness.NewSessionError(harness.SessionErrorStorage,
			fmt.Sprintf("Failed to serialize entry %s: %s", entry.ID, err.Error()), err)
	}

	result := s.fs.AppendFile(context.Background(), s.filePath, append(data, '\n'))
	if !result.OK {
		return harness.NewSessionError(harness.SessionErrorStorage,
			fmt.Sprintf("Failed to append session entry %s: %s", entry.ID, result.Err.Error()), result.Err)
	}
	return nil
}

// leafIDAfterEntryFromJSON returns the leaf ID after an entry.
// For leaf entries, returns TargetID; for other entries, returns the entry's own ID.
func leafIDAfterEntryFromJSON(entry *harness.SessionTreeEntry) *string {
	if entry.Type == "leaf" {
		return entry.TargetID
	}
	id := entry.ID
	return &id
}

// ============================================================================
// Parsing helpers
// ============================================================================

func parseHeaderLine(line, filePath string) (jsonlSessionHeader, error) {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(line), &parsed); err != nil {
		return jsonlSessionHeader{}, harness.NewSessionError(harness.SessionErrorInvalidSession,
			fmt.Sprintf("Invalid JSONL session file %s: first line is not a valid session header", filePath), err)
	}

	if getString(parsed, "type") != "session" {
		return jsonlSessionHeader{}, harness.NewSessionError(harness.SessionErrorInvalidSession,
			fmt.Sprintf("Invalid JSONL session file %s: first line is not a valid session header", filePath), nil)
	}

	version, _ := parsed["version"].(float64)
	if int(version) != 3 {
		return jsonlSessionHeader{}, harness.NewSessionError(harness.SessionErrorInvalidSession,
			fmt.Sprintf("Invalid JSONL session file %s: unsupported session version", filePath), nil)
	}

	id := getString(parsed, "id")
	if id == "" {
		return jsonlSessionHeader{}, harness.NewSessionError(harness.SessionErrorInvalidSession,
			fmt.Sprintf("Invalid JSONL session file %s: session header is missing id", filePath), nil)
	}

	timestamp := getString(parsed, "timestamp")
	if timestamp == "" {
		return jsonlSessionHeader{}, harness.NewSessionError(harness.SessionErrorInvalidSession,
			fmt.Sprintf("Invalid JSONL session file %s: session header is missing timestamp", filePath), nil)
	}

	cwd := getString(parsed, "cwd")
	if cwd == "" {
		return jsonlSessionHeader{}, harness.NewSessionError(harness.SessionErrorInvalidSession,
			fmt.Sprintf("Invalid JSONL session file %s: session header is missing cwd", filePath), nil)
	}

	var parentSession *string
	if ps, ok := parsed["parentSession"]; ok {
		if ps != nil {
			if psStr, ok := ps.(string); ok {
				parentSession = &psStr
			} else {
				return jsonlSessionHeader{}, harness.NewSessionError(harness.SessionErrorInvalidSession,
					fmt.Sprintf("Invalid JSONL session file %s: session header parentSession must be a string", filePath), nil)
			}
		}
	}

	return jsonlSessionHeader{
		Type:          "session",
		Version:       3,
		ID:            id,
		Timestamp:     timestamp,
		Cwd:           cwd,
		ParentSession: parentSession,
	}, nil
}

func parseEntryLine(line, filePath string, lineNumber int) (harness.SessionTreeEntry, error) {
	var entry harness.SessionTreeEntry
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return harness.SessionTreeEntry{}, harness.NewSessionError(harness.SessionErrorInvalidEntry,
			fmt.Sprintf("Invalid JSONL session file %s: line %d is not valid JSON", filePath, lineNumber), err)
	}
	if entry.Type == "" {
		return harness.SessionTreeEntry{}, harness.NewSessionError(harness.SessionErrorInvalidEntry,
			fmt.Sprintf("Invalid JSONL session file %s: line %d is missing entry type", filePath, lineNumber), nil)
	}
	if entry.ID == "" {
		return harness.SessionTreeEntry{}, harness.NewSessionError(harness.SessionErrorInvalidEntry,
			fmt.Sprintf("Invalid JSONL session file %s: line %d is missing entry id", filePath, lineNumber), nil)
	}
	if entry.Timestamp == "" {
		return harness.SessionTreeEntry{}, harness.NewSessionError(harness.SessionErrorInvalidEntry,
			fmt.Sprintf("Invalid JSONL session file %s: line %d is missing timestamp", filePath, lineNumber), nil)
	}
	return entry, nil
}

func nonEmptyLines(content string) []string {
	var result []string
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) != "" {
			result = append(result, line)
		}
	}
	return result
}

func getString(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}
