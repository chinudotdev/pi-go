package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chinudotdev/pi-go/agent/harness"
	"github.com/chinudotdev/pi-go/ai"
)

// ============================================================================
// Mock filesystem for JSONL tests
// ============================================================================

type mockFS struct {
	files map[string]string // path -> content
	dirs  map[string]bool
	cwd   string
}

func newMockFS() *mockFS {
	return &mockFS{
		files: make(map[string]string),
		dirs:  make(map[string]bool),
	}
}

func (m *mockFS) Cwd() string { return m.cwd }

func (m *mockFS) AbsolutePath(_ context.Context, path string) harness.Result[string] {
	if filepath.IsAbs(path) {
		return harness.OkResult(path)
	}
	return harness.OkResult(filepath.Join(m.cwd, path))
}

func (m *mockFS) JoinPath(_ context.Context, parts ...string) harness.Result[string] {
	return harness.OkResult(filepath.Join(parts...))
}

func (m *mockFS) ReadTextFile(_ context.Context, path string) harness.Result[string] {
	if content, ok := m.files[path]; ok {
		return harness.OkResult(content)
	}
	return harness.ErrResult[string](harness.NewFileError("not_found", "file not found", path, nil))
}

func (m *mockFS) ReadTextLines(_ context.Context, path string, maxLines int) harness.Result[[]string] {
	content, ok := m.files[path]
	if !ok {
		return harness.ErrResult[[]string](harness.NewFileError("not_found", "file not found", path, nil))
	}
	lines := strings.Split(content, "\n")
	// Remove trailing empty from final newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return harness.OkResult(lines)
}

func (m *mockFS) WriteFile(_ context.Context, path string, content []byte) harness.Result[struct{}] {
	m.ensureDir(filepath.Dir(path))
	m.files[path] = string(content)
	m.dirs[filepath.Dir(path)] = true
	return harness.OkResult(struct{}{})
}

func (m *mockFS) AppendFile(_ context.Context, path string, content []byte) harness.Result[struct{}] {
	m.files[path] += string(content)
	return harness.OkResult(struct{}{})
}

func (m *mockFS) ListDir(_ context.Context, path string) harness.Result[[]harness.FileInfo] {
	var infos []harness.FileInfo
	for p, content := range m.files {
		dir := filepath.Dir(p)
		if dir == path {
			infos = append(infos, harness.FileInfo{
				Name: filepath.Base(p),
				Path: p,
				Kind: harness.FileKindFile,
				Size: int64(len(content)),
			})
		}
	}
	for d := range m.dirs {
		if filepath.Dir(d) == path {
			infos = append(infos, harness.FileInfo{
				Name: filepath.Base(d),
				Path: d,
				Kind: harness.FileKindDirectory,
			})
		}
	}
	return harness.OkResult(infos)
}

func (m *mockFS) Exists(_ context.Context, path string) harness.Result[bool] {
	if _, ok := m.files[path]; ok {
		return harness.OkResult(true)
	}
	if m.dirs[path] {
		return harness.OkResult(true)
	}
	return harness.OkResult(false)
}

func (m *mockFS) CreateDir(_ context.Context, path string, recursive bool) harness.Result[struct{}] {
	m.dirs[path] = true
	return harness.OkResult(struct{}{})
}

func (m *mockFS) Remove(_ context.Context, path string, _ bool, _ bool) harness.Result[struct{}] {
	delete(m.files, path)
	return harness.OkResult(struct{}{})
}

func (m *mockFS) ReadBinaryFile(_ context.Context, path string) harness.Result[[]byte] {
	if content, ok := m.files[path]; ok {
		return harness.OkResult([]byte(content))
	}
	return harness.ErrResult[[]byte](harness.NewFileError("not_found", "file not found", path, nil))
}

func (m *mockFS) FileInfo(_ context.Context, path string) harness.Result[harness.FileInfo] {
	if content, ok := m.files[path]; ok {
		return harness.OkResult(harness.FileInfo{
			Name: filepath.Base(path),
			Path: path,
			Kind: harness.FileKindFile,
			Size: int64(len(content)),
		})
	}
	return harness.ErrResult[harness.FileInfo](harness.NewFileError("not_found", "not found", path, nil))
}

func (m *mockFS) CanonicalPath(_ context.Context, path string) harness.Result[string] {
	return harness.OkResult(path)
}

func (m *mockFS) CreateTempDir(_ context.Context, prefix string) harness.Result[string] {
	return harness.OkResult("/tmp/" + prefix + "123")
}

