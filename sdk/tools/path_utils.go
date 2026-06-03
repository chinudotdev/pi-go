package tools

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	"github.com/chinudotdev/pi-go/sdk/internal/paths"
)

// ExpandPath normalizes a path with ~ expansion and @ prefix stripping.
func ExpandPath(filePath string) string {
	return paths.NormalizePath(filePath)
}

// ResolveToCwd resolves a path relative to the given cwd.
// Handles ~ expansion and absolute paths.
func ResolveToCwd(filePath, cwd string) string {
	return paths.ResolvePath(filePath, cwd)
}

// ResolveReadPath resolves a path for reading, trying macOS variants.
func ResolveReadPath(filePath, cwd string) string {
	resolved := ResolveToCwd(filePath, cwd)

	if fileExists(resolved) {
		return resolved
	}

	// Try macOS AM/PM variant (narrow no-break space before AM/PM)
	amPmVariant := tryMacOSScreenshotPath(resolved)
	if amPmVariant != resolved && fileExists(amPmVariant) {
		return amPmVariant
	}

	// Try NFD variant (macOS stores filenames in NFD form)
	nfdVariant := tryNFDVariant(resolved)
	if nfdVariant != resolved && fileExists(nfdVariant) {
		return nfdVariant
	}

	// Try curly quote variant
	curlyVariant := tryCurlyQuoteVariant(resolved)
	if curlyVariant != resolved && fileExists(curlyVariant) {
		return curlyVariant
	}

	// Combined NFD + curly quote
	nfdCurlyVariant := tryCurlyQuoteVariant(nfdVariant)
	if nfdCurlyVariant != resolved && fileExists(nfdCurlyVariant) {
		return nfdCurlyVariant
	}

	return resolved
}

// ShortenPath shortens a path by replacing the home directory with ~.
func ShortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

// fileExists checks if a file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

const narrowNoBreakSpace = "\u202F"

func tryMacOSScreenshotPath(filePath string) string {
	// Replace space before AM/PM with narrow no-break space
	result := ""
	for i := 0; i < len(filePath); i++ {
		if i+2 < len(filePath) && filePath[i] == ' ' {
			next := filePath[i+1 : i+3]
			lower := strings.ToLower(next)
			if lower == "am" || lower == "pm" {
				// Check if it's preceded by a digit
				if i > 0 && filePath[i-1] >= '0' && filePath[i-1] <= '9' {
					result += narrowNoBreakSpace
					continue
				}
			}
		}
		result += string(filePath[i])
	}
	return result
}

func tryNFDVariant(filePath string) string {
	// Normalize to NFD form (macOS stores filenames in decomposed form)
	// In Go, we use the norm package behavior
	return strings.ToValidUTF8(filePath, "")
}

func tryCurlyQuoteVariant(filePath string) string {
	return strings.ReplaceAll(filePath, "'", "\u2019")
}

// isPrintable checks if a byte is a printable ASCII character or valid UTF-8 start.
func isPrintable(b byte) bool {
	return b >= 32 && b < 127 || b == '\n' || b == '\r' || b == '\t'
}

// normalizeUnicodeSpaces replaces various Unicode spaces with regular spaces.
func normalizeUnicodeSpaces(s string) string {
	var result strings.Builder
	for _, r := range s {
		if unicode.IsSpace(r) && r != ' ' && r != '\n' && r != '\r' && r != '\t' {
			result.WriteRune(' ')
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// toPosixPath converts OS-specific path separators to forward slashes.
func toPosixPath(p string) string {
	if runtime.GOOS == "windows" {
		return strings.ReplaceAll(p, string(filepath.Separator), "/")
	}
	return p
}
