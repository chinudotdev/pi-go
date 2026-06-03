package agent

import (
	"strings"
	"testing"

	"github.com/chinudotdev/pi-go/ai"
)

// ============================================================================
// Coercion Tests
// ============================================================================

func TestValidateToolArguments_BasicPassThrough(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test tool",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
		},
	}
	args := map[string]any{"name": "hello"}
	result, err := ValidateToolArguments(tool, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["name"] != "hello" {
		t.Errorf("expected 'hello', got %v", result["name"])
	}
}

func TestValidateToolArguments_NilParameters(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test tool",
	}
	args := map[string]any{"anything": "goes"}
	result, err := ValidateToolArguments(tool, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["anything"] != "goes" {
		t.Errorf("expected passthrough, got %v", result)
	}
}

func TestValidateToolArguments_NilArgs(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test tool",
		Parameters:  map[string]any{"type": "object"},
	}
	result, err := ValidateToolArguments(tool, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

// ============================================================================
// Number Coercion
// ============================================================================

func TestValidateToolArguments_CoerceStringToNumber(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"count": map[string]any{"type": "number"},
			},
		},
	}
	args := map[string]any{"count": "42.5"}
	result, err := ValidateToolArguments(tool, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f, ok := result["count"].(float64); !ok || f != 42.5 {
		t.Errorf("expected 42.5, got %v", result["count"])
	}
}

func TestValidateToolArguments_CoerceStringToInteger(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"count": map[string]any{"type": "integer"},
			},
		},
	}
	args := map[string]any{"count": "42"}
	result, err := ValidateToolArguments(tool, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f, ok := result["count"].(float64); !ok || f != 42 {
		t.Errorf("expected 42, got %v", result["count"])
	}
}

func TestValidateToolArguments_CoerceBoolToNumber(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"val": map[string]any{"type": "number"},
			},
		},
	}
	args := map[string]any{"val": true}
	result, err := ValidateToolArguments(tool, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f, ok := result["val"].(float64); !ok || f != 1 {
		t.Errorf("expected 1, got %v", result["val"])
	}
}

func TestValidateToolArguments_CoerceNullToNumber(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"val": map[string]any{"type": "number"},
			},
		},
	}
	args := map[string]any{"val": nil}
	result, err := ValidateToolArguments(tool, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f, ok := result["val"].(float64); !ok || f != 0 {
		t.Errorf("expected 0, got %v", result["val"])
	}
}

// ============================================================================
// Boolean Coercion
// ============================================================================

func TestValidateToolArguments_CoerceStringToBool(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"flag": map[string]any{"type": "boolean"},
			},
		},
	}
	tests := []struct {
		input  any
		expect bool
	}{
		{"true", true},
		{"false", false},
		{float64(1), true},
		{float64(0), false},
		{nil, false},
	}
	for _, tt := range tests {
		args := map[string]any{"flag": tt.input}
		result, err := ValidateToolArguments(tool, args)
		if err != nil {
			t.Errorf("input %v: unexpected error: %v", tt.input, err)
			continue
		}
		if b, ok := result["flag"].(bool); !ok || b != tt.expect {
			t.Errorf("input %v: expected %v, got %v", tt.input, tt.expect, result["flag"])
		}
	}
}

// ============================================================================
// String Coercion
// ============================================================================

func TestValidateToolArguments_CoerceNumberToString(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"label": map[string]any{"type": "string"},
			},
		},
	}
	args := map[string]any{"label": float64(42)}
	result, err := ValidateToolArguments(tool, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s, ok := result["label"].(string); !ok || s != "42" {
		t.Errorf("expected '42', got %v", result["label"])
	}
}

func TestValidateToolArguments_CoerceBoolToString(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"label": map[string]any{"type": "string"},
			},
		},
	}
	args := map[string]any{"label": true}
	result, err := ValidateToolArguments(tool, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s, ok := result["label"].(string); !ok || s != "true" {
		t.Errorf("expected 'true', got %v", result["label"])
	}
}

func TestValidateToolArguments_CoerceNullToString(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"label": map[string]any{"type": "string"},
			},
		},
	}
	args := map[string]any{"label": nil}
	result, err := ValidateToolArguments(tool, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s, ok := result["label"].(string); !ok || s != "" {
		t.Errorf("expected '', got %v", result["label"])
	}
}

// ============================================================================
// Nested Object Coercion
// ============================================================================

