package agent

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
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

// ============================================================================
// Coercion
// ============================================================================

func coerceWithSchema(value any, schema map[string]any) any {
	// allOf: apply each sub-schema coercion in order
	if allOf, ok := sliceField(schema, "allOf"); ok {
		for _, sub := range allOf {
			if m, ok := sub.(map[string]any); ok {
				value = coerceWithSchema(value, m)
			}
		}
	}

	// anyOf/oneOf: try each sub-schema, pick first that validates
	for _, key := range []string{"anyOf", "oneOf"} {
		if subs, ok := sliceField(schema, key); ok {
			value = coerceWithUnion(value, subs)
		}
	}

	// Get schema type(s)
	schemaTypes := getStringSlice(schema, "type")

	// If the value already matches one of the union types, skip coercion
	if len(schemaTypes) > 1 {
		matched := false
		for _, t := range schemaTypes {
			if matchesJSONType(value, t) {
				matched = true
				break
			}
		}
		if matched {
			return value
		}
	}

	// Primitive coercion
	if len(schemaTypes) > 0 {
		for _, t := range schemaTypes {
			candidate := coercePrimitive(value, t)
			if isDifferent(candidate, value) {
				value = candidate
				break
			}
		}
	}

	// Object coercion
	if hasType(schemaTypes, "object") {
		if m, ok := value.(map[string]any); ok {
			coerceObject(m, schema)
		}
	}

	// Array coercion
	if hasType(schemaTypes, "array") {
		if arr, ok := value.([]any); ok {
			coerceArray(arr, schema)
		}
	}

	return value
}

func coercePrimitive(value any, targetType string) any {
	switch targetType {
	case "number":
		return coerceNumber(value)
	case "integer":
		return coerceInteger(value)
	case "boolean":
		return coerceBool(value)
	case "string":
		return coerceString(value)
	case "null":
		if value == "" || value == float64(0) || value == false {
			return nil
		}
	}
	return value
}

func coerceNumber(value any) any {
	switch v := value.(type) {
	case nil:
		return float64(0)
	case string:
		if strings.TrimSpace(v) == "" {
			return value
		}
		// Try to parse as number
		var f float64
		if err := json.Unmarshal([]byte(v), &f); err == nil && !math.IsInf(f, 0) && !math.IsNaN(f) {
			return f
		}
	case bool:
		if v {
			return float64(1)
		}
		return float64(0)
	}
	return value
}

func coerceInteger(value any) any {
	switch v := value.(type) {
	case nil:
		return float64(0)
	case string:
		if strings.TrimSpace(v) == "" {
			return value
		}
		var f float64
		if err := json.Unmarshal([]byte(v), &f); err == nil && f == math.Trunc(f) {
			return f
		}
	case bool:
		if v {
			return float64(1)
		}
		return float64(0)
	}
	return value
}

func coerceBool(value any) any {
	switch v := value.(type) {
	case nil:
		return false
	case string:
		if v == "true" {
			return true
		}
		if v == "false" {
			return false
		}
	case float64:
		if v == 1 {
			return true
		}
		if v == 0 {
			return false
		}
	}
	return value
}