func (m *mockFS) CreateTempFile(_ context.Context, prefix, suffix string) harness.Result[string] {
	return harness.OkResult("/tmp/" + prefix + "123" + suffix)
}

func (m *mockFS) Cleanup(_ context.Context) {}

func (m *mockFS) ensureDir(dir string) {
	m.dirs[dir] = true
}

// ============================================================================
// JSONL Storage Tests
// ============================================================================

func TestJsonlStorage_CreateAndOpen(t *testing.T) {
	fs := newMockFS()
	ctx := context.Background()

	storage, err := CreateJsonlSession(ctx, fs, "/sessions/test.jsonl", "/home/user", "session-1", nil)
	if err != nil {
		t.Fatal(err)
	}

	meta, err := storage.GetMetadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ID != "session-1" {
		t.Errorf("expected session-1, got %s", meta.ID)
	}

	// Re-open from file
	storage2, err := OpenJsonlSession(ctx, fs, "/sessions/test.jsonl")
	if err != nil {
		t.Fatal(err)
	}

	meta2, err := storage2.GetMetadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if meta2.ID != "session-1" {
		t.Errorf("expected session-1, got %s", meta2.ID)
	}
}

func TestJsonlStorage_AppendAndGetEntry(t *testing.T) {
	fs := newMockFS()
	ctx := context.Background()

	storage, err := CreateJsonlSession(ctx, fs, "/sessions/test.jsonl", "/home/user", "session-1", nil)
	if err != nil {
		t.Fatal(err)
	}

	entryID, err := storage.CreateEntryID(ctx)
	if err != nil {
		t.Fatal(err)
	}

	entry := harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{
			Type:      "message",
			ID:        entryID,
			ParentID:  nil,
			Timestamp: "2025-01-01T00:00:00Z",
		},
	}

	if err := storage.AppendEntry(ctx, entry); err != nil {
		t.Fatal(err)
	}

	got, err := storage.GetEntry(ctx, entryID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected entry, got nil")
	}
	if got.ID != entryID {
		t.Errorf("expected %s, got %s", entryID, got.ID)
	}

	// Re-open and verify
	storage2, err := OpenJsonlSession(ctx, fs, "/sessions/test.jsonl")
	if err != nil {
		t.Fatal(err)
	}

	got2, err := storage2.GetEntry(ctx, entryID)
	if err != nil {
		t.Fatal(err)
	}
	if got2 == nil {
		t.Fatal("expected entry after reopen, got nil")
	}
	if got2.ID != entryID {
		t.Errorf("expected %s after reopen, got %s", entryID, got2.ID)
	}
}

func TestJsonlStorage_LeafTracking(t *testing.T) {
	fs := newMockFS()
	ctx := context.Background()

	storage, err := CreateJsonlSession(ctx, fs, "/sessions/test.jsonl", "/home/user", "session-1", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Initially no leaf
	leaf, err := storage.GetLeafID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if leaf != nil {
		t.Errorf("expected nil leaf, got %s", *leaf)
	}

	// Append an entry — leaf becomes that entry's ID
	entryID, _ := storage.CreateEntryID(ctx)
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{
			Type:      "message",
			ID:        entryID,
			Timestamp: "2025-01-01T00:00:00Z",
		},
	})

	leaf, err = storage.GetLeafID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if leaf == nil || *leaf != entryID {
		t.Errorf("expected leaf %s, got %v", entryID, leaf)
	}

	// SetLeafID to a specific target
	newLeafID, _ := storage.CreateEntryID(ctx)
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{
			Type:      "message",
			ID:        newLeafID,
			ParentID:  &entryID,
			Timestamp: "2025-01-01T00:01:00Z",
		},
	})

	err = storage.SetLeafID(ctx, &entryID)
	if err != nil {
		t.Fatal(err)
	}

	leaf, err = storage.GetLeafID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if leaf == nil || *leaf != entryID {
		t.Errorf("expected leaf %s after SetLeafID, got %v", entryID, leaf)
	}
}

func TestJsonlStorage_GetEntries(t *testing.T) {
	fs := newMockFS()
	ctx := context.Background()

	storage, _ := CreateJsonlSession(ctx, fs, "/sessions/test.jsonl", "/home/user", "session-1", nil)

	ids := make([]string, 3)
	for i := 0; i < 3; i++ {
		id, _ := storage.CreateEntryID(ctx)
		ids[i] = id
		storage.AppendEntry(ctx, harness.SessionTreeEntry{
			SessionTreeEntryBase: harness.SessionTreeEntryBase{
				Type:      "message",
				ID:        id,
				Timestamp: "2025-01-01T00:00:00Z",
			},
		})
	}

	entries, err := storage.GetEntries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// Verify order
	for i, entry := range entries {
		if entry.ID != ids[i] {
			t.Errorf("entry %d: expected %s, got %s", i, ids[i], entry.ID)
		}
	}
}

