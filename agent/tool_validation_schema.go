package agent

import (
	"fmt"
	"math"
	"reflect"
	"strings"
)

// ============================================================================
// Schema validation
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
