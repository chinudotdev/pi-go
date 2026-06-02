package harness

import (
	"errors"
	"testing"
)

func TestOkResult(t *testing.T) {
	r := OkResult(42)
	if !r.OK {
		t.Error("expected OK=true")
	}
	if r.Value != 42 {
		t.Errorf("expected 42, got %d", r.Value)
	}
	if r.Err != nil {
		t.Errorf("expected nil err, got %v", r.Err)
	}
}

func TestErrResult(t *testing.T) {
	r := ErrResult[int](errors.New("fail"))
	if r.OK {
		t.Error("expected OK=false")
	}
	if r.Err == nil {
		t.Error("expected non-nil err")
	}
}

func TestGetOrThrow_Success(t *testing.T) {
	r := OkResult("hello")
	v := GetOrThrow(r)
	if v != "hello" {
		t.Errorf("expected 'hello', got %q", v)
	}
}

func TestGetOrThrow_Failure(t *testing.T) {
	defer func() {
		if rec := recover(); rec == nil {
			t.Error("expected panic")
		}
	}()
	r := ErrResult[string](errors.New("boom"))
	GetOrThrow(r)
}

func TestGetOrZero(t *testing.T) {
	r1 := OkResult(99)
	if v := GetOrZero(r1); v != 99 {
		t.Errorf("expected 99, got %d", v)
	}

	r2 := ErrResult[int](errors.New("x"))
	if v := GetOrZero(r2); v != 0 {
		t.Errorf("expected 0, got %d", v)
	}
}

// ============================================================================
// Typed errors
// ============================================================================

func TestFileError(t *testing.T) {
	e := NewFileError(FileErrorNotFound, "not found", "/tmp/test.txt", nil)
	if e.Code != FileErrorNotFound {
		t.Errorf("expected %s, got %s", FileErrorNotFound, e.Code)
	}
	if e.Path != "/tmp/test.txt" {
		t.Errorf("expected path /tmp/test.txt, got %s", e.Path)
	}
	msg := e.Error()
	if msg == "" {
		t.Error("expected non-empty error message")
	}
}

func TestExecutionError(t *testing.T) {
	e := NewExecutionError(ExecErrorTimeout, "timed out", nil)
	if e.Code != ExecErrorTimeout {
		t.Errorf("expected %s, got %s", ExecErrorTimeout, e.Code)
	}
}

func TestCompactionError(t *testing.T) {
	e := NewCompactionError(CompactionErrorSummarizationFailed, "failed", nil)
	if e.Code != CompactionErrorSummarizationFailed {
		t.Errorf("expected %s, got %s", CompactionErrorSummarizationFailed, e.Code)
	}
}

func TestSessionError(t *testing.T) {
	e := NewSessionError(SessionErrorNotFound, "not found", nil)
	if e.Code != SessionErrorNotFound {
		t.Errorf("expected %s, got %s", SessionErrorNotFound, e.Code)
	}
}

func TestAgentHarnessError(t *testing.T) {
	e := NewAgentHarnessError(HarnessErrorBusy, "busy", nil)
	if e.Code != HarnessErrorBusy {
		t.Errorf("expected %s, got %s", HarnessErrorBusy, e.Code)
	}
}

func TestNormalizeHarnessError(t *testing.T) {
	// Already an AgentHarnessError
	he := NewAgentHarnessError(HarnessErrorAuth, "auth fail", nil)
	result := NormalizeHarnessError(he, HarnessErrorUnknown)
	if result.Code != HarnessErrorAuth {
		t.Errorf("expected %s, got %s", HarnessErrorAuth, result.Code)
	}

	// SessionError wraps into session code
	se := NewSessionError(SessionErrorStorage, "disk full", nil)
	result = NormalizeHarnessError(se, HarnessErrorUnknown)
	if result.Code != HarnessErrorSession {
		t.Errorf("expected %s, got %s", HarnessErrorSession, result.Code)
	}

	// Unknown error gets fallback code
	result = NormalizeHarnessError(errors.New("mystery"), HarnessErrorHook)
	if result.Code != HarnessErrorHook {
		t.Errorf("expected %s, got %s", HarnessErrorHook, result.Code)
	}
}

func TestToError(t *testing.T) {
	if e := ToError(errors.New("x")); e.Error() != "x" {
		t.Errorf("expected 'x', got %q", e.Error())
	}
	if e := ToError("string err"); e == nil {
		t.Error("expected non-nil error from string")
	}
	if e := ToError(42); e == nil {
		t.Error("expected non-nil error from int")
	}
}

// ============================================================================
// Stream options
// ============================================================================

func TestHarnessStreamOptions_Clone(t *testing.T) {
	orig := HarnessStreamOptions{
		Headers: map[string]string{"a": "1", "b": "2"},
		Metadata: map[string]any{"key": "val"},
	}
	clone := orig.Clone()

	clone.Headers["a"] = "modified"
	if orig.Headers["a"] != "1" {
		t.Error("clone should not affect original headers")
	}
	clone.Metadata["key"] = "changed"
	if orig.Metadata["key"] != "val" {
		t.Error("clone should not affect original metadata")
	}
}

func TestApplyStreamOptionsPatch(t *testing.T) {
	base := HarnessStreamOptions{
		Headers: map[string]string{"a": "1", "b": "2"},
	}

	timeout := 5000
	patch := &HarnessStreamOptionsPatch{
		TimeoutMs: &timeout,
		Headers: map[string]*string{
			"b": nil, // delete b
			"c": strPtr("3"),
		},
	}

	result := ApplyStreamOptionsPatch(base, patch)

	if result.Headers["a"] != "1" {
		t.Error("header 'a' should be preserved")
	}
	if _, ok := result.Headers["b"]; ok {
		t.Error("header 'b' should be deleted")
	}
	if result.Headers["c"] != "3" {
		t.Error("header 'c' should be added")
	}
	if result.TimeoutMs == nil || *result.TimeoutMs != 5000 {
		t.Error("timeout should be 5000")
	}
}

func TestApplyStreamOptionsPatch_Nil(t *testing.T) {
	base := HarnessStreamOptions{
		Headers: map[string]string{"a": "1"},
	}
	result := ApplyStreamOptionsPatch(base, nil)
	if result.Headers["a"] != "1" {
		t.Error("nil patch should not modify base")
	}
}

func TestMergeHeaders(t *testing.T) {
	merged := MergeHeaders(
		map[string]string{"a": "1"},
		nil,
		map[string]string{"b": "2", "a": "override"},
	)
	if merged["a"] != "override" {
		t.Errorf("expected 'override', got %q", merged["a"])
	}
	if merged["b"] != "2" {
		t.Errorf("expected '2', got %q", merged["b"])
	}
}

// ============================================================================
// DefaultCompactionSettings
// ============================================================================

func TestDefaultCompactionSettings(t *testing.T) {
	s := DefaultCompactionSettings()
	if !s.Enabled {
		t.Error("expected enabled=true")
	}
	if s.ReserveTokens <= 0 {
		t.Error("expected positive reserve tokens")
	}
	if s.KeepRecentTokens <= 0 {
		t.Error("expected positive keep recent tokens")
	}
}

// ============================================================================
// Helpers
// ============================================================================

func strPtr(s string) *string { return &s }
