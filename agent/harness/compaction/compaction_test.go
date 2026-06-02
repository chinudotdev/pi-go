package compaction

import (
	"testing"

	"github.com/chinudotdev/pi-go/agent/harness"
	"github.com/chinudotdev/pi-go/ai"
)

// ============================================================================
// Token estimation
// ============================================================================

func TestEstimateTokens_UserString(t *testing.T) {
	msg := ai.Message{Role: "user", Content: "hello world"}
	tokens := EstimateTokens(msg)
	if tokens == 0 {
		t.Error("expected non-zero tokens")
	}
	// "hello world" = 11 chars / 4 = 3 tokens
	if tokens != 3 {
		t.Errorf("expected 3 tokens, got %d", tokens)
	}
}

func TestEstimateTokens_Assistant(t *testing.T) {
	msg := ai.Message{
		Role: "assistant",
		AssistantContent: []ai.ContentBlock{
			{Type: "text", Text: "hello"},
			{Type: "thinking", Thinking: "hmmm"},
		},
	}
	tokens := EstimateTokens(msg)
	// "hello" (5) + "hmmm" (4) = 9 chars / 4 = 3 tokens
	if tokens != 3 {
		t.Errorf("expected 3 tokens, got %d", tokens)
	}
}

func TestEstimateTokens_ToolResult(t *testing.T) {
	msg := ai.Message{
		Role: "toolResult",
		ToolResultContent: []ai.ContentBlock{
			{Type: "text", Text: "file contents here"},
		},
	}
	tokens := EstimateTokens(msg)
	if tokens == 0 {
		t.Error("expected non-zero tokens")
	}
}

func TestEstimateTokens_Empty(t *testing.T) {
	msg := ai.Message{Role: "user", Content: ""}
	tokens := EstimateTokens(msg)
	if tokens != 0 {
		t.Errorf("expected 0 tokens for empty message, got %d", tokens)
	}
}

// ============================================================================
// Context token estimation
// ============================================================================

func TestEstimateContextTokens_NoUsage(t *testing.T) {
	messages := []ai.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", AssistantContent: []ai.ContentBlock{{Type: "text", Text: "hi"}}},
	}
	est := EstimateContextTokens(messages)
	if est.Tokens == 0 {
		t.Error("expected non-zero tokens")
	}
	if est.UsageTokens != 0 {
		t.Error("expected 0 usage tokens when no usage")
	}
	if est.LastUsageIndex != nil {
		t.Error("expected nil lastUsageIndex")
	}
}

func TestEstimateContextTokens_WithUsage(t *testing.T) {
	messages := []ai.Message{
		{Role: "user", Content: "hello"},
		{
			Role:    "assistant",
			Usage:   ai.Usage{TotalTokens: 100},
			AssistantContent: []ai.ContentBlock{{Type: "text", Text: "hi"}},
		},
		{Role: "user", Content: "follow up"},
	}
	est := EstimateContextTokens(messages)
	if est.Tokens != 103 { // 100 usage + 3 trailing ("follow up" = 10 chars / 4 = 3)
		t.Errorf("expected 103 tokens, got %d", est.Tokens)
	}
	if est.UsageTokens != 100 {
		t.Errorf("expected 100 usage tokens, got %d", est.UsageTokens)
	}
	if est.LastUsageIndex == nil || *est.LastUsageIndex != 1 {
		t.Errorf("expected lastUsageIndex=1, got %v", est.LastUsageIndex)
	}
}

// ============================================================================
// ShouldCompact
// ============================================================================

func TestShouldCompact(t *testing.T) {
	settings := harness.CompactionSettings{Enabled: true, ReserveTokens: 1000}

	if ShouldCompact(5000, 8000, settings) {
		t.Error("should not compact: 5000 < 8000-1000")
	}
	if !ShouldCompact(7500, 8000, settings) {
		t.Error("should compact: 7500 > 8000-1000")
	}

	settings.Enabled = false
	if ShouldCompact(10000, 8000, settings) {
		t.Error("should not compact when disabled")
	}
}

// ============================================================================
// CalculateContextTokens
// ============================================================================

func TestCalculateContextTokens(t *testing.T) {
	tests := []struct {
		usage   ai.Usage
		expect  int
	}{
		{ai.Usage{TotalTokens: 100}, 100},
		{ai.Usage{Input: 50, Output: 30, CacheRead: 10, CacheWrite: 10}, 100},
		{ai.Usage{}, 0},
	}
	for _, tt := range tests {
		got := CalculateContextTokens(tt.usage)
		if got != tt.expect {
			t.Errorf("expected %d, got %d", tt.expect, got)
		}
	}
}

// ============================================================================
// File operations
// ============================================================================

func TestCreateFileOps(t *testing.T) {
	ops := CreateFileOps()
	if len(ops.Read) != 0 || len(ops.Written) != 0 || len(ops.Edited) != 0 {
		t.Error("expected empty file ops")
	}
}

