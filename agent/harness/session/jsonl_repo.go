package session

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chinudotdev/pi-go/agent/harness"
)

// JsonlSessionRepo manages JSONL-backed sessions on the filesystem.
type JsonlSessionRepo struct {
	fs             JsonlRepoFileSystem
	sessionsRootIn string
	sessionsRoot   *string
}

// NewJsonlSessionRepo creates a new JSONL session repository.
func NewJsonlSessionRepo(fs JsonlRepoFileSystem, sessionsRoot string) *JsonlSessionRepo {
	return &JsonlSessionRepo{
		fs:             fs,
		sessionsRootIn: sessionsRoot,
	}
}

// Create creates a new JSONL session file.
func (r *JsonlSessionRepo) Create(ctx context.Context, opts *JsonlCreateOptions) (*Session, error) {
	if opts == nil {
		opts = &JsonlCreateOptions{}
	}

	id := opts.ID
	if id == "" {
		id = UUIDv7()
	}

	createdAt := harness.NowISO()
	sessionDir, err := r.getSessionDir(ctx, opts.Cwd)
	if err != nil {
		return nil, err
	}

	dirResult := r.fs.CreateDir(ctx, sessionDir, true)
	if !dirResult.OK {
		return nil, harness.NewSessionError(harness.SessionErrorStorage,
			fmt.Sprintf("Failed to create session directory %s: %s", sessionDir, dirResult.Err.Error()), dirResult.Err)
	}

	filePath := r.sessionFilePath(sessionDir, id, createdAt)
	storage, err := CreateJsonlSession(ctx, r.fs, filePath, opts.Cwd, id, opts.ParentSessionPath)
	if err != nil {
		return nil, err
	}

	return NewSession(storage), nil
}

// Open opens an existing JSONL session by metadata.
func (r *JsonlSessionRepo) Open(ctx context.Context, metadata harness.JsonlSessionMetadata) (*Session, error) {
	existsResult := r.fs.Exists(ctx, metadata.Path)
	if !existsResult.OK {
		return nil, harness.NewSessionError(harness.SessionErrorStorage,
			fmt.Sprintf("Failed to check session %s: %s", metadata.Path, existsResult.Err.Error()), existsResult.Err)
	}
	if !existsResult.Value {
		return nil, harness.NewSessionError(harness.SessionErrorNotFound,
			"Session not found: "+metadata.Path, nil)
	}

	storage, err := OpenJsonlSession(ctx, r.fs, metadata.Path)
	if err != nil {
		return nil, err
	}
	return NewSession(storage), nil
}

// ListOptions controls listing behavior.
type ListOptions struct {
	// Cwd, if set, lists only sessions for this working directory.
	Cwd string
}

// List returns metadata for all sessions, sorted newest first.
func (r *JsonlSessionRepo) List(ctx context.Context, opts *ListOptions) ([]harness.JsonlSessionMetadata, error) {
	var dirs []string
	if opts != nil && opts.Cwd != "" {
		dir, err := r.getSessionDir(ctx, opts.Cwd)
		if err != nil {
			return nil, err
		}
		dirs = []string{dir}
	} else {
		var err error
		dirs, err = r.listSessionDirs(ctx)
		if err != nil {
			return nil, err
		}
	}

	var sessions []harness.JsonlSessionMetadata
	for _, dir := range dirs {
		existsResult := r.fs.Exists(ctx, dir)
		if !existsResult.OK || !existsResult.Value {
			continue
		}

		entriesResult := r.fs.ListDir(ctx, dir)
		if !entriesResult.OK {
			continue
		}

		for _, file := range entriesResult.Value {
			if file.Kind == harness.FileKindDirectory || !strings.HasSuffix(file.Name, ".jsonl") {
				continue
			}
			meta, err := LoadJsonlSessionMetadata(ctx, r.fs, file.Path)
			if err != nil {
				// Skip invalid session files (matching TS behavior)
				if se, ok := err.(*harness.SessionError); ok && se.Code == harness.SessionErrorInvalidSession {
					continue
				}
				return nil, err
			}
			sessions = append(sessions, meta)
		}
	}

	// Sort by creation time, newest first
	sort.Slice(sessions, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339Nano, sessions[i].CreatedAt)
		tj, _ := time.Parse(time.RFC3339Nano, sessions[j].CreatedAt)
		return tj.Before(ti)
	})

	return sessions, nil
}

