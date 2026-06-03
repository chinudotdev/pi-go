package shell

import (
	"os"
	"testing"
)

func TestGetConfigBash(t *testing.T) {
	cfg, err := GetConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Shell == "" {
		t.Error("expected a shell path")
	}
	if len(cfg.Args) != 1 || cfg.Args[0] != "-c" {
		t.Errorf("expected args [-c], got %v", cfg.Args)
	}
}

func TestGetConfigCustomPath(t *testing.T) {
	// /bin/sh should always exist on Unix
	cfg, err := GetConfig("/bin/sh")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Shell != "/bin/sh" {
		t.Errorf("expected /bin/sh, got %s", cfg.Shell)
	}
}

func TestGetConfigInvalidCustomPath(t *testing.T) {
	_, err := GetConfig("/nonexistent/shell/binary")
	if err == nil {
		t.Error("expected error for nonexistent shell path")
	}
}

func TestDetect(t *testing.T) {
	original := os.Getenv("SHELL")
	os.Setenv("SHELL", "/bin/zsh")
	defer os.Setenv("SHELL", original)

	shell := Detect()
	if shell != "zsh" {
		t.Errorf("expected zsh, got %s", shell)
	}
}

func TestDetectEmpty(t *testing.T) {
	original := os.Getenv("SHELL")
	os.Unsetenv("SHELL")
	defer os.Setenv("SHELL", original)

	shell := Detect()
	if shell != "unknown" {
		t.Errorf("expected unknown, got %s", shell)
	}
}
