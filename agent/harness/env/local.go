package env

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chinudotdev/pi-go/agent/harness"
)

// LocalEnv implements harness.ExecutionEnv using the local filesystem and shell.
type LocalEnv struct {
	cwd       string
	shellPath string
	shellEnv  []string // extra env vars (KEY=VALUE)
}

// NewLocalEnv creates a new LocalEnv for the given working directory.
func NewLocalEnv(cwd string, opts ...LocalEnvOption) *LocalEnv {
	env := &LocalEnv{cwd: cwd}
	for _, opt := range opts {
		opt(env)
	}
	return env
}

// LocalEnvOption configures a LocalEnv.
type LocalEnvOption func(*LocalEnv)

// WithShellPath sets a custom shell binary path.
func WithShellPath(path string) LocalEnvOption {
	return func(e *LocalEnv) { e.shellPath = path }
}

// WithEnvVars sets extra environment variables (KEY=VALUE format).
func WithEnvVars(vars []string) LocalEnvOption {
	return func(e *LocalEnv) { e.shellEnv = vars }
}

// ============================================================================
// FileSystem interface
// ============================================================================

// Cwd returns the current working directory.
func (e *LocalEnv) Cwd() string { return e.cwd }

// AbsolutePath returns an absolute path without requiring it to exist.
func (e *LocalEnv) AbsolutePath(_ context.Context, path string) harness.Result[string] {
	if filepath.IsAbs(path) {
		return harness.OkResult(path)
	}
	return harness.OkResult(filepath.Join(e.cwd, path))
}

// JoinPath joins path segments.
func (e *LocalEnv) JoinPath(_ context.Context, parts ...string) harness.Result[string] {
	return harness.OkResult(filepath.Join(parts...))
}

// ReadTextFile reads a UTF-8 text file.
func (e *LocalEnv) ReadTextFile(_ context.Context, path string) harness.Result[string] {
	resolved := e.resolvePath(path)
	data, err := os.ReadFile(resolved)
	if err != nil {
		return harness.ErrResult[string](toFileError(err, resolved))
	}
	return harness.OkResult(string(data))
}

// ReadTextLines reads UTF-8 text lines, stopping after maxLines if > 0.
func (e *LocalEnv) ReadTextLines(_ context.Context, path string, maxLines int) harness.Result[[]string] {
	resolved := e.resolvePath(path)
	data, err := os.ReadFile(resolved)
	if err != nil {
		return harness.ErrResult[[]string](toFileError(err, resolved))
	}
	allLines := strings.Split(string(data), "\n")
	// Remove trailing empty line from final newline
	if len(allLines) > 0 && allLines[len(allLines)-1] == "" {
		allLines = allLines[:len(allLines)-1]
	}
	if maxLines > 0 && len(allLines) > maxLines {
		allLines = allLines[:maxLines]
	}
	return harness.OkResult(allLines)
}

// ReadBinaryFile reads a binary file.
func (e *LocalEnv) ReadBinaryFile(_ context.Context, path string) harness.Result[[]byte] {
	resolved := e.resolvePath(path)
	data, err := os.ReadFile(resolved)
	if err != nil {
		return harness.ErrResult[[]byte](toFileError(err, resolved))
	}
	return harness.OkResult(data)
}

// WriteFile creates or overwrites a file, creating parent directories.
func (e *LocalEnv) WriteFile(_ context.Context, path string, content []byte) harness.Result[struct{}] {
	resolved := e.resolvePath(path)
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return harness.ErrResult[struct{}](toFileError(err, resolved))
	}
	if err := os.WriteFile(resolved, content, 0o644); err != nil {
		return harness.ErrResult[struct{}](toFileError(err, resolved))
	}
	return harness.OkResult(struct{}{})
}

// AppendFile creates or appends to a file.
func (e *LocalEnv) AppendFile(_ context.Context, path string, content []byte) harness.Result[struct{}] {
	resolved := e.resolvePath(path)
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return harness.ErrResult[struct{}](toFileError(err, resolved))
	}
	f, err := os.OpenFile(resolved, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return harness.ErrResult[struct{}](toFileError(err, resolved))
	}
	defer f.Close()
	if _, err := f.Write(content); err != nil {
		return harness.ErrResult[struct{}](toFileError(err, resolved))
	}
	return harness.OkResult(struct{}{})
}

