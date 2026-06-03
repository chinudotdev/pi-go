// Package jsonutil provides JSON utilities.
package jsonutil

import (
	"regexp"
	"strings"
)

// stripCommentsRegex matches either a JSON string literal (to preserve) or
// a // comment (to remove). The order matters: strings are matched first.
var stripCommentsRe = regexp.MustCompile(`"(?:\\.|[^"\\])*"|//[^\n]*`)

// stripTrailingCommasRegex matches a JSON string literal (preserve) or a
// trailing comma before } or ] (remove).
var stripTrailingCommasRe = regexp.MustCompile(`"(?:\\.|[^"\\])*"|,(\s*[}\]])`)

// StripComments removes // line comments and trailing commas from JSON,
// leaving string literals untouched.
func StripComments(input string) string {
	// Remove // comments (but not inside strings)
	result := stripCommentsRe.ReplaceAllStringFunc(input, func(m string) string {
		if strings.HasPrefix(m, `"`) {
			return m // It's a string literal, keep it
		}
		return "" // It's a comment, remove it
	})

	// Remove trailing commas before } or ]
	result = stripTrailingCommasRe.ReplaceAllStringFunc(result, func(m string) string {
		if strings.HasPrefix(m, `"`) {
			return m // It's a string literal, keep it
		}
		// The capture group is the closing brace/bracket
		return stripTrailingCommasRe.ReplaceAllString(m, "$1")
	})

	return result
}