func coerceString(value any) any {
	switch v := value.(type) {
	case nil:
		return ""
	case float64:
		return formatNumber(v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	}
	return value
}

func coerceObject(m map[string]any, schema map[string]any) {
	props, _ := schema["properties"].(map[string]any)
	definedKeys := make(map[string]bool, len(props))
	for k := range props {
		definedKeys[k] = true
	}

	// Coerce defined properties
	for key, propSchema := range props {
		val, ok := m[key]
		if !ok {
			continue
		}
		if ps, ok := propSchema.(map[string]any); ok {
			m[key] = coerceWithSchema(val, ps)
		}
	}

	// Coerce additional properties
	if ap, ok := schema["additionalProperties"]; ok {
		if apSchema, ok := ap.(map[string]any); ok {
			for key, val := range m {
				if definedKeys[key] {
					continue
				}
				m[key] = coerceWithSchema(val, apSchema)
			}
		}
	}
}

func coerceArray(arr []any, schema map[string]any) {
	items := schema["items"]
	switch it := items.(type) {
	case []any:
		// Tuple validation: items is an array of schemas
		for i := 0; i < len(arr) && i < len(it); i++ {
			if schema, ok := it[i].(map[string]any); ok {
				arr[i] = coerceWithSchema(arr[i], schema)
			}
		}
	case map[string]any:
		// All items match same schema
		for i := range arr {
			arr[i] = coerceWithSchema(arr[i], it)
		}
	}
}

func coerceWithUnion(value any, subs []any) any {
	for _, sub := range subs {
		schema, ok := sub.(map[string]any)
		if !ok {
			continue
		}
		candidate := deepClone(value)
		coerced := coerceWithSchema(candidate, schema)
		if len(validateAgainstSchema(candidate, schema, "")) == 0 {
			return coerced
		}
	}
	return value
}

// ============================================================================
// Validation
// ============================================================================

func validateAgainstSchema(value any, schema map[string]any, pathPrefix string) []FieldError {
	var errs []FieldError

	// allOf: must match all
	if allOf, ok := sliceField(schema, "allOf"); ok {
		for _, sub := range allOf {
			if m, ok := sub.(map[string]any); ok {
				errs = append(errs, validateAgainstSchema(value, m, pathPrefix)...)
			}
		}
	}

	// anyOf: must match at least one
	if anyOf, ok := sliceField(schema, "anyOf"); ok {
		matched := false
		for _, sub := range anyOf {
			if m, ok := sub.(map[string]any); ok {
				if len(validateAgainstSchema(value, m, pathPrefix)) == 0 {
					matched = true
					break
				}
			}
		}
		if !matched {
			errs = append(errs, FieldError{Path: pathPrefix, Message: "must match at least one anyOf schema"})
		}
	}

	// oneOf: must match exactly one
	if oneOf, ok := sliceField(schema, "oneOf"); ok {
		matches := 0
		for _, sub := range oneOf {
			if m, ok := sub.(map[string]any); ok {
				if len(validateAgainstSchema(value, m, pathPrefix)) == 0 {
					matches++
				}
			}
		}
		if matches != 1 {
			errs = append(errs, FieldError{Path: pathPrefix, Message: fmt.Sprintf("must match exactly one oneOf schema (matched %d)", matches)})
		}
	}

	// type check
	if t, ok := schema["type"]; ok {
		errs = append(errs, validateType(value, t, pathPrefix)...)
	}

	// enum
	if e, ok := schema["enum"]; ok {
		errs = append(errs, validateEnum(value, e, pathPrefix)...)
	}

	// Object-specific checks
	if m, ok := value.(map[string]any); ok {
		errs = append(errs, validateObject(m, schema, pathPrefix)...)
	}

	// Array-specific checks
	if arr, ok := value.([]any); ok {
		errs = append(errs, validateArray(arr, schema, pathPrefix)...)
	}

	// Number checks
	if f, ok := toFloat(value); ok {
		errs = append(errs, validateNumber(f, schema, pathPrefix)...)
	}

	// String checks
	if s, ok := value.(string); ok {
		errs = append(errs, validateString(s, schema, pathPrefix)...)
	}

	return errs
}

func validateType(value any, typeSpec any, path string) []FieldError {
	switch t := typeSpec.(type) {
	case string:
		if !matchesJSONType(value, t) {
			return []FieldError{{Path: path, Message: fmt.Sprintf("expected type %s, got %s", t, jsonTypeOf(value))}}
		}
	case []any:
		for _, s := range t {
			if str, ok := s.(string); ok && matchesJSONType(value, str) {
				return nil
			}
		}
		names := make([]string, len(t))
		for i, s := range t {
			names[i] = fmt.Sprint(s)
		}
		return []FieldError{{Path: path, Message: fmt.Sprintf("expected one of types [%s], got %s", strings.Join(names, ", "), jsonTypeOf(value))}}
	}
	return nil
}

func validateEnum(value any, enumSpec any, path string) []FieldError {
	vals, ok := enumSpec.([]any)
	if !ok {
		return nil
	}
	for _, v := range vals {
		if reflect.DeepEqual(value, v) {
			return nil
		}
	}
	return []FieldError{{Path: path, Message: fmt.Sprintf("value must be one of %v", vals)}}
}

func validateObject(m map[string]any, schema map[string]any, pathPrefix string) []FieldError {
	var errs []FieldError

	// required
	if req, ok := sliceField(schema, "required"); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				if _, exists := m[s]; !exists {
					p := s
					if pathPrefix != "" {
						p = pathPrefix + "." + s
					}
					errs = append(errs, FieldError{Path: p, Message: "required property missing"})
				}
			}
		}
	}

	// properties
	if props, ok := schema["properties"].(map[string]any); ok {
		for key, propSchema := range props {
			val, exists := m[key]
			if !exists {
				continue
			}
			if ps, ok := propSchema.(map[string]any); ok {
				p := key
				if pathPrefix != "" {
					p = pathPrefix + "." + key
				}
				errs = append(errs, validateAgainstSchema(val, ps, p)...)
			}
		}
	}

	// additionalProperties
	if ap, ok := schema["additionalProperties"]; ok {
		definedKeys := make(map[string]bool)
		if props, ok := schema["properties"].(map[string]any); ok {
			for k := range props {
				definedKeys[k] = true
			}
		}
		switch apVal := ap.(type) {
		case bool:
			if !apVal {
				for k := range m {
					if !definedKeys[k] {
						p := k
						if pathPrefix != "" {
							p = pathPrefix + "." + k
						}
						errs = append(errs, FieldError{Path: p, Message: "additional property not allowed"})
					}
				}
			}
		case map[string]any:
			for k, v := range m {
				if definedKeys[k] {
					continue
				}
				p := k
				if pathPrefix != "" {
					p = pathPrefix + "." + k
				}
				errs = append(errs, validateAgainstSchema(v, apVal, p)...)
			}
		}
	}

	return errs
}

