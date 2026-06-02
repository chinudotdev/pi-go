package harness

import (
	"testing"

	"github.com/chinudotdev/pi-go/ai"
)

func TestConvertToLlm_UserAndAssistant(t *testing.T) {
	messages := []ai.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", AssistantContent: []ai.ContentBlock{{Type: "text", Text: "hi"}}},
	}
	result := ConvertToLlm(messages)
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
	if result[0].Role != "user" {
		t.Errorf("expected user, got %q", result[0].Role)
	}
	if result[1].Role != "assistant" {
		t.Errorf("expected assistant, got %q", result[1].Role)
	}
}

func TestConvertToLlm_BranchSummary(t *testing.T) {
	messages := []ai.Message{
		NewBranchSummaryMessage("test summary", "branch1", 1000),
	}
	result := ConvertToLlm(messages)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	if result[0].Role != "user" {
		t.Errorf("expected user, got %q", result[0].Role)
	}
	content, ok := result[0].Content.(string)
	if !ok {
		t.Fatal("expected string content")
	}
	if !containsSubstring(content, BranchSummaryPrefix) {
		t.Error("expected branch summary prefix in content")
	}
}

func TestConvertToLlm_CompactionSummary(t *testing.T) {
	messages := []ai.Message{
		NewCompactionSummaryMessage("test summary", 1000, 1000),
	}
	result := ConvertToLlm(messages)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	content, ok := result[0].Content.(string)
	if !ok {
		t.Fatal("expected string content")
	}
	if !containsSubstring(content, CompactionSummaryPrefix) {
		t.Error("expected compaction summary prefix in content")
	}
}

func TestConvertToLlm_CustomMessage(t *testing.T) {
	messages := []ai.Message{
		NewCustomMessage("test_type", "custom content", true, 1000),
	}
	result := ConvertToLlm(messages)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	if result[0].Role != "user" {
		t.Errorf("expected user, got %q", result[0].Role)
	}
}

func TestConvertToLlm_BashExecution(t *testing.T) {
	messages := []ai.Message{
		NewBashExecutionMessage(BashExecutionPayload{
			Command: "ls -la",
			Output:  "file1.txt\nfile2.txt",
		}, 1000),
	}
	result := ConvertToLlm(messages)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	if result[0].Role != "user" {
		t.Errorf("expected user, got %q", result[0].Role)
	}
	content, ok := result[0].Content.(string)
	if !ok {
		t.Fatal("expected string content")
	}
	if !containsSubstring(content, "ls -la") {
		t.Error("expected command in content")
	}
}

func TestSerializeConversation(t *testing.T) {
	messages := []ai.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", AssistantContent: []ai.ContentBlock{{Type: "text", Text: "hi there"}}},
	}
	text := SerializeConversation(messages)
	if !containsSubstring(text, "[User]: hello") {
		t.Error("expected [User]: hello in output")
	}
	if !containsSubstring(text, "[Assistant]: hi there") {
		t.Error("expected [Assistant]: hi there in output")
	}
}

func TestSerializeConversation_WithToolCalls(t *testing.T) {
	messages := []ai.Message{
		{
			Role: "assistant",
			AssistantContent: []ai.ContentBlock{
				{Type: "text", Text: "let me check"},
				{Type: "toolCall", ToolCallName: "read", ToolCallArguments: map[string]any{"path": "/tmp/test.txt"}},
			},
		},
	}
	text := SerializeConversation(messages)
	if !containsSubstring(text, "[Assistant]: let me check") {
		t.Error("expected assistant text in output")
	}
	if !containsSubstring(text, "[Assistant tool calls]: read(path=") {
		t.Error("expected tool call in output")
	}
}

