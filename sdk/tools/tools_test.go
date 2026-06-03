package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/chinudotdev/pi-go/agent"
)

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "pi-tools-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestWriteTool_CreatesFile(t *testing.T) {
	cwd := tempDir(t)
	tool := CreateWriteTool(cwd, nil)

	result, err := tool.Execute(context.Background(), "tc1", map[string]any{
		"path":    "test.txt",
		"content": "hello world",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content")
	}

	data, err := os.ReadFile(filepath.Join(cwd, "test.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(data))
	}
}

func TestWriteTool_CreatesParentDirs(t *testing.T) {
	cwd := tempDir(t)
	tool := CreateWriteTool(cwd, nil)

	_, err := tool.Execute(context.Background(), "tc1", map[string]any{
		"path":    "sub/deep/test.txt",
		"content": "nested",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(cwd, "sub/deep/test.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "nested" {
		t.Errorf("expected 'nested', got %q", string(data))
	}
}

func TestWriteTool_RequiresPath(t *testing.T) {
	tool := CreateWriteTool(tempDir(t), nil)
	_, err := tool.Execute(context.Background(), "tc1", map[string]any{
		"content": "test",
	}, nil)
	if err == nil {
		t.Error("expected error for missing path")
	}
}

func TestReadTool_ReadsFile(t *testing.T) {
	cwd := tempDir(t)
	os.WriteFile(filepath.Join(cwd, "test.txt"), []byte("line1\nline2\nline3"), 0644)

	tool := CreateReadTool(cwd, nil)
	result, err := tool.Execute(context.Background(), "tc1", map[string]any{
		"path": "test.txt",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content")
	}
	text := result.Content[0].Text
	if text != "line1\nline2\nline3" {
		t.Errorf("unexpected content: %q", text)
	}
}

func TestReadTool_WithOffset(t *testing.T) {
	cwd := tempDir(t)
	os.WriteFile(filepath.Join(cwd, "test.txt"), []byte("line1\nline2\nline3\nline4\nline5"), 0644)

	tool := CreateReadTool(cwd, nil)
	result, err := tool.Execute(context.Background(), "tc1", map[string]any{
		"path":   "test.txt",
		"offset": float64(2),
		"limit":  float64(2),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	text := result.Content[0].Text
	// File has 5 lines, offset=2, limit=2 reads lines 2-3, shows continuation notice
	if !containsStr(text, "line2\nline3") {
		t.Errorf("expected content to contain lines 2-3, got: %q", text)
	}
}

func TestReadTool_FileNotFound(t *testing.T) {
	tool := CreateReadTool(tempDir(t), nil)
	_, err := tool.Execute(context.Background(), "tc1", map[string]any{
		"path": "nonexistent.txt",
	}, nil)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestEditTool_SingleEdit(t *testing.T) {
	cwd := tempDir(t)
	os.WriteFile(filepath.Join(cwd, "test.txt"), []byte("hello world"), 0644)

	tool := CreateEditTool(cwd, nil)
	result, err := tool.Execute(context.Background(), "tc1", map[string]any{
		"path": "test.txt",
		"edits": []any{
			map[string]any{"oldText": "world", "newText": "Go"},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content")
	}

	data, _ := os.ReadFile(filepath.Join(cwd, "test.txt"))
	if string(data) != "hello Go" {
		t.Errorf("expected 'hello Go', got %q", string(data))
	}
}

func TestEditTool_MultipleEdits(t *testing.T) {
	cwd := tempDir(t)
	os.WriteFile(filepath.Join(cwd, "test.txt"), []byte("aaa\nbbb\nccc\nddd"), 0644)

	tool := CreateEditTool(cwd, nil)
	_, err := tool.Execute(context.Background(), "tc1", map[string]any{
		"path": "test.txt",
		"edits": []any{
			map[string]any{"oldText": "aaa", "newText": "AAA"},
			map[string]any{"oldText": "ccc", "newText": "CCC"},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(cwd, "test.txt"))
	if string(data) != "AAA\nbbb\nCCC\nddd" {
		t.Errorf("unexpected content: %q", string(data))
	}
}

func TestEditTool_NoMatch(t *testing.T) {
	cwd := tempDir(t)
	os.WriteFile(filepath.Join(cwd, "test.txt"), []byte("hello world"), 0644)

	tool := CreateEditTool(cwd, nil)
	_, err := tool.Execute(context.Background(), "tc1", map[string]any{
		"path": "test.txt",
		"edits": []any{
			map[string]any{"oldText": "nonexistent", "newText": "xxx"},
		},
	}, nil)
	if err == nil {
		t.Error("expected error for missing text")
	}
}

func TestEditTool_OverlapError(t *testing.T) {
	cwd := tempDir(t)
	os.WriteFile(filepath.Join(cwd, "test.txt"), []byte("aaa bbb ccc"), 0644)

	tool := CreateEditTool(cwd, nil)
	_, err := tool.Execute(context.Background(), "tc1", map[string]any{
		"path": "test.txt",
		"edits": []any{
			map[string]any{"oldText": "aaa bbb", "newText": "XXX"},
			map[string]any{"oldText": "bbb ccc", "newText": "YYY"},
		},
	}, nil)
	if err == nil {
		t.Error("expected error for overlapping edits")
	}
}

func TestEditTool_LegacyForm(t *testing.T) {
	cwd := tempDir(t)
	os.WriteFile(filepath.Join(cwd, "test.txt"), []byte("hello world"), 0644)

	tool := CreateEditTool(cwd, nil)
	// Test prepareArguments converts old form
	args := map[string]any{
		"path":    "test.txt",
		"oldText": "world",
		"newText": "Go",
	}
	args = tool.PrepareArguments(args)
	_, err := tool.Execute(context.Background(), "tc1", args, nil)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(cwd, "test.txt"))
	if string(data) != "hello Go" {
		t.Errorf("expected 'hello Go', got %q", string(data))
	}
}

func TestLsTool_ListsDirectory(t *testing.T) {
	cwd := tempDir(t)
	os.WriteFile(filepath.Join(cwd, "file1.txt"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(cwd, "file2.go"), []byte("x"), 0644)
	os.Mkdir(filepath.Join(cwd, "subdir"), 0755)

	tool := CreateLsTool(cwd, nil)
	result, err := tool.Execute(context.Background(), "tc1", map[string]any{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	text := result.Content[0].Text
	if text == "(empty directory)" {
		t.Fatal("expected entries")
	}
	// Should have file1.txt, file2.go, subdir/
	if !contains(text, "subdir/") {
		t.Errorf("expected subdir/ in output: %q", text)
	}
}

func TestLsTool_NotADirectory(t *testing.T) {
	cwd := tempDir(t)
	os.WriteFile(filepath.Join(cwd, "file.txt"), []byte("x"), 0644)

	tool := CreateLsTool(cwd, nil)
	_, err := tool.Execute(context.Background(), "tc1", map[string]any{
		"path": "file.txt",
	}, nil)
	if err == nil {
		t.Error("expected error for file path")
	}
}

func TestCreateCodingTools(t *testing.T) {
	cwd := tempDir(t)
	tools := CreateCodingTools(cwd, nil)
	if len(tools) != 4 {
		t.Errorf("expected 4 tools, got %d", len(tools))
	}
	expected := []string{"read", "bash", "edit", "write"}
	for i, name := range expected {
		if tools[i].Name != name {
			t.Errorf("tool[%d]: expected %s, got %s", i, name, tools[i].Name)
		}
	}
}

func TestCreateReadOnlyTools(t *testing.T) {
	cwd := tempDir(t)
	tools := CreateReadOnlyTools(cwd, nil)
	if len(tools) != 4 {
		t.Errorf("expected 4 tools, got %d", len(tools))
	}
}

func TestCreateAllTools(t *testing.T) {
	cwd := tempDir(t)
	all := CreateAllTools(cwd, nil)
	if len(all) != 7 {
		t.Errorf("expected 7 tools, got %d", len(all))
	}
}

func TestCreateTool_UnknownName(t *testing.T) {
	_, err := CreateTool("unknown", tempDir(t), nil)
	if err == nil {
		t.Error("expected error for unknown tool")
	}
}

// Verify tools satisfy agent.Tool interface at compile time
func TestToolImplementsAgentTool(t *testing.T) {
	cwd := tempDir(t)
	var _ *agent.Tool = CreateReadTool(cwd, nil)
	var _ *agent.Tool = CreateBashTool(cwd, nil)
	var _ *agent.Tool = CreateEditTool(cwd, nil)
	var _ *agent.Tool = CreateWriteTool(cwd, nil)
	var _ *agent.Tool = CreateGrepTool(cwd, nil)
	var _ *agent.Tool = CreateFindTool(cwd, nil)
	var _ *agent.Tool = CreateLsTool(cwd, nil)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
