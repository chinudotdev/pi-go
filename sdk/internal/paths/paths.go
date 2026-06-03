// Package paths provides path normalization and resolution utilities.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var unicodeSpacesRe = strings.NewReplacer(
	"\u00A0", " ",
	"\u2000", " ",
	"\u2001", " ",
	"\u2002", " ",
	"\u2003", " ",
	"\u2004", " ",
	"\u2005", " ",
	"\u2006", " ",
	"\u2007", " ",
	"\u2008", " ",
	"\u2009", " ",
	"\u200A", " ",
	"\u202F", " ",
	"\u205F", " ",
	"\u3000", " ",
)

// NormalizeOptions controls NormalizePath behavior.
type NormalizeOptions struct {
	Trim                  bool
	ExpandTilde           bool // default true
	HomeDir               string
	StripAtPrefix         bool
	NormalizeUnicodeSpaces bool
}

// NormalizePath normalizes a path string with the given options.
func NormalizePath(input string, opts ...NormalizeOptions) string {
	opt := NormalizeOptions{ExpandTilde: true}
	if len(opts) > 0 {
		opt = opts[0]
	}

	s := input
	if opt.Trim {
		s = strings.TrimSpace(s)
	}
	if opt.NormalizeUnicodeSpaces {
		s = unicodeSpacesRe.Replace(s)
	}
	if opt.StripAtPrefix && strings.HasPrefix(s, "@") {
		s = s[1:]
	}

	if opt.ExpandTilde {
		home := opt.HomeDir
		if home == "" {
			home, _ = os.UserHomeDir()
		}
		if s == "~" {
			return home
		}
		if strings.HasPrefix(s, "~/") || (runtime.GOOS == "windows" && strings.HasPrefix(s, "~\\")) {
			return filepath.Join(home, s[2:])
		}
	}

	return s
}

// ResolvePath resolves a path relative to a base directory.
func ResolvePath(input string, baseDir string, opts ...NormalizeOptions) string {
	normalized := NormalizePath(input, opts...)
	normalizedBase := NormalizePath(baseDir, opts...)
	if filepath.IsAbs(normalized) {
		return filepath.Clean(normalized)
	}
	return filepath.Clean(filepath.Join(normalizedBase, normalized))
}

// GetCwdRelativePath returns the path relative to cwd, or nil if it escapes cwd.
func GetCwdRelativePath(filePath string, cwd string) string {
	resolvedCwd := ResolvePath(cwd, cwd)
	resolvedPath := ResolvePath(filePath, resolvedCwd)
	rel, err := filepath.Rel(resolvedCwd, resolvedPath)
	if err != nil {
		return ""
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return ""
	}
	if rel == "." {
		return "."
	}
	return rel
}

// CanonicalizePath returns the real path following symlinks.
// Falls back to the raw path if resolution fails.
func CanonicalizePath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	return resolved
}