func TestBashExecutionToText(t *testing.T) {
	exitCode := 1
	tests := []struct {
		name    string
		payload BashExecutionPayload
		check   func(string) bool
	}{
		{
			name:    "with output",
			payload: BashExecutionPayload{Command: "echo hi", Output: "hi"},
			check:   func(s string) bool { return containsSubstring(s, "echo hi") && containsSubstring(s, "hi") },
		},
		{
			name:    "no output",
			payload: BashExecutionPayload{Command: "true"},
			check:   func(s string) bool { return containsSubstring(s, "(no output)") },
		},
		{
			name:    "nonzero exit",
			payload: BashExecutionPayload{Command: "false", ExitCode: &exitCode},
			check:   func(s string) bool { return containsSubstring(s, "exited with code 1") },
		},
		{
			name:    "cancelled",
			payload: BashExecutionPayload{Command: "sleep 100", Cancelled: true},
			check:   func(s string) bool { return containsSubstring(s, "cancelled") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := BashExecutionToText(tt.payload)
			if !tt.check(text) {
				t.Errorf("check failed for output: %q", text)
			}
		})
	}
}

func TestGetMessageFromEntry(t *testing.T) {
	// Message entry
	entry := SessionTreeEntry{
		SessionTreeEntryBase: SessionTreeEntryBase{Type: "message", ID: "1", Timestamp: NowISO()},
		Message:              &ai.Message{Role: "user", Content: "hello"},
	}
	msg := GetMessageFromEntry(entry)
	if msg == nil || msg.Role != "user" {
		t.Error("expected user message")
	}

	// Branch summary entry
	entry2 := SessionTreeEntry{
		SessionTreeEntryBase: SessionTreeEntryBase{Type: "branch_summary", ID: "2", Timestamp: NowISO()},
		Summary:              "branch summary",
		FromID:               "old-leaf",
	}
	msg2 := GetMessageFromEntry(entry2)
	if msg2 == nil || msg2.Role != RoleBranchSummary {
		t.Error("expected branch summary message")
	}

	// Compaction entry
	entry3 := SessionTreeEntry{
		SessionTreeEntryBase: SessionTreeEntryBase{Type: "compaction", ID: "3", Timestamp: NowISO()},
		Summary:              "compacted",
		TokensBefore:         5000,
	}
	msg3 := GetMessageFromEntry(entry3)
	if msg3 == nil || msg3.Role != RoleCompactionSummary {
		t.Error("expected compaction summary message")
	}
}

func TestGetMessageFromEntryForCompaction(t *testing.T) {
	// Compaction entries should be skipped
	entry := SessionTreeEntry{
		SessionTreeEntryBase: SessionTreeEntryBase{Type: "compaction", ID: "1"},
		Summary:              "sum",
	}
	msg := GetMessageFromEntryForCompaction(entry)
	if msg != nil {
		t.Error("expected nil for compaction entry")
	}

	// Message entries should work
	entry2 := SessionTreeEntry{
		SessionTreeEntryBase: SessionTreeEntryBase{Type: "message", ID: "2"},
		Message:              &ai.Message{Role: "user", Content: "hello"},
	}
	msg2 := GetMessageFromEntryForCompaction(entry2)
	if msg2 == nil {
		t.Error("expected message for non-compaction entry")
	}
}

func TestSafeJSONStringify(t *testing.T) {
	result := SafeJSONStringify(map[string]any{"key": "value"})
	if result != `{"key":"value"}` {
		t.Errorf("unexpected: %q", result)
	}
}

func TestTruncateForSummary(t *testing.T) {
	// Short text — no truncation
	text := "hello"
	if truncated := TruncateForSummary(text, 100); truncated != text {
		t.Error("expected no truncation")
	}

	// Long text — truncation
	longText := ""
	for i := 0; i < 100; i++ {
		longText += "x"
	}
	truncated := TruncateForSummary(longText, 50)
	if len(truncated) <= 50 {
		t.Error("expected truncated text to include truncation notice")
	}
	if !containsSubstring(truncated, "truncated") {
		t.Error("expected truncation notice")
	}
}

func TestTimestampFromISO(t *testing.T) {
	ts := TimestampFromISO("2024-01-15T10:30:00Z")
	if ts == 0 {
		t.Error("expected non-zero timestamp")
	}

	ts = TimestampFromISO("")
	if ts != 0 {
		t.Error("expected 0 for empty string")
	}

	ts = TimestampFromISO("not-a-date")
	if ts != 0 {
		t.Error("expected 0 for invalid date")
	}
}

// Helper
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