func TestExtractFileOpsFromMessage(t *testing.T) {
	ops := CreateFileOps()
	msg := ai.Message{
		Role: "assistant",
		AssistantContent: []ai.ContentBlock{
			{
				Type:            "toolCall",
				ToolCallName:    "read",
				ToolCallArguments: map[string]any{"path": "/tmp/a.txt"},
			},
			{
				Type:            "toolCall",
				ToolCallName:    "write",
				ToolCallArguments: map[string]any{"path": "/tmp/b.txt"},
			},
			{
				Type:            "toolCall",
				ToolCallName:    "edit",
				ToolCallArguments: map[string]any{"path": "/tmp/c.txt"},
			},
		},
	}
	ExtractFileOpsFromMessage(msg, &ops)

	if !ops.Read["/tmp/a.txt"] {
		t.Error("expected /tmp/a.txt in read")
	}
	if !ops.Written["/tmp/b.txt"] {
		t.Error("expected /tmp/b.txt in written")
	}
	if !ops.Edited["/tmp/c.txt"] {
		t.Error("expected /tmp/c.txt in edited")
	}
}

func TestExtractFileOpsFromMessage_NonAssistant(t *testing.T) {
	ops := CreateFileOps()
	msg := ai.Message{Role: "user", Content: "hello"}
	ExtractFileOpsFromMessage(msg, &ops)
	if len(ops.Read) != 0 {
		t.Error("expected no file ops from user message")
	}
}

func TestComputeFileLists(t *testing.T) {
	ops := FileOperations{
		Read:    map[string]bool{"/tmp/a.txt": true, "/tmp/b.txt": true},
		Written: map[string]bool{"/tmp/b.txt": true},
		Edited:  map[string]bool{"/tmp/c.txt": true},
	}
	readFiles, modifiedFiles := ComputeFileLists(ops)

	if len(readFiles) != 1 || readFiles[0] != "/tmp/a.txt" {
		t.Errorf("expected [/tmp/a.txt], got %v", readFiles)
	}
	if len(modifiedFiles) != 2 {
		t.Errorf("expected 2 modified files, got %v", modifiedFiles)
	}
	// Should be sorted
	if modifiedFiles[0] != "/tmp/b.txt" || modifiedFiles[1] != "/tmp/c.txt" {
		t.Errorf("expected sorted modified files, got %v", modifiedFiles)
	}
}

func TestFormatFileOperations(t *testing.T) {
	result := FormatFileOperations([]string{"/a.txt"}, []string{"/b.txt"})
	if result == "" {
		t.Error("expected non-empty result")
	}
	if !contains(result, "<read-files>") || !contains(result, "<modified-files>") {
		t.Error("expected both tags in result")
	}

	empty := FormatFileOperations(nil, nil)
	if empty != "" {
		t.Error("expected empty string for empty lists")
	}
}

// ============================================================================
// Cut point finding
// ============================================================================

func TestFindCutPoint_Empty(t *testing.T) {
	result := FindCutPoint(nil, 0, 0, 1000)
	if result.FirstKeptEntryIndex != 0 {
		t.Errorf("expected 0, got %d", result.FirstKeptEntryIndex)
	}
}

func TestFindCutPoint_SingleUserMessage(t *testing.T) {
	entries := []harness.SessionTreeEntry{
		{
			SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: "1"},
			Message:              &ai.Message{Role: "user", Content: "hello"},
		},
	}
	result := FindCutPoint(entries, 0, 1, 0)
	if result.FirstKeptEntryIndex != 0 {
		t.Errorf("expected 0, got %d", result.FirstKeptEntryIndex)
	}
}

func TestFindCutPoint_MultipleMessages(t *testing.T) {
	entries := []harness.SessionTreeEntry{
		{SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: "1"}, Message: &ai.Message{Role: "user", Content: "first message that is somewhat long"}},
		{SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: "2"}, Message: &ai.Message{Role: "assistant", AssistantContent: []ai.ContentBlock{{Type: "text", Text: "response"}}}},
		{SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: "3"}, Message: &ai.Message{Role: "user", Content: "second prompt"}},
		{SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: "4"}, Message: &ai.Message{Role: "assistant", AssistantContent: []ai.ContentBlock{{Type: "text", Text: "second response"}}}},
	}

	// Keep 50 recent tokens — should keep all or most
	result := FindCutPoint(entries, 0, 4, 50)
	if result.FirstKeptEntryIndex < 0 || result.FirstKeptEntryIndex >= 4 {
		t.Errorf("expected valid cut index, got %d", result.FirstKeptEntryIndex)
	}
}

func TestFindCutPoint_WithCompaction(t *testing.T) {
	entries := []harness.SessionTreeEntry{
		{SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "compaction", ID: "c1"}, Summary: "old summary", FirstKeptEntryID: "3"},
		{SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: "3"}, Message: &ai.Message{Role: "user", Content: "kept message"}},
		{SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: "4"}, Message: &ai.Message{Role: "assistant", AssistantContent: []ai.ContentBlock{{Type: "text", Text: "response"}}}},
	}

	// Very large keep budget — should keep everything
	result := FindCutPoint(entries, 1, 3, 100000)
	if result.FirstKeptEntryIndex != 1 {
		t.Errorf("expected cut at index 1, got %d", result.FirstKeptEntryIndex)
	}
}