func validateArray(arr []any, schema map[string]any, pathPrefix string) []FieldError {
	var errs []FieldError

	// items
	if items, ok := schema["items"]; ok {
		switch it := items.(type) {
		case map[string]any:
			for i, v := range arr {
				p := fmt.Sprintf("%s[%d]", pathPrefix, i)
				errs = append(errs, validateAgainstSchema(v, it, p)...)
			}
		case []any:
			for i := 0; i < len(arr) && i < len(it); i++ {
				if subSchema, ok := it[i].(map[string]any); ok {
					p := fmt.Sprintf("%s[%d]", pathPrefix, i)
					errs = append(errs, validateAgainstSchema(arr[i], subSchema, p)...)
				}
			}
		}
	}

	// minItems
	if min, ok := getInt(schema, "minItems"); ok && len(arr) < min {
		errs = append(errs, FieldError{Path: pathPrefix, Message: fmt.Sprintf("array must have at least %d items, got %d", min, len(arr))})
	}

	// maxItems
	if max, ok := getInt(schema, "maxItems"); ok && len(arr) > max {
		errs = append(errs, FieldError{Path: pathPrefix, Message: fmt.Sprintf("array must have at most %d items, got %d", max, len(arr))})
	}

	return errs
}

func validateNumber(f float64, schema map[string]any, path string) []FieldError {
	var errs []FieldError

	if min, ok := getFloat(schema, "minimum"); ok && f < min {
		errs = append(errs, FieldError{Path: path, Message: fmt.Sprintf("must be >= %v, got %v", min, f)})
	}
	if max, ok := getFloat(schema, "maximum"); ok && f > max {
		errs = append(errs, FieldError{Path: path, Message: fmt.Sprintf("must be <= %v, got %v", max, f)})
	}
	if emin, ok := getFloat(schema, "exclusiveMinimum"); ok && f <= emin {
		errs = append(errs, FieldError{Path: path, Message: fmt.Sprintf("must be > %v, got %v", emin, f)})
	}
	if emax, ok := getFloat(schema, "exclusiveMaximum"); ok && f >= emax {
		errs = append(errs, FieldError{Path: path, Message: fmt.Sprintf("must be < %v, got %v", emax, f)})
	}

	return errs
}