func TestJsonlStorage_FindEntries(t *testing.T) {
	fs := newMockFS()
	ctx := context.Background()

	storage, _ := CreateJsonlSession(ctx, fs, "/sessions/test.jsonl", "/home/user", "session-1", nil)

	id1, _ := storage.CreateEntryID(ctx)
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{
			Type:      "message",
			ID:        id1,
			Timestamp: "2025-01-01T00:00:00Z",
		},
	})

	id2, _ := storage.CreateEntryID(ctx)
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{
			Type:      "model_change",
			ID:        id2,
			Timestamp: "2025-01-01T00:01:00Z",
		},
	})

	messages, err := storage.FindEntries(ctx, "message")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(messages))
	}

	modelChanges, err := storage.FindEntries(ctx, "model_change")
	if err != nil {
		t.Fatal(err)
	}
	if len(modelChanges) != 1 {
		t.Errorf("expected 1 model_change, got %d", len(modelChanges))
	}
}

func TestJsonlStorage_Labels(t *testing.T) {
	fs := newMockFS()
	ctx := context.Background()

	storage, _ := CreateJsonlSession(ctx, fs, "/sessions/test.jsonl", "/home/user", "session-1", nil)

	entryID, _ := storage.CreateEntryID(ctx)
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{
			Type:      "message",
			ID:        entryID,
			Timestamp: "2025-01-01T00:00:00Z",
		},
	})

	labelText := "my-label"
	labelID, _ := storage.CreateEntryID(ctx)
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{
			Type:      "label",
			ID:        labelID,
			Timestamp: "2025-01-01T00:01:00Z",
		},
		TargetID: &entryID,
		Label:    &labelText,
	})

	label, err := storage.GetLabel(ctx, entryID)
	if err != nil {
		t.Fatal(err)
	}
	if label == nil || *label != "my-label" {
		t.Errorf("expected 'my-label', got %v", label)
	}
}

func TestJsonlStorage_GetPathToRoot(t *testing.T) {
	fs := newMockFS()
	ctx := context.Background()

	storage, _ := CreateJsonlSession(ctx, fs, "/sessions/test.jsonl", "/home/user", "session-1", nil)

	// Build a chain: e1 -> e2 -> e3
	id1, _ := storage.CreateEntryID(ctx)
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{
			Type:      "message",
			ID:        id1,
			Timestamp: "2025-01-01T00:00:00Z",
		},
	})

	id2, _ := storage.CreateEntryID(ctx)
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{
			Type:      "message",
			ID:        id2,
			ParentID:  &id1,
			Timestamp: "2025-01-01T00:01:00Z",
		},
	})

	id3, _ := storage.CreateEntryID(ctx)
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{
			Type:      "message",
			ID:        id3,
			ParentID:  &id2,
			Timestamp: "2025-01-01T00:02:00Z",
		},
	})

	path, err := storage.GetPathToRoot(ctx, &id3)
	if err != nil {
		t.Fatal(err)
	}
	if len(path) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(path))
	}
	if path[0].ID != id1 || path[1].ID != id2 || path[2].ID != id3 {
		t.Errorf("path order wrong: %v", pathIDs(path))
	}
}

func TestJsonlStorage_FileContent(t *testing.T) {
	// Verify that the JSONL file actually contains the expected content
	fs := newMockFS()
	ctx := context.Background()

	storage, _ := CreateJsonlSession(ctx, fs, "/sessions/test.jsonl", "/home/user", "session-1", nil)

	entryID, _ := storage.CreateEntryID(ctx)
	storage.AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{
			Type:      "message",
			ID:        entryID,
			Timestamp: "2025-01-01T00:00:00Z",
		},
	})

	content := fs.files["/sessions/test.jsonl"]
	lines := nonEmptyLines(content)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (header + entry), got %d", len(lines))
	}

	// Verify header
	var header map[string]any
	json.Unmarshal([]byte(lines[0]), &header)
	if header["type"] != "session" {
		t.Errorf("expected session header, got %v", header["type"])
	}
	if header["id"] != "session-1" {
		t.Errorf("expected session-1, got %v", header["id"])
	}

	// Verify entry
	var entry map[string]any
	json.Unmarshal([]byte(lines[1]), &entry)
	if entry["type"] != "message" {
		t.Errorf("expected message entry, got %v", entry["type"])
	}
	if entry["id"] != entryID {
		t.Errorf("expected %s, got %v", entryID, entry["id"])
	}
}

