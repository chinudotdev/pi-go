package harness

import "context"

// FileInfo holds metadata for one filesystem object.
type FileInfo struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	Kind    FileKind `json:"kind"`
	Size    int64    `json:"size"`
	MtimeMs int64    `json:"mtimeMs"`
}

// ExecOptions controls shell command execution.
type ExecOptions struct {
	Cwd      string
	Env      map[string]string
	Timeout  int // seconds
	OnStdout func(chunk string)
	OnStderr func(chunk string)
}

// ExecResult holds the result of a shell command.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// FileSystem is the filesystem capability used by the harness.
// All methods receive context.Context for cancellation.
// All methods return Result[T, *FileError] instead of panicking.
type FileSystem interface {
	// Cwd returns the current working directory.
	Cwd() string

	// AbsolutePath returns an absolute path without requiring it to exist.
	AbsolutePath(ctx context.Context, path string) Result[string]
	// JoinPath joins path segments without requiring the result to exist.
	JoinPath(ctx context.Context, parts ...string) Result[string]
	// ReadTextFile reads a UTF-8 text file.
	ReadTextFile(ctx context.Context, path string) Result[string]
	// ReadTextLines reads UTF-8 text lines, stopping after maxLines if > 0.
	ReadTextLines(ctx context.Context, path string, maxLines int) Result[[]string]
	// ReadBinaryFile reads a binary file.
	ReadBinaryFile(ctx context.Context, path string) Result[[]byte]
	// WriteFile creates or overwrites a file, creating parent directories.
	WriteFile(ctx context.Context, path string, content []byte) Result[struct{}]
	// AppendFile creates or appends to a file, creating parent directories.
	AppendFile(ctx context.Context, path string, content []byte) Result[struct{}]
	// FileInfo returns metadata for the addressed path.
	FileInfo(ctx context.Context, path string) Result[FileInfo]
	// ListDir lists direct children of a directory.
	ListDir(ctx context.Context, path string) Result[[]FileInfo]
	// CanonicalPath returns the canonical path, resolving symlinks.
	CanonicalPath(ctx context.Context, path string) Result[string]
	// Exists returns false for missing paths. Other errors return FileError.
	Exists(ctx context.Context, path string) Result[bool]
	// CreateDir creates a directory. Use recursive=true for mkdir -p.
	CreateDir(ctx context.Context, path string, recursive bool) Result[struct{}]
	// Remove removes a file or directory.
	Remove(ctx context.Context, path string, recursive bool, force bool) Result[struct{}]
	// CreateTempDir creates a temporary directory and returns its absolute path.
	CreateTempDir(ctx context.Context, prefix string) Result[string]
	// CreateTempFile creates a temporary file and returns its absolute path.
	CreateTempFile(ctx context.Context, prefix, suffix string) Result[string]
	// Cleanup releases filesystem resources.
	Cleanup(ctx context.Context)
}

// Shell is the shell execution capability used by the harness.
type Shell interface {
	// Exec executes a shell command.
	Exec(ctx context.Context, command string, opts *ExecOptions) Result[ExecResult]
	// Cleanup releases shell resources.
	Cleanup(ctx context.Context)
}

// ExecutionEnv combines FileSystem and Shell.
type ExecutionEnv interface {
	FileSystem
	Shell
}