func validateString(s string, schema map[string]any, path string) []FieldError {
	var errs []FieldError

	if min, ok := getInt(schema, "minLength"); ok && len(s) < min {
		errs = append(errs, FieldError{Path: path, Message: fmt.Sprintf("string length must be >= %d, got %d", min, len(s))})
	}
	if max, ok := getInt(schema, "maxLength"); ok && len(s) > max {
		errs = append(errs, FieldError{Path: path, Message: fmt.Sprintf("string length must be <= %d, got %d", max, len(s))})
	}

	return errs
}

// ============================================================================
// Type helpers
// ============================================================================

func matchesJSONType(value any, typeName string) bool {
	switch typeName {
	case "number":
		_, ok := toFloat(value)
		return ok
	case "integer":
		f, ok := toFloat(value)
		return ok && f == math.Trunc(f)
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "null":
		return value == nil
	case "array":
		_, ok := value.([]any)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	}
	return false
}

func jsonTypeOf(value any) string {
	if value == nil {
		return "null"
	}
	switch v := value.(type) {
	case bool:
		return "boolean"
	case float64:
		if v == math.Trunc(v) {
			return "integer"
		}
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return fmt.Sprintf("%T", value)
}

func toFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	}
	return 0, false
}

func formatNumber(f float64) string {
	if f == math.Trunc(f) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%g", f)
}

// ============================================================================
// Schema field helpers
// ============================================================================

func sliceField(schema map[string]any, key string) ([]any, bool) {
	v, ok := schema[key]
	if !ok {
		return nil, false
	}
	s, ok := v.([]any)
	return s, ok
}

func getStringSlice(schema map[string]any, key string) []string {
	v, ok := schema[key]
	if !ok {
		return nil
	}
	switch t := v.(type) {
	case string:
		return []string{t}
	case []any:
		result := make([]string, 0, len(t))
		for _, s := range t {
			if str, ok := s.(string); ok {
				result = append(result, str)
			}
		}
		return result
	}
	return nil
}

func getInt(schema map[string]any, key string) (int, bool) {
	v, ok := schema[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	}
	return 0, false
}

func getFloat(schema map[string]any, key string) (float64, bool) {
	v, ok := schema[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	}
	return 0, false
}

func hasType(types []string, t string) bool {
	for _, s := range types {
		if s == t {
			return true
		}
	}
	return false
}

// isDifferent safely compares two interface values without panicking on
// incomparable types like maps or slices.
func isDifferent(a, b any) bool {
	// Fast path for nil
	if a == nil || b == nil {
		return a != b
	}
	// Use reflect for safe comparison
	return !reflect.DeepEqual(a, b)
}

// ============================================================================
// Deep clone helpers
// ============================================================================

func deepClone(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return deepCloneMap(val)
	case []any:
		clone := make([]any, len(val))
		for i, v := range val {
			clone[i] = deepClone(v)
		}
		return clone
	default:
		return v // primitives are value-copied
	}
}

func deepCloneMap(m map[string]any) map[string]any {
	clone := make(map[string]any, len(m))
	for k, v := range m {
		clone[k] = deepClone(v)
	}
	return clone
}