func TestJsonlStorage_ParentSession(t *testing.T) {
	fs := newMockFS()
	ctx := context.Background()

	parentPath := "/sessions/parent.jsonl"
	storage, err := CreateJsonlSession(ctx, fs, "/sessions/child.jsonl", "/home/user", "child-1", &parentPath)
	if err != nil {
		t.Fatal(err)
	}

	meta, err := storage.GetJsonlMetadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ParentSessionPath == nil || *meta.ParentSessionPath != parentPath {
		t.Errorf("expected parent path %s, got %v", parentPath, meta.ParentSessionPath)
	}
}

func TestJsonlStorage_InvalidFile(t *testing.T) {
	fs := newMockFS()
	ctx := context.Background()

	// Empty file
	fs.files["/sessions/empty.jsonl"] = ""
	_, err := OpenJsonlSession(ctx, fs, "/sessions/empty.jsonl")
	if err == nil {
		t.Fatal("expected error for empty file")
	}

	// Invalid JSON
	fs.files["/sessions/bad.jsonl"] = "not json\n"
	_, err = OpenJsonlSession(ctx, fs, "/sessions/bad.jsonl")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}

	// Missing version
	fs.files["/sessions/noversion.jsonl"] = `{"type":"session","id":"test","timestamp":"2025-01-01","cwd":"/home"}`
	_, err = OpenJsonlSession(ctx, fs, "/sessions/noversion.jsonl")
	if err == nil {
		t.Fatal("expected error for missing version")
	}
}

// ============================================================================
// JSONL Repo Tests
// ============================================================================

func TestJsonlRepo_CreateAndOpen(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "jsonl-repo-test-")
	defer os.RemoveAll(tmpDir)

	// Use real filesystem via env package would be ideal, but let's use mock
	fs := newMockFS()
	fs.cwd = tmpDir
	fs.dirs[tmpDir] = true

	repo := NewJsonlSessionRepo(fs, tmpDir)
	ctx := context.Background()

	sess, err := repo.Create(ctx, &JsonlCreateOptions{
		Cwd: "/home/user/project",
		ID:  "test-session",
	})
	if err != nil {
		t.Fatal(err)
	}

	meta, err := sess.GetMetadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ID != "test-session" {
		t.Errorf("expected test-session, got %s", meta.ID)
	}

	// List should find it
	sessions, err := repo.List(ctx, &ListOptions{Cwd: "/home/user/project"})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].ID != "test-session" {
		t.Errorf("expected test-session, got %s", sessions[0].ID)
	}
}

func TestJsonlRepo_Delete(t *testing.T) {
	fs := newMockFS()
	fs.cwd = "/root"
	fs.dirs["/root"] = true

	repo := NewJsonlSessionRepo(fs, "/root")
	ctx := context.Background()

	repo.Create(ctx, &JsonlCreateOptions{Cwd: "/home/user", ID: "to-delete"})

	// Get full metadata via List
	sessions, err := repo.List(ctx, &ListOptions{Cwd: "/home/user"})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatal("expected 1 session")
	}
	if err := repo.Delete(ctx, sessions[0]); err != nil {
		t.Fatal(err)
	}

	sessions2, _ := repo.List(ctx, &ListOptions{Cwd: "/home/user"})
	if len(sessions2) != 0 {
		t.Errorf("expected 0 sessions after delete, got %d", len(sessions2))
	}
}

func TestJsonlRepo_Fork(t *testing.T) {
	fs := newMockFS()
	fs.cwd = "/root"
	fs.dirs["/root"] = true

	repo := NewJsonlSessionRepo(fs, "/root")
	ctx := context.Background()

	// Create source session with entries
	source, _ := repo.Create(ctx, &JsonlCreateOptions{Cwd: "/home/user", ID: "source"})

	entryID, _ := source.GetStorage().CreateEntryID(ctx)
	source.GetStorage().AppendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: harness.SessionTreeEntryBase{
			Type:      "message",
			ID:        entryID,
			Timestamp: "2025-01-01T00:00:00Z",
		},
	})

	// Get full metadata via List for the file path
	sources, _ := repo.List(ctx, &ListOptions{Cwd: "/home/user"})
	if len(sources) != 1 {
		t.Fatal("expected 1 session")
	}
	sourceJsonlMeta := sources[0]

	// Fork
	forked, err := repo.Fork(ctx, sourceJsonlMeta, &JsonlForkOptions{
		Cwd: "/home/user",
		ID:  "forked",
	})
	if err != nil {
		t.Fatal(err)
	}

	forkedMeta, _ := forked.GetMetadata(ctx)
	if forkedMeta.ID != "forked" {
		t.Errorf("expected forked, got %s", forkedMeta.ID)
	}

	// Should have the same entry
	entries, _ := forked.GetEntries(ctx)
	if len(entries) != 1 {
		t.Errorf("expected 1 entry in forked session, got %d", len(entries))
	}
}