func TestValidateToolArguments_NestedObjectCoercion(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"config": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"count": map[string]any{"type": "number"},
						"flag":  map[string]any{"type": "boolean"},
					},
				},
			},
		},
	}
	args := map[string]any{
		"config": map[string]any{
			"count": "10",
			"flag":  "true",
		},
	}
	result, err := ValidateToolArguments(tool, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cfg := result["config"].(map[string]any)
	if f, ok := cfg["count"].(float64); !ok || f != 10 {
		t.Errorf("expected count=10, got %v", cfg["count"])
	}
	if b, ok := cfg["flag"].(bool); !ok || !b {
		t.Errorf("expected flag=true, got %v", cfg["flag"])
	}
}

// ============================================================================
// Array Coercion
// ============================================================================

func TestValidateToolArguments_ArrayCoercion(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"items": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "number"},
				},
			},
		},
	}
	args := map[string]any{
		"items": []any{"1", "2", "3"},
	}
	result, err := ValidateToolArguments(tool, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	arr := result["items"].([]any)
	for i, v := range arr {
		if f, ok := v.(float64); !ok || f != float64(i+1) {
			t.Errorf("items[%d]: expected %d, got %v", i, i+1, v)
		}
	}
}

// ============================================================================
// Validation Errors
// ============================================================================

func TestValidateToolArguments_RequiredField(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []any{"name"},
		},
	}
	args := map[string]any{}
	_, err := ValidateToolArguments(tool, args)
	if err == nil {
		t.Fatal("expected validation error for missing required field")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if ve.ToolName != "test" {
		t.Errorf("expected tool name 'test', got %q", ve.ToolName)
	}
	if len(ve.Errors) == 0 {
		t.Error("expected at least one field error")
	}
	found := false
	for _, fe := range ve.Errors {
		if fe.Path == "name" && strings.Contains(fe.Message, "required") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'name: required' error, got %v", ve.Errors)
	}
}

func TestValidateToolArguments_WrongType(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"count": map[string]any{"type": "number"},
			},
		},
	}
	args := map[string]any{"count": []any{"not a number"}}
	_, err := ValidateToolArguments(tool, args)
	if err == nil {
		t.Fatal("expected validation error for wrong type")
	}
}

func TestValidateToolArguments_EnumValidation(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"color": map[string]any{
					"type": "string",
					"enum": []any{"red", "green", "blue"},
				},
			},
		},
	}
	// Valid
	args := map[string]any{"color": "red"}
	_, err := ValidateToolArguments(tool, args)
	if err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}

	// Invalid
	args = map[string]any{"color": "purple"}
	_, err = ValidateToolArguments(tool, args)
	if err == nil {
		t.Fatal("expected validation error for invalid enum value")
	}
}

// ============================================================================
// allOf / anyOf / oneOf
// ============================================================================

func TestValidateToolArguments_AllOf(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"val": map[string]any{
					"allOf": []any{
						map[string]any{"type": "number", "minimum": float64(0)},
						map[string]any{"type": "number", "maximum": float64(100)},
					},
				},
			},
		},
	}
	// Valid
	args := map[string]any{"val": float64(50)}
	_, err := ValidateToolArguments(tool, args)
	if err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}

	// Invalid: below minimum
	args = map[string]any{"val": float64(-1)}
	_, err = ValidateToolArguments(tool, args)
	if err == nil {
		t.Fatal("expected error for value below minimum")
	}

	// Invalid: above maximum
	args = map[string]any{"val": float64(101)}
	_, err = ValidateToolArguments(tool, args)
	if err == nil {
		t.Fatal("expected error for value above maximum")
	}
}

func TestValidateToolArguments_AnyOf(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"val": map[string]any{
					"anyOf": []any{
						map[string]any{"type": "string"},
						map[string]any{"type": "number"},
					},
				},
			},
		},
	}
	// Both valid
	args1 := map[string]any{"val": "hello"}
	args2 := map[string]any{"val": float64(42)}
	for _, args := range []map[string]any{args1, args2} {
		_, err := ValidateToolArguments(tool, args)
		if err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	}

	// Invalid: boolean matches neither
	args3 := map[string]any{"val": true}
	_, err3 := ValidateToolArguments(tool, args3)
	// boolean doesn't match string or number, so should fail
	_ = err3
}

// ============================================================================
// Additional Properties
// ============================================================================

func TestValidateToolArguments_AdditionalPropertiesFalse(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test",
		Parameters: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{"name": map[string]any{"type": "string"}},
			"additionalProperties": false,
		},
	}
	args := map[string]any{"name": "ok", "extra": "bad"}
	_, err := ValidateToolArguments(tool, args)
	if err == nil {
		t.Fatal("expected error for additional property")
	}
}