// ============================================================================
// PrepareCompaction
// ============================================================================

func TestPrepareCompaction_Empty(t *testing.T) {
	result, err := PrepareCompaction(nil, harness.DefaultCompactionSettings())
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Error("expected nil for empty entries")
	}
}

func TestPrepareCompaction_LastIsCompaction(t *testing.T) {
	entries := []harness.SessionTreeEntry{
		{SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "compaction", ID: "c1"}, Summary: "summary"},
	}
	result, err := PrepareCompaction(entries, harness.DefaultCompactionSettings())
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Error("expected nil when last entry is compaction")
	}
}

func TestPrepareCompaction_SingleMessage(t *testing.T) {
	entries := []harness.SessionTreeEntry{
		{SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: "1"}, Message: &ai.Message{Role: "user", Content: "hello"}},
	}
	result, err := PrepareCompaction(entries, harness.CompactionSettings{Enabled: true, ReserveTokens: 8000, KeepRecentTokens: 20000})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// With only one message, cut point should keep it (keepRecentTokens is larger than all tokens)
	if result.FirstKeptEntryID != "1" {
		t.Errorf("expected firstKeptEntryID=1, got %s", result.FirstKeptEntryID)
	}
}

func TestPrepareCompaction_NoID(t *testing.T) {
	entries := []harness.SessionTreeEntry{
		{SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: ""}, Message: &ai.Message{Role: "user", Content: "hello"}},
	}
	_, err := PrepareCompaction(entries, harness.CompactionSettings{Enabled: true, ReserveTokens: 0, KeepRecentTokens: 0})
	if err == nil {
		t.Error("expected error for entry without UUID")
	}
}

// ============================================================================
// GetLastAssistantUsage
// ============================================================================

func TestGetLastAssistantUsage(t *testing.T) {
	entries := []harness.SessionTreeEntry{
		{SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: "1"}, Message: &ai.Message{Role: "user", Content: "hi"}},
		{
			SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: "2"},
			Message: &ai.Message{Role: "assistant", Usage: ai.Usage{TotalTokens: 42}, AssistantContent: []ai.ContentBlock{{Type: "text", Text: "hello"}}},
		},
	}
	usage := GetLastAssistantUsage(entries)
	if usage == nil || usage.TotalTokens != 42 {
		t.Errorf("expected 42, got %v", usage)
	}
}

func TestGetLastAssistantUsage_Aborted(t *testing.T) {
	entries := []harness.SessionTreeEntry{
		{
			SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: "1"},
			Message: &ai.Message{Role: "assistant", StopReason: "aborted", Usage: ai.Usage{TotalTokens: 99}},
		},
	}
	usage := GetLastAssistantUsage(entries)
	if usage != nil {
		t.Error("expected nil for aborted message")
	}
}

// ============================================================================
// GetModel
// ============================================================================

func TestGetModel_FromModelChange(t *testing.T) {
	entries := []harness.SessionTreeEntry{
		{SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "model_change", ID: "mc1"}, Provider: "openai", ModelID: "gpt-4o"},
	}
	model := GetModel(entries)
	if model == nil || model.Provider != "openai" {
		t.Errorf("expected openai, got %v", model)
	}
}

func TestGetModel_FromAssistant(t *testing.T) {
	entries := []harness.SessionTreeEntry{
		{
			SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: "1"},
			Message: &ai.Message{Role: "assistant", Provider: "anthropic", Model: "claude-3", AssistantContent: []ai.ContentBlock{{Type: "text", Text: "hi"}}},
		},
	}
	model := GetModel(entries)
	if model == nil || model.Provider != "anthropic" || model.ModelID != "claude-3" {
		t.Errorf("expected anthropic/claude-3, got %v", model)
	}
}

func TestGetModel_None(t *testing.T) {
	entries := []harness.SessionTreeEntry{
		{SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: "1"}, Message: &ai.Message{Role: "user", Content: "hi"}},
	}
	model := GetModel(entries)
	if model != nil {
		t.Error("expected nil for no model")
	}
}

// ============================================================================
// FindTurnStartIndex
// ============================================================================

func TestFindTurnStartIndex(t *testing.T) {
	entries := []harness.SessionTreeEntry{
		{SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: "1"}, Message: &ai.Message{Role: "user", Content: "hello"}},
		{SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: "2"}, Message: &ai.Message{Role: "assistant", AssistantContent: []ai.ContentBlock{{Type: "text", Text: "hi"}}}},
		{SessionTreeEntryBase: harness.SessionTreeEntryBase{Type: "message", ID: "3"}, Message: &ai.Message{Role: "user", Content: "follow up"}},
	}

	// Looking for turn start from assistant message (index 2) should find user at index 2
	idx := FindTurnStartIndex(entries, 2, 0)
	if idx != 2 {
		t.Errorf("expected 2, got %d", idx)
	}

	// Looking from assistant (index 1) should find user at index 0
	idx = FindTurnStartIndex(entries, 1, 0)
	if idx != 0 {
		t.Errorf("expected 0, got %d", idx)
	}
}

// ============================================================================
// Helpers
// ============================================================================

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