// FileInfo returns metadata for the addressed path.
func (e *LocalEnv) FileInfo(_ context.Context, path string) harness.Result[harness.FileInfo] {
	resolved := e.resolvePath(path)
	info, err := os.Lstat(resolved)
	if err != nil {
		return harness.ErrResult[harness.FileInfo](toFileError(err, resolved))
	}
	kind := fileKindFromMode(info.Mode())
	if kind == "" {
		return harness.ErrResult[harness.FileInfo](harness.NewFileError("invalid", "Unsupported file type", resolved, nil))
	}
	return harness.OkResult(harness.FileInfo{
		Name:    filepath.Base(resolved),
		Path:    resolved,
		Kind:    kind,
		Size:    info.Size(),
		MtimeMs: info.ModTime().UnixMilli(),
	})
}

// ListDir lists direct children of a directory.
func (e *LocalEnv) ListDir(_ context.Context, path string) harness.Result[[]harness.FileInfo] {
	resolved := e.resolvePath(path)
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return harness.ErrResult[[]harness.FileInfo](toFileError(err, resolved))
	}
	infos := make([]harness.FileInfo, 0, len(entries))
	for _, entry := range entries {
		entryPath := filepath.Join(resolved, entry.Name())
		info, err := entry.Info()
		if err != nil {
			// Skip broken symlinks etc.
			continue
		}
		kind := fileKindFromMode(info.Mode())
		if kind == "" {
			continue
		}
		infos = append(infos, harness.FileInfo{
			Name:    entry.Name(),
			Path:    entryPath,
			Kind:    kind,
			Size:    info.Size(),
			MtimeMs: info.ModTime().UnixMilli(),
		})
	}
	return harness.OkResult(infos)
}

// CanonicalPath returns the canonical path, resolving symlinks.
func (e *LocalEnv) CanonicalPath(_ context.Context, path string) harness.Result[string] {
	resolved := e.resolvePath(path)
	canonical, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		return harness.ErrResult[string](toFileError(err, resolved))
	}
	return harness.OkResult(canonical)
}

// Exists returns false for missing paths.
func (e *LocalEnv) Exists(_ context.Context, path string) harness.Result[bool] {
	resolved := e.resolvePath(path)
	_, err := os.Stat(resolved)
	if err == nil {
		return harness.OkResult(true)
	}
	if os.IsNotExist(err) {
		return harness.OkResult(false)
	}
	return harness.ErrResult[bool](toFileError(err, resolved))
}

// CreateDir creates a directory.
func (e *LocalEnv) CreateDir(_ context.Context, path string, recursive bool) harness.Result[struct{}] {
	resolved := e.resolvePath(path)
	if recursive {
		if err := os.MkdirAll(resolved, 0o755); err != nil {
			return harness.ErrResult[struct{}](toFileError(err, resolved))
		}
	} else {
		if err := os.Mkdir(resolved, 0o755); err != nil {
			return harness.ErrResult[struct{}](toFileError(err, resolved))
		}
	}
	return harness.OkResult(struct{}{})
}

// Remove removes a file or directory.
func (e *LocalEnv) Remove(_ context.Context, path string, recursive bool, force bool) harness.Result[struct{}] {
	resolved := e.resolvePath(path)
	if recursive && force {
		err := os.RemoveAll(resolved)
		if err != nil {
			return harness.ErrResult[struct{}](toFileError(err, resolved))
		}
	} else if recursive {
		err := os.RemoveAll(resolved)
		if err != nil {
			return harness.ErrResult[struct{}](toFileError(err, resolved))
		}
	} else {
		err := os.Remove(resolved)
		if err != nil && !os.IsNotExist(err) {
			return harness.ErrResult[struct{}](toFileError(err, resolved))
		}
	}
	return harness.OkResult(struct{}{})
}

// CreateTempDir creates a temporary directory.
func (e *LocalEnv) CreateTempDir(_ context.Context, prefix string) harness.Result[string] {
	path, err := os.MkdirTemp("", prefix)
	if err != nil {
		return harness.ErrResult[string](toFileError(err, ""))
	}
	return harness.OkResult(path)
}

// CreateTempFile creates a temporary file.
func (e *LocalEnv) CreateTempFile(_ context.Context, prefix, suffix string) harness.Result[string] {
	dir, err := os.MkdirTemp("", "tmp-")
	if err != nil {
		return harness.ErrResult[string](toFileError(err, ""))
	}
	filePath := filepath.Join(dir, prefix+generateID()+suffix)
	if err := os.WriteFile(filePath, nil, 0o644); err != nil {
		return harness.ErrResult[string](toFileError(err, filePath))
	}
	return harness.OkResult(filePath)
}

