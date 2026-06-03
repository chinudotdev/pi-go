// Package shell detects the user's shell configuration.
package shell

import (
	"os"
	"os/exec"
	"strings"
)

// Config holds the resolved shell binary and its arguments.
type Config struct {
	Shell string
	Args  []string
}

// GetConfig resolves the shell configuration.
//
// Resolution order:
//  1. customShellPath (if provided and exists)
//  2. /bin/bash (if exists)
//  3. bash found on PATH
//  4. fallback to /bin/sh
func GetConfig(customShellPath string) (Config, error) {
	if customShellPath != "" {
		if _, err := os.Stat(customShellPath); err == nil {
			return Config{Shell: customShellPath, Args: []string{"-c"}}, nil
		}
		return Config{}, &os.PathError{Op: "stat", Path: customShellPath, Err: os.ErrNotExist}
	}

	// Prefer /bin/bash
	if _, err := os.Stat("/bin/bash"); err == nil {
		return Config{Shell: "/bin/bash", Args: []string{"-c"}}, nil
	}

	// Try bash on PATH
	if path, err := exec.LookPath("bash"); err == nil {
		return Config{Shell: path, Args: []string{"-c"}}, nil
	}

	// Fallback to sh
	return Config{Shell: "/bin/sh", Args: []string{"-c"}}, nil
}

// Detect detects the user's shell from the SHELL environment variable.
// Returns "bash", "zsh", "fish", or "unknown".
func Detect() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return "unknown"
	}
	base := shell[strings.LastIndex(shell, "/")+1:]
	switch base {
	case "bash", "zsh", "fish", "sh", "dash", "ksh", "tcsh":
		return base
	default:
		return "unknown"
	}
}

// GetShellEnv returns the environment with the managed bin directory
// prepended to PATH.
func GetShellEnv(binDir string) []string {
	env := os.Environ()
	pathKey := "PATH"
	for i, e := range env {
		if strings.HasPrefix(strings.ToUpper(e), "PATH=") {
			pathKey = e[:5]
			currentPath := e[5:]
			if strings.Contains(currentPath, binDir) {
				return env // Already includes bin dir
			}
			env[i] = pathKey + "=" + binDir + ":" + currentPath
			return env
		}
	}
	// No PATH found — add one
	env = append(env, "PATH="+binDir)
	return env
}
