// Package configvalue resolves configuration values that may contain
// shell commands, environment variable references, or literal strings.
//
// Resolution rules:
//   - "!command" → execute as shell command, return trimmed stdout (cached)
//   - "$VAR" or "${VAR}" → environment variable lookup
//   - "$$" → literal "$", "$!" → literal "!"
//   - otherwise → literal string
package configvalue

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Cache for shell command results (persists for process lifetime).
var (
	cacheMu   sync.Mutex
	cmdCache  = make(map[string]*string)
)

// Env variable name validation.
var (
	envVarNameRE     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	envVarPrefixRE   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*`)
	legacyEnvVarName = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
)

// templatePart is either a literal string or an env var reference.
type templatePart struct {
	typ  string // "literal" or "env"
	val  string // literal value or env var name
}

// configValueRef is a parsed config value: either a shell command or a template.
type configValueRef struct {
	typ   string // "command" or "template"
	parts []templatePart
}

// parseConfigValueTemplate parses "$VAR", "${VAR}", "$$", "$!" references
// in a config string, returning ordered template parts.
func parseConfigValueTemplate(config string) []templatePart {
	var parts []templatePart
	i := 0

	appendLiteral := func(val string) {
		if val == "" {
			return
		}
		if len(parts) > 0 && parts[len(parts)-1].typ == "literal" {
			parts[len(parts)-1].val += val
			return
		}
		parts = append(parts, templatePart{typ: "literal", val: val})
	}

	for i < len(config) {
		dollarIdx := strings.IndexByte(config[i:], '$')
		if dollarIdx < 0 {
			appendLiteral(config[i:])
			break
		}
		dollarIdx += i

		appendLiteral(config[i:dollarIdx])

		if dollarIdx+1 >= len(config) {
			appendLiteral("$")
			break
		}
		next := config[dollarIdx+1]

		// Escaped: $$ → $, $! → !
		if next == '$' || next == '!' {
			appendLiteral(string(next))
			i = dollarIdx + 2
			continue
		}

		// ${VAR} form
		if next == '{' {
			endIdx := strings.IndexByte(config[dollarIdx+2:], '}')
			if endIdx < 0 {
				appendLiteral("$")
				i = dollarIdx + 1
				continue
			}
			endIdx += dollarIdx + 2
			name := config[dollarIdx+2 : endIdx]
			if envVarNameRE.MatchString(name) {
				parts = append(parts, templatePart{typ: "env", val: name})
			} else {
				appendLiteral(config[dollarIdx : endIdx+1])
			}
			i = endIdx + 1
			continue
		}

		// $VAR form
		remaining := config[dollarIdx+1:]
		m := envVarPrefixRE.FindString(remaining)
		if m != "" {
			parts = append(parts, templatePart{typ: "env", val: m})
			i = dollarIdx + 1 + len(m)
			continue
		}

		appendLiteral("$")
		i = dollarIdx + 1
	}

	return parts
}

func parseConfigValueRef(config string) configValueRef {
	if strings.HasPrefix(config, "!") {
		return configValueRef{typ: "command"}
	}
	return configValueRef{typ: "template", parts: parseConfigValueTemplate(config)}
}

func resolveTemplate(parts []templatePart) *string {
	var sb strings.Builder
	for _, p := range parts {
		if p.typ == "literal" {
			sb.WriteString(p.val)
			continue
		}
		val, ok := os.LookupEnv(p.val)
		if !ok {
			return nil
		}
		sb.WriteString(val)
	}
	result := sb.String()
	return &result
}

func templateEnvVarNames(parts []templatePart) []string {
	seen := make(map[string]bool)
	var names []string
	for _, p := range parts {
		if p.typ == "env" && !seen[p.val] {
			seen[p.val] = true
			names = append(names, p.val)
		}
	}
	return names
}

// ResolveConfigValue resolves a config value to a concrete string.
// Returns nil if the value cannot be resolved (missing env var, command failure).
func ResolveConfigValue(config string) *string {
	ref := parseConfigValueRef(config)
	if ref.typ == "command" {
		return executeCommand(config)
	}
	return resolveTemplate(ref.parts)
}

// ResolveConfigValueUncached resolves a config value without caching command results.
func ResolveConfigValueUncached(config string) *string {
	ref := parseConfigValueRef(config)
	if ref.typ == "command" {
		return executeCommandUncached(config)
	}
	return resolveTemplate(ref.parts)
}

// ResolveConfigValueOrThrow resolves a config value, panicking with a descriptive
// error on failure.
func ResolveConfigValueOrThrow(config string, description string) string {
	result := ResolveConfigValueUncached(config)
	if result != nil {
		return *result
	}

	ref := parseConfigValueRef(config)
	if ref.typ == "command" {
		panic("failed to resolve " + description + " from shell command: " + config[1:])
	}

	missing := GetMissingConfigValueEnvVarNames(config)
	if len(missing) == 1 {
		panic("failed to resolve " + description + " from environment variable: " + missing[0])
	}
	if len(missing) > 1 {
		panic("failed to resolve " + description + " from environment variables: " + strings.Join(missing, ", "))
	}

	panic("failed to resolve " + description)
}

// ResolveHeaders resolves all values in a header map using ResolveConfigValue.
// Returns nil if no headers resolved successfully.
func ResolveHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}
	resolved := make(map[string]string)
	for k, v := range headers {
		if val := ResolveConfigValue(v); val != nil {
			resolved[k] = *val
		}
	}
	if len(resolved) == 0 {
		return nil
	}
	return resolved
}

// ResolveHeadersOrThrow resolves all header values, panicking on any failure.
func ResolveHeadersOrThrow(headers map[string]string, description string) map[string]string {
	if headers == nil {
		return nil
	}
	resolved := make(map[string]string)
	for k, v := range headers {
		resolved[k] = ResolveConfigValueOrThrow(v, description+" header \""+k+"\"")
	}
	return resolved
}

// GetConfigValueEnvVarName returns the single env var name if config is a
// plain "$VAR" reference, otherwise nil.
func GetConfigValueEnvVarName(config string) *string {
	ref := parseConfigValueRef(config)
	if ref.typ != "template" || len(ref.parts) != 1 {
		return nil
	}
	if ref.parts[0].typ == "env" {
		return &ref.parts[0].val
	}
	return nil
}

// GetConfigValueEnvVarNames returns all env var names referenced in the config.
func GetConfigValueEnvVarNames(config string) []string {
	ref := parseConfigValueRef(config)
	if ref.typ != "template" {
		return nil
	}
	return templateEnvVarNames(ref.parts)
}

// GetMissingConfigValueEnvVarNames returns env var names that are unset.
func GetMissingConfigValueEnvVarNames(config string) []string {
	var missing []string
	for _, name := range GetConfigValueEnvVarNames(config) {
		if _, ok := os.LookupEnv(name); !ok {
			missing = append(missing, name)
		}
	}
	return missing
}

// IsCommandConfigValue returns true if the config is a shell command ("!...").
func IsCommandConfigValue(config string) bool {
	return parseConfigValueRef(config).typ == "command"
}

// IsConfigValueConfigured returns true if all referenced env vars are set.
func IsConfigValueConfigured(config string) bool {
	return len(GetMissingConfigValueEnvVarNames(config)) == 0
}

// IsLegacyEnvVarName returns true if the config is a legacy-style env var name
// (all uppercase with underscores).
func IsLegacyEnvVarName(config string) bool {
	return legacyEnvVarName.MatchString(config)
}

// ClearCache clears the shell command result cache. Exported for testing.
func ClearCache() {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	cmdCache = make(map[string]*string)
}

// executeCommand runs a shell command (after "!") and returns trimmed stdout.
// Results are cached for the process lifetime.
func executeCommand(config string) *string {
	cacheMu.Lock()
	if cached, ok := cmdCache[config]; ok {
		cacheMu.Unlock()
		return cached
	}
	cacheMu.Unlock()

	result := executeCommandUncached(config)

	cacheMu.Lock()
	cmdCache[config] = result
	cacheMu.Unlock()

	return result
}

// executeCommandUncached runs a shell command without caching.
func executeCommandUncached(config string) *string {
	command := config[1:] // strip leading "!"

	shell, args := getShellAndArgs()
	cmd := exec.Command(shell, append(args, command)...)
	cmd.Stdin = nil
	cmd.Stderr = nil

	var out []byte
	done := make(chan struct{})
	go func() {
		out, _ = cmd.Output()
		close(done)
	}()

	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()

	select {
	case <-done:
		// ok
	case <-timer.C:
		_ = cmd.Process.Kill()
		return nil
	}

	result := strings.TrimSpace(string(out))
	if result == "" {
		return nil
	}
	return &result
}

// getShellAndArgs returns the shell and arguments for command execution.
func getShellAndArgs() (string, []string) {
	// On Unix, prefer /bin/bash then /bin/sh
	// On Windows, would need to find Git Bash — but we don't target Windows yet
	if _, err := os.Stat("/bin/bash"); err == nil {
		return "/bin/bash", []string{"-c"}
	}
	return "/bin/sh", []string{"-c"}
}
