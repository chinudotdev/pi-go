package harness

import (
	"path/filepath"
	"strings"
)

// ignoreMatcher is a simplified gitignore-style pattern matcher.
type ignoreMatcher struct {
	patterns []string
}

func newIgnoreMatcher() *ignoreMatcher {
	return &ignoreMatcher{}
}

func (ig *ignoreMatcher) addPattern(pattern string) {
	ig.patterns = append(ig.patterns, pattern)
}

func (ig *ignoreMatcher) matches(path string) bool {
	for _, pattern := range ig.patterns {
		if matchIgnorePattern(pattern, path) {
			return true
		}
	}
	return false
}

// matchIgnorePattern does simple glob-style matching.
// Supports: exact match, * wildcard, directory prefix (trailing /), negation (!).
func matchIgnorePattern(pattern, path string) bool {
	negated := false
	if strings.HasPrefix(pattern, "!") {
		negated = true
		pattern = pattern[1:]
	}

	dirOnly := false
	if strings.HasSuffix(pattern, "/") {
		dirOnly = true
		pattern = strings.TrimSuffix(pattern, "/")
	}
	_ = dirOnly // reserved for future directory-only matching

	matched := false

	// Exact match
	if pattern == path {
		matched = true
	}

	// Directory prefix match
	if !matched && strings.HasSuffix(pattern, "/") {
		prefix := strings.TrimSuffix(pattern, "/")
		if strings.HasPrefix(path, prefix+"/") || path == prefix {
			matched = true
		}
	}

	// Glob with *
	if !matched {
		matched = globMatch(pattern, path)
	}

	// Basename match (if pattern has no /)
	if !matched && !strings.Contains(pattern, "/") {
		base := filepath.Base(path)
		matched = globMatch(pattern, base)
	}

	if negated {
		return !matched
	}
	return matched
}

func globMatch(pattern, str string) bool {
	if pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return pattern == str
	}
	// Simple * glob: split on *, check each segment appears in order
	parts := strings.SplitN(pattern, "*", -1)
	idx := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		pos := strings.Index(str[idx:], part)
		if pos < 0 {
			return false
		}
		if i == 0 && pos != 0 {
			return false
		}
		idx += pos + len(part)
	}
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		if last != "" && !strings.HasSuffix(str, last) {
			return false
		}
	}
	return true
}