// Cleanup releases resources (no-op for local env).
func (e *LocalEnv) Cleanup(_ context.Context) {}

// ============================================================================
// Shell interface
// ============================================================================

// Exec executes a shell command.
func (e *LocalEnv) Exec(ctx context.Context, command string, opts *harness.ExecOptions) harness.Result[harness.ExecResult] {
	if ctx.Err() != nil {
		return harness.ErrResult[harness.ExecResult](harness.NewExecutionError("aborted", "context cancelled", nil))
	}

	shell, args := e.getShellConfig()

	cwd := e.cwd
	if opts != nil && opts.Cwd != "" {
		if filepath.IsAbs(opts.Cwd) {
			cwd = opts.Cwd
		} else {
			cwd = filepath.Join(e.cwd, opts.Cwd)
		}
	}

	cmd := exec.CommandContext(ctx, shell, append(args, command)...)
	cmd.Dir = cwd
	cmd.Env = e.buildEnv(opts)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Set process group for killing tree
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if opts != nil && opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(opts.Timeout)*time.Second)
		defer cancel()
		cmd = exec.CommandContext(ctx, shell, append(args, command)...)
		cmd.Dir = cwd
		cmd.Env = e.buildEnv(opts)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}

	// Stream output if callbacks provided
	if opts != nil && (opts.OnStdout != nil || opts.OnStderr != nil) {
		stdoutWriter := &callbackWriter{fn: opts.OnStdout, buf: &stdout}
		stderrWriter := &callbackWriter{fn: opts.OnStderr, buf: &stderr}
		cmd.Stdout = stdoutWriter
		cmd.Stderr = stderrWriter
	}

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if ctx.Err() != nil {
			return harness.ErrResult[harness.ExecResult](harness.NewExecutionError("aborted", "aborted", nil))
		} else {
			return harness.ErrResult[harness.ExecResult](harness.NewExecutionError("spawn_error", err.Error(), err))
		}
	}

	return harness.OkResult(harness.ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	})
}

// ============================================================================
// Internal helpers
// ============================================================================

func (e *LocalEnv) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(e.cwd, path)
}

func (e *LocalEnv) getShellConfig() (string, []string) {
	if e.shellPath != "" {
		return e.shellPath, []string{"-c"}
	}
	// Default to bash, fall back to sh
	for _, candidate := range []string{"/bin/bash", "/bin/sh"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, []string{"-c"}
		}
	}
	return "sh", []string{"-c"}
}

func (e *LocalEnv) buildEnv(opts *harness.ExecOptions) []string {
	env := os.Environ()
	if e.shellEnv != nil {
		env = append(env, e.shellEnv...)
	}
	if opts != nil && opts.Env != nil {
		for k, v := range opts.Env {
			env = append(env, k+"="+v)
		}
	}
	return env
}

func fileKindFromMode(mode fs.FileMode) harness.FileKind {
	switch {
	case mode&fs.ModeSymlink != 0:
		return harness.FileKindSymlink
	case mode.IsRegular():
		return harness.FileKindFile
	case mode.IsDir():
		return harness.FileKindDirectory
	}
	return ""
}

func toFileError(err error, path string) *harness.FileError {
	if err == nil {
		return nil
	}
	if fe, ok := err.(*harness.FileError); ok {
		return fe
	}
	// Map OS errors to file error codes
	if os.IsNotExist(err) {
		return harness.NewFileError("not_found", err.Error(), path, err)
	}
	if os.IsPermission(err) {
		return harness.NewFileError("permission_denied", err.Error(), path, err)
	}
	if strings.Contains(err.Error(), "not a directory") {
		return harness.NewFileError("not_directory", err.Error(), path, err)
	}
	if strings.Contains(err.Error(), "is a directory") {
		return harness.NewFileError("is_directory", err.Error(), path, err)
	}
	return harness.NewFileError("unknown", err.Error(), path, err)
}

func generateID() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

// callbackWriter wraps an io.Writer that calls a callback on each Write.
type callbackWriter struct {
	fn  func(string)
	buf *bytes.Buffer
	mu  sync.Mutex
}

func (w *callbackWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.buf.Write(p)
	if err == nil && w.fn != nil {
		w.fn(string(p))
	}
	return n, err
}

// Ensure LocalEnv implements the interfaces at compile time.
var (
	_ harness.FileSystem   = (*LocalEnv)(nil)
	_ harness.Shell        = (*LocalEnv)(nil)
	_ harness.ExecutionEnv = (*LocalEnv)(nil)
)