// Delete removes a JSONL session file.
func (r *JsonlSessionRepo) Delete(ctx context.Context, metadata harness.JsonlSessionMetadata) error {
	result := r.fs.Remove(ctx, metadata.Path, false, true)
	if !result.OK {
		return harness.NewSessionError(harness.SessionErrorStorage,
			fmt.Sprintf("Failed to delete session %s: %s", metadata.Path, result.Err.Error()), result.Err)
	}
	return nil
}

// Fork creates a new session by copying entries from an existing session.
func (r *JsonlSessionRepo) Fork(ctx context.Context, sourceMeta harness.JsonlSessionMetadata, opts *JsonlForkOptions) (*Session, error) {
	if opts == nil {
		opts = &JsonlForkOptions{}
	}

	source, err := r.Open(ctx, sourceMeta)
	if err != nil {
		return nil, err
	}

	forkOpts := SessionForkOptions{
		EntryID:  opts.EntryID,
		Position: opts.Position,
	}
	forkedEntries, err := GetEntriesToFork(ctx, source.GetStorage(), forkOpts)
	if err != nil {
		return nil, err
	}

	id := opts.ID
	if id == "" {
		id = UUIDv7()
	}

	parentPath := opts.ParentSessionPath
	if parentPath == nil {
		parentPath = &sourceMeta.Path
	}

	createOpts := &JsonlCreateOptions{
		Cwd:               opts.Cwd,
		ID:                id,
		ParentSessionPath: parentPath,
	}
	newSession, err := r.Create(ctx, createOpts)
	if err != nil {
		return nil, err
	}

	// Copy entries from source
	for _, entry := range forkedEntries {
		if err := newSession.GetStorage().AppendEntry(ctx, entry); err != nil {
			return nil, err
		}
	}

	return newSession, nil
}

// ============================================================================
// Internal helpers
// ============================================================================

func (r *JsonlSessionRepo) getSessionsRoot(ctx context.Context) (string, error) {
	if r.sessionsRoot != nil {
		return *r.sessionsRoot, nil
	}

	result := r.fs.AbsolutePath(ctx, r.sessionsRootIn)
	if !result.OK {
		return "", harness.NewSessionError(harness.SessionErrorStorage,
			fmt.Sprintf("Failed to resolve sessions root %s: %s", r.sessionsRootIn, result.Err.Error()), result.Err)
	}
	r.sessionsRoot = &result.Value
	return result.Value, nil
}

func (r *JsonlSessionRepo) getSessionDir(ctx context.Context, cwd string) (string, error) {
	root, err := r.getSessionsRoot(ctx)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, encodeCwd(cwd)), nil
}

func (r *JsonlSessionRepo) sessionFilePath(sessionDir, sessionID, timestamp string) string {
	sanitized := strings.NewReplacer(":", "-", ".", "-").Replace(timestamp)
	return filepath.Join(sessionDir, sanitized+"_"+sessionID+".jsonl")
}

func (r *JsonlSessionRepo) listSessionDirs(ctx context.Context) ([]string, error) {
	root, err := r.getSessionsRoot(ctx)
	if err != nil {
		return nil, err
	}

	existsResult := r.fs.Exists(ctx, root)
	if !existsResult.OK || !existsResult.Value {
		return nil, nil
	}

	entriesResult := r.fs.ListDir(ctx, root)
	if !entriesResult.OK {
		return nil, nil
	}

	var dirs []string
	for _, entry := range entriesResult.Value {
		if entry.Kind == harness.FileKindDirectory {
			dirs = append(dirs, entry.Path)
		}
	}
	return dirs, nil
}

// encodeCwd converts a cwd path to a safe directory name.
func encodeCwd(cwd string) string {
	s := strings.TrimLeft(cwd, "/\\")
	s = strings.TrimLeft(s, "\\")
	return "--" + strings.NewReplacer("/", "-", "\\", "-", ":", "-").Replace(s) + "--"
}

// JsonlCreateOptions configures JSONL session creation.
type JsonlCreateOptions struct {
	Cwd               string  `json:"cwd"`
	ID                string  `json:"id,omitempty"`
	ParentSessionPath *string `json:"parentSessionPath,omitempty"`
}

// JsonlForkOptions configures JSONL session forking.
type JsonlForkOptions struct {
	Cwd               string  `json:"cwd"`
	ID                string  `json:"id,omitempty"`
	EntryID           *string `json:"entryId,omitempty"`
	Position          string  `json:"position,omitempty"` // "before" or "at"
	ParentSessionPath *string `json:"parentSessionPath,omitempty"`
}
