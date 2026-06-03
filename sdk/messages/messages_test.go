package messages

import (
	"strings"
	"testing"

	"github.com/chinudotdev/pi-go/ai"
)

func TestConvertToLlm_BashExecution(t *testing.T) {
	msg := NewBashExecutionMessage("ls -la", "file1.txt\nfile2.txt", 0, false, false, "", 1000)
	result := ConvertToLlm([]ai.Message{msg})
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	text := extractText(result[0])
	if !strings.Contains(text, "Ran `ls -la`") {
		t.Errorf("expected command in text, got: %s", text)
	}
	if !strings.Contains(text, "file1.txt") {
		t.Errorf("expected output in text, got: %s", text)
	}
}

func TestConvertToLlm_BashExecution_Excluded(t *testing.T) {
	msg := NewBashExecutionMessage("ls", "", 0, false, false, "", 1000)
	msg.Details.(*BashExecutionDetails).ExcludeFromCtx = true
	result := ConvertToLlm([]ai.Message{msg})
	if len(result) != 0 {
		t.Errorf("expected excluded message to be nil, got %d results", len(result))
	}
}

func TestConvertToLlm_BashExecution_Cancelled(t *testing.T) {
	msg := NewBashExecutionMessage("sleep 10", "", -1, true, false, "", 1000)
	result := ConvertToLlm([]ai.Message{msg})
	text := extractText(result[0])
	if !strings.Contains(text, "command cancelled") {
		t.Errorf("expected cancelled notice, got: %s", text)
	}
}

func TestConvertToLlm_BashExecution_Truncated(t *testing.T) {
	msg := NewBashExecutionMessage("big-output", "output...", 0, false, true, "/tmp/output.log", 1000)
	result := ConvertToLlm([]ai.Message{msg})
	text := extractText(result[0])
	if !strings.Contains(text, "Output truncated") {
		t.Errorf("expected truncation notice, got: %s", text)
	}
}

func TestConvertToLlm_BashExecution_Error(t *testing.T) {
	msg := NewBashExecutionMessage("bad-cmd", "error!", 1, false, false, "", 1000)
	result := ConvertToLlm([]ai.Message{msg})
	text := extractText(result[0])
	if !strings.Contains(text, "Command exited with code 1") {
		t.Errorf("expected error code, got: %s", text)
	}
}

func TestConvertToLlm_BranchSummary(t *testing.T) {
	msg := NewBranchSummaryMessage("Branch did X, Y, Z", "branch-123", 1000)
	result := ConvertToLlm([]ai.Message{msg})
	text := extractText(result[0])
	if !strings.Contains(text, "<summary>") {
		t.Errorf("expected summary tags, got: %s", text)
	}
	if !strings.Contains(text, "Branch did X") {
		t.Errorf("expected summary content, got: %s", text)
	}
}

func TestConvertToLlm_CompactionSummary(t *testing.T) {
	msg := NewCompactionSummaryMessage("User asked about X, Y happened", 50000, 1000)
	result := ConvertToLlm([]ai.Message{msg})
	text := extractText(result[0])
	if !strings.Contains(text, "compacted into the following summary") {
		t.Errorf("expected compaction prefix, got: %s", text)
	}
}

func TestConvertToLlm_CustomMessage(t *testing.T) {
	msg := NewCustomMessage("test-type", "hello world", true, 1000)
	result := ConvertToLlm([]ai.Message{msg})
	text := extractText(result[0])
	if text != "hello world" {
		t.Errorf("expected 'hello world', got: %s", text)
	}
}

func TestConvertToLlm_StandardUserMessage(t *testing.T) {
	msg := ai.Message{Role: "user", Content: "test input", Timestamp: 1000}
	result := ConvertToLlm([]ai.Message{msg})
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	text := extractText(result[0])
	if text != "test input" {
		t.Errorf("expected 'test input', got: %s", text)
	}
}

func TestConvertToLlm_MixedMessages(t *testing.T) {
	msgs := []ai.Message{
		{Role: "user", Content: "hello", Timestamp: 1},
		NewBashExecutionMessage("ls", "file.txt", 0, false, false, "", 2),
		NewCompactionSummaryMessage("summary text", 100, 3),
	}

	result := ConvertToLlm(msgs)
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
}

func TestConvertToLlm_Passthrough(t *testing.T) {
	// Assistant messages pass through unchanged
	msg := ai.Message{
		Role:             "assistant",
		AssistantContent: []ai.ContentBlock{ai.NewTextContent("response")},
		Timestamp:        1000,
	}
	result := ConvertToLlm([]ai.Message{msg})
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if result[0].Role != "assistant" {
		t.Errorf("expected assistant role, got %s", result[0].Role)
	}
}

func extractText(msg ai.Message) string {
	switch c := msg.Content.(type) {
	case string:
		return c
	case []ai.ContentBlock:
		if len(c) > 0 {
			return c[0].Text
		}
	}
	return ""
}