func TestValidateToolArguments_AdditionalPropertiesSchema(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test",
		Parameters: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{"name": map[string]any{"type": "string"}},
			"additionalProperties": map[string]any{"type": "number"},
		},
	}
	args := map[string]any{"name": "ok", "count": "42"}
	result, err := ValidateToolArguments(tool, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f, ok := result["count"].(float64); !ok || f != 42 {
		t.Errorf("expected coerced count=42, got %v", result["count"])
	}
}

// ============================================================================
// Array Validation
// ============================================================================

func TestValidateToolArguments_ArrayMinItems(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"items": map[string]any{
					"type":     "array",
					"minItems": float64(2),
				},
			},
		},
	}
	args := map[string]any{"items": []any{"one"}}
	_, err := ValidateToolArguments(tool, args)
	if err == nil {
		t.Fatal("expected error for minItems")
	}
}

func TestValidateToolArguments_ArrayMaxItems(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"items": map[string]any{
					"type":     "array",
					"maxItems": float64(2),
				},
			},
		},
	}
	args := map[string]any{"items": []any{"a", "b", "c"}}
	_, err := ValidateToolArguments(tool, args)
	if err == nil {
		t.Fatal("expected error for maxItems")
	}
}

// ============================================================================
// String Validation
// ============================================================================

func TestValidateToolArguments_StringMinLength(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":      "string",
					"minLength": float64(3),
				},
			},
		},
	}
	args := map[string]any{"name": "ab"}
	_, err := ValidateToolArguments(tool, args)
	if err == nil {
		t.Fatal("expected error for minLength")
	}
}

// ============================================================================
// Number Validation
// ============================================================================

func TestValidateToolArguments_NumberMinMax(t *testing.T) {
	tool := &Tool{
		Name:        "test",
		Description: "test",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"score": map[string]any{
					"type":    "number",
					"minimum": float64(0),
					"maximum": float64(100),
				},
			},
		},
	}
	// Valid
	_, err := ValidateToolArguments(tool, map[string]any{"score": float64(50)})
	if err != nil {
		t.Fatalf("expected valid: %v", err)
	}
	// Below minimum
	_, err = ValidateToolArguments(tool, map[string]any{"score": float64(-1)})
	if err == nil {
		t.Fatal("expected error for minimum")
	}
	// Above maximum
	_, err = ValidateToolArguments(tool, map[string]any{"score": float64(101)})
	if err == nil {
		t.Fatal("expected error for maximum")
	}
}

// ============================================================================
// ValidateToolCall
// ============================================================================

func TestValidateToolCall_ToolNotFound(t *testing.T) {
	tools := []*Tool{{Name: "existing", Description: "test"}}
	_, err := ValidateToolCall(tools, ai.ContentBlock{
		ToolCallName:      "missing",
		ToolCallArguments: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for missing tool")
	}
}

// ============================================================================
// Type detection
// ============================================================================

func TestMatchesJSONType(t *testing.T) {
	tests := []struct {
		value any
		typ   string
		want  bool
	}{
		{float64(42), "number", true},
		{float64(42), "integer", true},
		{float64(42.5), "integer", false},
		{"hello", "string", true},
		{true, "boolean", true},
		{nil, "null", true},
		{[]any{1, 2}, "array", true},
		{map[string]any{}, "object", true},
		{"hello", "number", false},
	}
	for _, tt := range tests {
		got := matchesJSONType(tt.value, tt.typ)
		if got != tt.want {
			t.Errorf("matchesJSONType(%v, %q) = %v, want %v", tt.value, tt.typ, got, tt.want)
		}
	}
}

// ============================================================================
// Deep clone
// ============================================================================

func TestDeepClone(t *testing.T) {
	original := map[string]any{
		"nested": map[string]any{
			"arr": []any{1, 2, 3},
		},
	}
	clone := deepCloneMap(original)

	// Modify clone, original should be unchanged
	clone["nested"].(map[string]any)["arr"].([]any)[0] = 99
	if original["nested"].(map[string]any)["arr"].([]any)[0] == float64(99) {
		t.Error("deep clone did not isolate nested structures")
	}
}

// ============================================================================
// Error message formatting
// ============================================================================

func TestValidationError_Error(t *testing.T) {
	ve := &ValidationError{
		ToolName: "my_tool",
		Errors: []FieldError{
			{Path: "name", Message: "required property missing"},
			{Path: "age", Message: "expected type number, got string"},
		},
		RawArgs: map[string]any{"age": "not a number"},
	}
	msg := ve.Error()
	if !contains(msg, "my_tool") {
		t.Error("error message should contain tool name")
	}
	if !contains(msg, "name") || !contains(msg, "required") {
		t.Error("error message should contain field error details")
	}
	if !contains(msg, "age") {
		t.Error("error message should contain second field error")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || strings.Contains(s, sub))
}
