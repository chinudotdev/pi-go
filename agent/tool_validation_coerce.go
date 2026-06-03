package agent

import (
	"encoding/json"
	"math"
	"strings"
)

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
