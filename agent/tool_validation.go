package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chinudotdev/pi-go/ai"
)

// ============================================================================
// ValidateToolArguments
// ============================================================================

// ValidationError describes one or more JSON Schema validation failures.
type ValidationError struct {
	ToolName string
	Errors   []FieldError
	RawArgs  map[string]any
}

// FieldError describes a single field validation failure.
type FieldError struct {
	Path    string
	Message string
}

func (e *ValidationError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "validation failed for tool %q:", e.ToolName)
	for _, fe := range e.Errors {
		fmt.Fprintf(&b, "\n  - %s: %s", fe.Path, fe.Message)
	}
	fmt.Fprintf(&b, "\n\nReceived arguments:\n")
	enc, _ := json.MarshalIndent(e.RawArgs, "", "  ")
	b.Write(enc)
	return b.String()
}

// ValidateToolArguments coerces and validates tool call arguments against the
// tool's JSON Schema parameters. It returns the coerced arguments map or a
// *ValidationError.
//
// Coercion rules follow the TypeScript SDK behavior:
//   - string "123" → number 123 (when schema type is "number"/"integer")
//   - number 1 → boolean true (when schema type is "boolean")
//   - boolean true → string "true" (when schema type is "string")
//   - null → zero value of the target type
//
// Supported JSON Schema keywords: type, properties, required, items,
// additionalProperties, enum, allOf, anyOf, oneOf.
func ValidateToolArguments(tool *Tool, args map[string]any) (map[string]any, error) {
	if args == nil {
		args = map[string]any{}
	}

	// Deep clone args so we don't mutate the original
	coerced := deepCloneMap(args)

	// Coerce values against schema
	schema := tool.Parameters
	if schema == nil {
		return coerced, nil
	}

	coercedAny := coerceWithSchema(coerced, schema)
	if m, ok := coercedAny.(map[string]any); ok {
		coerced = m
	}

	// Validate
	errs := validateAgainstSchema(coerced, schema, "")
	if len(errs) > 0 {
		return nil, &ValidationError{
			ToolName: tool.Name,
			Errors:   errs,
			RawArgs:  args,
		}
	}

	return coerced, nil
}

// ValidateToolCall finds a tool by name and validates its arguments.
func ValidateToolCall(tools []*Tool, toolCall ai.ContentBlock) (map[string]any, error) {
	var tool *Tool
	for _, t := range tools {
		if t.Name == toolCall.ToolCallName {
			tool = t
			break
		}
	}
	if tool == nil {
		return nil, fmt.Errorf("tool %q not found", toolCall.ToolCallName)
	}
	args := toolCall.ToolCallArguments
	if args == nil {
		args = map[string]any{}
	}
	return ValidateToolArguments(tool, args)
}
