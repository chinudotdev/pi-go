package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeTilde(t *testing.T) {
	home, _ := os.UserHomeDir()

	result := NormalizePath("~")
	if result != home {
		t.Errorf("expected %s, got %s", home, result)
	}

	result = NormalizePath("~/foo/bar")
	expected := filepath.Join(home, "foo", "bar")
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestNormalizeNoTilde(t *testing.T) {
	result := NormalizePath("/abs/path", NormalizeOptions{ExpandTilde: false})
	if result != "/abs/path" {
		t.Errorf("expected /abs/path, got %s", result)
	}
}

func TestNormalizeTrim(t *testing.T) {
	result := NormalizePath("  hello  ", NormalizeOptions{Trim: true})
	if result != "hello" {
		t.Errorf("expected hello, got %s", result)
	}
}

func TestNormalizeStripAt(t *testing.T) {
	result := NormalizePath("@file.txt", NormalizeOptions{StripAtPrefix: true})
	if result != "file.txt" {
		t.Errorf("expected file.txt, got %s", result)
	}
}

func TestResolvePath(t *testing.T) {
	result := ResolvePath("foo/bar", "/home/user")
	expected := "/home/user/foo/bar"
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestResolvePathAbsolute(t *testing.T) {
	result := ResolvePath("/abs/path", "/home/user")
	if result != "/abs/path" {
		t.Errorf("expected /abs/path, got %s", result)
	}
}

func TestGetCwdRelativePathInside(t *testing.T) {
	result := GetCwdRelativePath("/home/user/src/file.go", "/home/user/src")
	if result != "file.go" {
		t.Errorf("expected file.go, got %q", result)
	}
}

func TestGetCwdRelativePathOutside(t *testing.T) {
	result := GetCwdRelativePath("/home/other/file.go", "/home/user")
	if result != "" {
		t.Errorf("expected empty for outside cwd, got %q", result)
	}
}

func TestGetCwdRelativePathDot(t *testing.T) {
	result := GetCwdRelativePath("/home/user/src", "/home/user/src")
	if result != "." {
		t.Errorf("expected ., got %q", result)
	}
}
