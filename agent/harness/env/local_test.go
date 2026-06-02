package env

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/chinudotdev/pi-go/agent/harness"
)

func newTestEnv(t *testing.T) *LocalEnv {
	t.Helper()
	dir, err := os.MkdirTemp("", "localenv-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return NewLocalEnv(dir)
}

func TestLocalEnv_Cwd(t *testing.T) {
	env := newTestEnv(t)
	if env.Cwd() == "" {
		t.Error("expected non-empty cwd")
	}
}

func TestLocalEnv_AbsolutePath(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// Relative path
	result := env.AbsolutePath(ctx, "foo.txt")
	if !result.OK {
		t.Fatal(result.Err)
	}
	if !filepath.IsAbs(result.Value) {
		t.Errorf("expected absolute path, got %s", result.Value)
	}

	// Already absolute
	result = env.AbsolutePath(ctx, "/tmp/test")
	if result.Value != "/tmp/test" {
		t.Errorf("expected /tmp/test, got %s", result.Value)
	}
}

func TestLocalEnv_WriteAndReadTextFile(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	writeResult := env.WriteFile(ctx, "test.txt", []byte("hello world"))
	if !writeResult.OK {
		t.Fatal(writeResult.Err)
	}

	readResult := env.ReadTextFile(ctx, "test.txt")
	if !readResult.OK {
		t.Fatal(readResult.Err)
	}
	if readResult.Value != "hello world" {
		t.Errorf("expected 'hello world', got %q", readResult.Value)
	}
}

func TestLocalEnv_WriteCreatesParentDirs(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	writeResult := env.WriteFile(ctx, "a/b/c/test.txt", []byte("nested"))
	if !writeResult.OK {
		t.Fatal(writeResult.Err)
	}

	readResult := env.ReadTextFile(ctx, "a/b/c/test.txt")
	if !readResult.OK {
		t.Fatal(readResult.Err)
	}
	if readResult.Value != "nested" {
		t.Errorf("expected 'nested', got %q", readResult.Value)
	}
}

func TestLocalEnv_ReadTextFile_NotFound(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	result := env.ReadTextFile(ctx, "nonexistent.txt")
	if result.OK {
		t.Error("expected error for nonexistent file")
	}
	fe, ok := result.Err.(*harness.FileError)
	if !ok {
		t.Errorf("expected FileError, got %T", result.Err)
	}
	if fe.Code != "not_found" {
		t.Errorf("expected not_found, got %s", fe.Code)
	}
}

func TestLocalEnv_ReadTextLines(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	env.WriteFile(ctx, "lines.txt", []byte("line1\nline2\nline3"))

	result := env.ReadTextLines(ctx, "lines.txt", 2)
	if !result.OK {
		t.Fatal(result.Err)
	}
	if len(result.Value) != 2 {
		t.Errorf("expected 2 lines, got %d", len(result.Value))
	}
	if result.Value[0] != "line1" {
		t.Errorf("expected 'line1', got %q", result.Value[0])
	}
}

func TestLocalEnv_ReadBinaryFile(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	data := []byte{0x00, 0x01, 0x02, 0xFF}
	env.WriteFile(ctx, "binary.bin", data)

	result := env.ReadBinaryFile(ctx, "binary.bin")
	if !result.OK {
		t.Fatal(result.Err)
	}
	if len(result.Value) != 4 {
		t.Errorf("expected 4 bytes, got %d", len(result.Value))
	}
}

func TestLocalEnv_AppendFile(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	env.WriteFile(ctx, "append.txt", []byte("first "))
	env.AppendFile(ctx, "append.txt", []byte("second"))

	result := env.ReadTextFile(ctx, "append.txt")
	if !result.OK {
		t.Fatal(result.Err)
	}
	if result.Value != "first second" {
		t.Errorf("expected 'first second', got %q", result.Value)
	}
}

func TestLocalEnv_FileInfo(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	env.WriteFile(ctx, "info.txt", []byte("content"))

	result := env.FileInfo(ctx, "info.txt")
	if !result.OK {
		t.Fatal(result.Err)
	}
	info := result.Value
	if info.Name != "info.txt" {
		t.Errorf("expected info.txt, got %s", info.Name)
	}
	if info.Kind != harness.FileKindFile {
		t.Errorf("expected file kind, got %s", info.Kind)
	}
	if info.Size != 7 {
		t.Errorf("expected size 7, got %d", info.Size)
	}
}

func TestLocalEnv_ListDir(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	env.WriteFile(ctx, "file1.txt", []byte("a"))
	env.WriteFile(ctx, "file2.txt", []byte("b"))
	os.MkdirAll(filepath.Join(env.Cwd(), "subdir"), 0o755)

	result := env.ListDir(ctx, ".")
	if !result.OK {
		t.Fatal(result.Err)
	}
	if len(result.Value) != 3 {
		t.Errorf("expected 3 entries, got %d: %v", len(result.Value), result.Value)
	}
}

func TestLocalEnv_CanonicalPath(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	env.WriteFile(ctx, "real.txt", []byte("content"))

	result := env.CanonicalPath(ctx, "real.txt")
	if !result.OK {
		t.Fatal(result.Err)
	}
	if !filepath.IsAbs(result.Value) {
		t.Errorf("expected absolute path, got %s", result.Value)
	}
}

func TestLocalEnv_Exists(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	env.WriteFile(ctx, "exists.txt", []byte("yes"))

	if result := env.Exists(ctx, "exists.txt"); !result.OK || !result.Value {
		t.Error("expected exists=true")
	}
	if result := env.Exists(ctx, "nope.txt"); !result.OK || result.Value {
		t.Error("expected exists=false")
	}
}

func TestLocalEnv_CreateDir(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	result := env.CreateDir(ctx, "newdir/nested", true)
	if !result.OK {
		t.Fatal(result.Err)
	}
	if stat, err := os.Stat(filepath.Join(env.Cwd(), "newdir", "nested")); err != nil {
		t.Errorf("expected directory to exist: %v", err)
	} else if !stat.IsDir() {
		t.Error("expected directory")
	}
}

func TestLocalEnv_Remove(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	env.WriteFile(ctx, "toremove.txt", []byte("data"))
	result := env.Remove(ctx, "toremove.txt", false, false)
	if !result.OK {
		t.Fatal(result.Err)
	}
	if _, err := os.Stat(filepath.Join(env.Cwd(), "toremove.txt")); !os.IsNotExist(err) {
		t.Error("expected file to be removed")
	}
}

func TestLocalEnv_CreateTempDir(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	result := env.CreateTempDir(ctx, "test-")
	if !result.OK {
		t.Fatal(result.Err)
	}
	if _, err := os.Stat(result.Value); err != nil {
		t.Errorf("expected temp dir to exist: %v", err)
	}
}

func TestLocalEnv_CreateTempFile(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	result := env.CreateTempFile(ctx, "prefix-", ".txt")
	if !result.OK {
		t.Fatal(result.Err)
	}
	if _, err := os.Stat(result.Value); err != nil {
		t.Errorf("expected temp file to exist: %v", err)
	}
}

func TestLocalEnv_Exec_Echo(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	result := env.Exec(ctx, "echo hello", nil)
	if !result.OK {
		t.Fatal(result.Err)
	}
	if result.Value.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.Value.ExitCode)
	}
	if result.Value.Stdout != "hello\n" {
		t.Errorf("expected 'hello\\n', got %q", result.Value.Stdout)
	}
}

func TestLocalEnv_Exec_ExitCode(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	result := env.Exec(ctx, "exit 42", nil)
	if !result.OK {
		t.Fatal(result.Err)
	}
	if result.Value.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", result.Value.ExitCode)
	}
}

func TestLocalEnv_Exec_WithCwd(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// Create a subdirectory and write a file
	os.MkdirAll(filepath.Join(env.Cwd(), "sub"), 0o755)
	env.WriteFile(ctx, "sub/test.txt", []byte("found"))

	result := env.Exec(ctx, "cat test.txt", &harness.ExecOptions{Cwd: "sub"})
	if !result.OK {
		t.Fatal(result.Err)
	}
	if result.Value.Stdout != "found" {
		t.Errorf("expected 'found', got %q", result.Value.Stdout)
	}
}

func TestLocalEnv_JoinPath(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	result := env.JoinPath(ctx, "a", "b", "c.txt")
	if !result.OK {
		t.Fatal(result.Err)
	}
	expected := filepath.Join("a", "b", "c.txt")
	if result.Value != expected {
		t.Errorf("expected %s, got %s", expected, result.Value)
	}
}