func TestJsonlRepo_OpenNotFound(t *testing.T) {
	fs := newMockFS()
	fs.cwd = "/root"

	repo := NewJsonlSessionRepo(fs, "/root")
	ctx := context.Background()

	_, err := repo.Open(ctx, harness.JsonlSessionMetadata{
		SessionMetadata: harness.SessionMetadata{ID: "missing"},
		Path:            "/nonexistent.jsonl",
	})
	if err == nil {
		t.Fatal("expected error for missing session")
	}
}

func TestJsonlRepo_ListEmpty(t *testing.T) {
	fs := newMockFS()
	fs.cwd = "/root"

	repo := NewJsonlSessionRepo(fs, "/root")
	ctx := context.Background()

	sessions, err := repo.List(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

// ============================================================================
// Helpers
// ============================================================================

func pathIDs(entries []harness.SessionTreeEntry) []string {
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	return ids
}

func TestEncodeCwd(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/home/user/project", "--home-user-project--"},
		{"/", "----"},
		{"C:\\Users\\test", "--C--Users-test--"},
	}
	for _, tc := range tests {
		got := encodeCwd(tc.input)
		if got != tc.expected {
			t.Errorf("encodeCwd(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestJsonlStorage_SessionIntegration(t *testing.T) {
	// Integration test: use Session wrapper on top of JsonlSessionStorage
	fs := newMockFS()
	ctx := context.Background()

	storage, err := CreateJsonlSession(ctx, fs, "/sessions/test.jsonl", "/home/user", "integ-session", nil)
	if err != nil {
		t.Fatal(err)
	}

	sess := NewSession(storage)

	// Append a message via session
	_, err = sess.AppendMessage(ctx, ai.Message{
		Role:      "user",
		Content:   "hello",
		Timestamp: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}

	entries, err := sess.GetEntries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestLoadJsonlSessionMetadata(t *testing.T) {
	fs := newMockFS()
	ctx := context.Background()

	// Create a session file with known content
	header := map[string]any{
		"type":      "session",
		"version":   3,
		"id":        "meta-test",
		"timestamp": "2025-01-15T10:30:00Z",
		"cwd":       "/home/user",
	}
	data, _ := json.Marshal(header)
	fs.files["/sessions/meta.jsonl"] = string(data) + "\n"

	meta, err := LoadJsonlSessionMetadata(ctx, fs, "/sessions/meta.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if meta.ID != "meta-test" {
		t.Errorf("expected meta-test, got %s", meta.ID)
	}
	if meta.Cwd != "/home/user" {
		t.Errorf("expected /home/user, got %s", meta.Cwd)
	}
}

func TestJsonlStorage_ReopenWithMultipleEntries(t *testing.T) {
	fs := newMockFS()
	ctx := context.Background()

	storage, _ := CreateJsonlSession(ctx, fs, "/sessions/multi.jsonl", "/home", "multi-1", nil)

	// Add 5 entries
	ids := make([]string, 5)
	for i := 0; i < 5; i++ {
		id, _ := storage.CreateEntryID(ctx)
		ids[i] = id
		var parentID *string
		if i > 0 {
			parentID = &ids[i-1]
		}
		storage.AppendEntry(ctx, harness.SessionTreeEntry{
			SessionTreeEntryBase: harness.SessionTreeEntryBase{
				Type:      "message",
				ID:        id,
				ParentID:  parentID,
				Timestamp: fmt.Sprintf("2025-01-01T00:0%d:00Z", i),
			},
		})
	}

	// Reopen
	storage2, err := OpenJsonlSession(ctx, fs, "/sessions/multi.jsonl")
	if err != nil {
		t.Fatal(err)
	}

	entries, _ := storage2.GetEntries(ctx)
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}

	// Verify path to root
	path, err := storage2.GetPathToRoot(ctx, &ids[4])
	if err != nil {
		t.Fatal(err)
	}
	if len(path) != 5 {
		t.Errorf("expected 5 entries in path, got %d", len(path))
	}
}
