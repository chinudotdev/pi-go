// Package config provides application configuration paths and constants
// for the pi coding agent SDK.
package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Application identity constants.
const (
	AppName       = "pi"
	AppTitle      = "π"
	ConfigDirName = ".pi"
	Version       = "0.1.0" // TODO: wire from build-time ldflags
)

// Environment variable names.
var (
	EnvAgentDir   = AppName + "_CODING_AGENT_DIR"
	EnvSessionDir = AppName + "_CODING_AGENT_SESSION_DIR"
)

// GetAgentDir returns the agent configuration directory.
// It respects the PI_CODING_AGENT_DIR environment variable override,
// otherwise defaults to ~/.pi/agent/.
func GetAgentDir() string {
	if envDir := os.Getenv(EnvAgentDir); envDir != "" {
		return ExpandTildePath(envDir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/"
	}
	return filepath.Join(home, ConfigDirName, "agent")
}

// GetModelsPath returns the path to models.json.
func GetModelsPath() string {
	return filepath.Join(GetAgentDir(), "models.json")
}

// GetAuthPath returns the path to auth.json.
func GetAuthPath() string {
	return filepath.Join(GetAgentDir(), "auth.json")
}

// GetSettingsPath returns the path to settings.json.
func GetSettingsPath() string {
	return filepath.Join(GetAgentDir(), "settings.json")
}

// GetSessionsDir returns the path to the sessions directory.
func GetSessionsDir() string {
	return filepath.Join(GetAgentDir(), "sessions")
}

// GetPromptsDir returns the path to user prompt templates.
func GetPromptsDir() string {
	return filepath.Join(GetAgentDir(), "prompts")
}

// GetBinDir returns the path to managed binaries (fd, rg, etc.).
func GetBinDir() string {
	return filepath.Join(GetAgentDir(), "bin")
}

// GetDebugLogPath returns the path to the debug log file.
func GetDebugLogPath() string {
	return filepath.Join(GetAgentDir(), AppName+"-debug.log")
}

// GetProjectConfigDir returns the per-project config directory (e.g. .pi/)
// relative to the given cwd.
func GetProjectConfigDir(cwd string) string {
	return filepath.Join(cwd, ConfigDirName)
}

// ExpandTildePath expands a leading ~ to the user's home directory.
func ExpandTildePath(path string) string {
	if path == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	// Handle Windows-style ~\ paths
	if runtime.GOOS == "windows" && strings.HasPrefix(path, "~\\") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
