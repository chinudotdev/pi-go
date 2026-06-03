package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chinudotdev/pi-go/ai"
)

// ============================================================================
// Test server helpers
// ============================================================================

func newProxyServer(handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/stream", handler)
	return httptest.NewServer(mux)
}

func writeSSE(w http.ResponseWriter, event any) {
	data, _ := json.Marshal(event)
	fmt.Fprintf(w, "data: %s\n\n", data)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func parseProxyRequest(r *http.Request) (model *ai.Model, convCtx *ai.Context, err error) {
	if r.Method != "POST" {
		return nil, nil, fmt.Errorf("expected POST, got %s", r.Method)
	}
	if r.Header.Get("Authorization") != "Bearer test-token" {
		return nil, nil, fmt.Errorf("expected Bearer test-token, got %s", r.Header.Get("Authorization"))
	}
	if r.Header.Get("Content-Type") != "application/json" {
		return nil, nil, fmt.Errorf("expected application/json content-type")
	}

	var body struct {
		Model   *ai.Model   `json:"model"`
		Context *ai.Context `json:"context"`
		Options any         `json:"options"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, nil, err
	}
	return body.Model, body.Context, nil
}

// drainStream reads all events from the stream until context is cancelled.
// Returns the last "done" or "error" event's message.
func drainStream(stream *ai.EventStream) (*ai.AssistantMessage, []ai.AssistantMessageEvent) {
	var events []ai.AssistantMessageEvent
	var result *ai.AssistantMessage
	// Phase 1: read events until context is done
	for {
		select {
		case event, ok := <-stream.Events:
			if !ok {
				return result, events
			}
			events = append(events, event)
			if event.Type == "done" && event.Message != nil {
				result = event.Message
			}
			if event.Type == "error" && event.Error != nil {
				result = event.Error
			}
		case <-stream.Context().Done():
			goto drain
		}
	}

drain:
	// Phase 2: drain any buffered events
	for {
		select {
		case event, ok := <-stream.Events:
			if !ok {
				return result, events
			}
			events = append(events, event)
			if event.Type == "done" && event.Message != nil {
				result = event.Message
			}
			if event.Type == "error" && event.Error != nil {
				result = event.Error
			}
		default:
			return result, events
		}
	}
}

// ============================================================================
// Tests
// ============================================================================

func TestStreamProxy_TextGeneration(t *testing.T) {
	server := newProxyServer(func(w http.ResponseWriter, r *http.Request) {
		model, _, err := parseProxyRequest(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if model.ID != "gpt-4" {
			t.Errorf("expected gpt-4, got %s", model.ID)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(w, ProxyAssistantMessageEvent{Type: "start"})
		writeSSE(w, ProxyAssistantMessageEvent{Type: "text_start", ContentIndex: 0})
		writeSSE(w, ProxyAssistantMessageEvent{Type: "text_delta", ContentIndex: 0, Delta: "Hello"})
		writeSSE(w, ProxyAssistantMessageEvent{Type: "text_delta", ContentIndex: 0, Delta: " world"})
		writeSSE(w, ProxyAssistantMessageEvent{Type: "text_end", ContentIndex: 0})
		writeSSE(w, ProxyAssistantMessageEvent{
			Type:   "done",
			Reason: "stop",
			Usage: &ai.Usage{
				Input:  10,
				Output: 5,
				Cost:   ai.Cost{Input: 0.001, Output: 0.002, Total: 0.003},
			},
		})
	})
	defer server.Close()

	model := &ai.Model{ID: "gpt-4", Provider: "openai", API: "openai-completions"}
	convCtx := &ai.Context{
		Messages: []ai.Message{
			{Role: "user", Content: "Say hello"},
		},
	}

	stream, err := StreamProxy(model, convCtx, &ProxyStreamOptions{
		AuthToken: "test-token",
		ProxyURL:  server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, events := drainStream(stream)

	var gotDone bool
	for _, e := range events {
		if e.Type == "done" {
			gotDone = true
		}
	}
	if !gotDone {
		t.Fatal("expected done event")
	}
	if result == nil {
		t.Fatal("expected result message")
	}
	if result.Content[0].Text != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", result.Content[0].Text)
	}
	if result.StopReason != ai.StopReasonStop {
		t.Errorf("expected stop, got %s", result.StopReason)
	}
	if result.Usage.Input != 10 || result.Usage.Output != 5 {
		t.Errorf("expected usage 10/5, got %d/%d", result.Usage.Input, result.Usage.Output)
	}
}

func TestStreamProxy_ThinkingAndText(t *testing.T) {
	server := newProxyServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(w, ProxyAssistantMessageEvent{Type: "start"})
		writeSSE(w, ProxyAssistantMessageEvent{Type: "thinking_start", ContentIndex: 0})
		writeSSE(w, ProxyAssistantMessageEvent{Type: "thinking_delta", ContentIndex: 0, Delta: "Let me think..."})
		sig := "abc123"
		writeSSE(w, ProxyAssistantMessageEvent{Type: "thinking_end", ContentIndex: 0, ContentSignature: &sig})
		writeSSE(w, ProxyAssistantMessageEvent{Type: "text_start", ContentIndex: 1})
		writeSSE(w, ProxyAssistantMessageEvent{Type: "text_delta", ContentIndex: 1, Delta: "Answer"})
		writeSSE(w, ProxyAssistantMessageEvent{Type: "text_end", ContentIndex: 1})
		writeSSE(w, ProxyAssistantMessageEvent{Type: "done", Reason: "stop", Usage: &ai.Usage{}})
	})
	defer server.Close()

	model := &ai.Model{ID: "claude-4", Provider: "anthropic", API: "anthropic"}
	convCtx := &ai.Context{Messages: []ai.Message{{Role: "user", Content: "Think and answer"}}}

	stream, _ := StreamProxy(model, convCtx, &ProxyStreamOptions{
		AuthToken: "test-token",
		ProxyURL:  server.URL,
	})

	result, _ := drainStream(stream)
	if result == nil {
		t.Fatal("expected result")
	}
	if len(result.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(result.Content))
	}
	if result.Content[0].Type != "thinking" || result.Content[0].Thinking != "Let me think..." {
		t.Errorf("expected thinking block, got %+v", result.Content[0])
	}
	if result.Content[1].Type != "text" || result.Content[1].Text != "Answer" {
		t.Errorf("expected text block, got %+v", result.Content[1])
	}
	if result.Content[0].ThinkingSignature == nil || *result.Content[0].ThinkingSignature != "abc123" {
		t.Errorf("expected thinking signature")
	}
}

func TestStreamProxy_ToolCall(t *testing.T) {
	server := newProxyServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(w, ProxyAssistantMessageEvent{Type: "start"})
		writeSSE(w, ProxyAssistantMessageEvent{Type: "text_start", ContentIndex: 0})
		writeSSE(w, ProxyAssistantMessageEvent{Type: "text_delta", ContentIndex: 0, Delta: "Let me check."})
		writeSSE(w, ProxyAssistantMessageEvent{Type: "text_end", ContentIndex: 0})
		writeSSE(w, ProxyAssistantMessageEvent{Type: "toolcall_start", ContentIndex: 1, ID: "tc-1", ToolName: "read_file"})
		writeSSE(w, ProxyAssistantMessageEvent{Type: "toolcall_delta", ContentIndex: 1, Delta: "{\"path\":"})
		writeSSE(w, ProxyAssistantMessageEvent{Type: "toolcall_delta", ContentIndex: 1, Delta: "\"/tmp/test\"}"})
		writeSSE(w, ProxyAssistantMessageEvent{Type: "toolcall_end", ContentIndex: 1})
		writeSSE(w, ProxyAssistantMessageEvent{Type: "done", Reason: "toolUse", Usage: &ai.Usage{}})
	})
	defer server.Close()

	model := &ai.Model{ID: "gpt-4", Provider: "openai", API: "openai-completions"}
	convCtx := &ai.Context{Messages: []ai.Message{{Role: "user", Content: "Read test file"}}}

	stream, _ := StreamProxy(model, convCtx, &ProxyStreamOptions{
		AuthToken: "test-token",
		ProxyURL:  server.URL,
	})

	result, _ := drainStream(stream)
	if result == nil {
		t.Fatal("expected result")
	}
	if result.StopReason != ai.StopReasonToolUse {
		t.Errorf("expected toolUse, got %s", result.StopReason)
	}
	if len(result.Content) < 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(result.Content))
	}
	tc := result.Content[1]
	if tc.Type != "toolCall" {
		t.Errorf("expected toolCall, got %s", tc.Type)
	}
	if tc.ToolCallID != "tc-1" {
		t.Errorf("expected tc-1, got %s", tc.ToolCallID)
	}
	if tc.ToolCallName != "read_file" {
		t.Errorf("expected read_file, got %s", tc.ToolCallName)
	}
	// Verify tool call arguments were properly accumulated from delta events
	if tc.ToolCallArguments == nil {
		t.Error("expected tool call arguments to be parsed")
	} else if path, ok := tc.ToolCallArguments["path"]; !ok || path != "/tmp/test" {
		t.Errorf("expected path=/tmp/test, got %v", tc.ToolCallArguments)
	}
}

func TestStreamProxy_Error(t *testing.T) {
	server := newProxyServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(w, ProxyAssistantMessageEvent{Type: "start"})
		writeSSE(w, ProxyAssistantMessageEvent{Type: "text_start", ContentIndex: 0})
		writeSSE(w, ProxyAssistantMessageEvent{Type: "text_delta", ContentIndex: 0, Delta: "partial"})
		writeSSE(w, ProxyAssistantMessageEvent{
			Type:         "error",
			Reason:       "error",
			ErrorMessage: "Rate limit exceeded",
			Usage:        &ai.Usage{Input: 5},
		})
	})
	defer server.Close()

	model := &ai.Model{ID: "gpt-4", Provider: "openai", API: "openai-completions"}
	convCtx := &ai.Context{Messages: []ai.Message{{Role: "user", Content: "test"}}}

	stream, _ := StreamProxy(model, convCtx, &ProxyStreamOptions{
		AuthToken: "test-token",
		ProxyURL:  server.URL,
	})

	_, events := drainStream(stream)
	var errorEvent *ai.AssistantMessageEvent
	for _, e := range events {
		if e.Type == "error" {
			errorEvent = &e
		}
	}
	if errorEvent == nil {
		t.Fatal("expected error event")
	}
	if errorEvent.Error == nil {
		t.Fatal("expected error message")
	}
	if errorEvent.Error.StopReason != ai.StopReasonError {
		t.Errorf("expected error stop reason, got %s", errorEvent.Error.StopReason)
	}
	if errorEvent.Error.ErrorMessage == nil || !strings.Contains(*errorEvent.Error.ErrorMessage, "Rate limit") {
		t.Errorf("expected rate limit error, got %v", errorEvent.Error.ErrorMessage)
	}
}

func TestStreamProxy_HTTPError(t *testing.T) {
	server := newProxyServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{"error": "Too many requests"})
	})
	defer server.Close()

	model := &ai.Model{ID: "gpt-4", Provider: "openai", API: "openai-completions"}
	convCtx := &ai.Context{Messages: []ai.Message{{Role: "user", Content: "test"}}}

	stream, _ := StreamProxy(model, convCtx, &ProxyStreamOptions{
		AuthToken: "test-token",
		ProxyURL:  server.URL,
	})

	_, events := drainStream(stream)
	var errorEvent *ai.AssistantMessageEvent
	for _, e := range events {
		if e.Type == "error" {
			errorEvent = &e
		}
	}
	if errorEvent == nil {
		t.Fatal("expected error event for HTTP error")
	}
	if errorEvent.Error == nil || errorEvent.Error.ErrorMessage == nil {
		t.Fatal("expected error message")
	}
	if !strings.Contains(*errorEvent.Error.ErrorMessage, "Too many requests") {
		t.Errorf("expected 'Too many requests' in error, got %s", *errorEvent.Error.ErrorMessage)
	}
}

func TestStreamProxy_AuthHeader(t *testing.T) {
	var receivedAuth string
	server := newProxyServer(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(w, ProxyAssistantMessageEvent{Type: "done", Reason: "stop", Usage: &ai.Usage{}})
	})
	defer server.Close()

	model := &ai.Model{ID: "test", Provider: "test", API: "test"}
	convCtx := &ai.Context{Messages: []ai.Message{{Role: "user", Content: "test"}}}

	stream, _ := StreamProxy(model, convCtx, &ProxyStreamOptions{
		AuthToken: "my-secret-token",
		ProxyURL:  server.URL,
	})

	drainStream(stream)
	if receivedAuth != "Bearer my-secret-token" {
		t.Errorf("expected 'Bearer my-secret-token', got %q", receivedAuth)
	}
}

func TestStreamProxy_ContextCancellation(t *testing.T) {
	blockCh := make(chan struct{})
	server := newProxyServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(w, ProxyAssistantMessageEvent{Type: "start"})
		<-blockCh
	})
	defer func() {
		close(blockCh)
		server.Close()
	}()

	model := &ai.Model{ID: "test", Provider: "test", API: "test"}
	convCtx := &ai.Context{Messages: []ai.Message{{Role: "user", Content: "test"}}}

	stream, _ := StreamProxy(model, convCtx, &ProxyStreamOptions{
		AuthToken: "test-token",
		ProxyURL:  server.URL,
	})

	go func() {
		time.Sleep(100 * time.Millisecond)
		stream.Cancel()
	}()

	_, events := drainStream(stream)
	if len(events) == 0 {
		t.Error("expected at least one event before cancellation")
	}
}

func TestStreamProxy_SendsCorrectBody(t *testing.T) {
	var receivedBody struct {
		Model   *ai.Model   `json:"model"`
		Context *ai.Context `json:"context"`
		Options any         `json:"options"`
	}

	server := newProxyServer(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(w, ProxyAssistantMessageEvent{Type: "done", Reason: "stop", Usage: &ai.Usage{}})
	})
	defer server.Close()

	model := &ai.Model{ID: "gpt-4o", Provider: "openai", API: "openai-completions"}
	convCtx := &ai.Context{
		Messages: []ai.Message{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", AssistantContent: []ai.ContentBlock{{Type: "text", Text: "Hi"}}},
		},
	}

	temp := 0.7
	stream, _ := StreamProxy(model, convCtx, &ProxyStreamOptions{
		AuthToken: "test-token",
		ProxyURL:  server.URL,
		ProxySerializableStreamOptions: ProxySerializableStreamOptions{
			Temperature: &temp,
			MaxTokens:   100,
		},
	})

	drainStream(stream)

	if receivedBody.Model == nil || receivedBody.Model.ID != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %+v", receivedBody.Model)
	}
	if receivedBody.Context == nil || len(receivedBody.Context.Messages) != 2 {
		t.Errorf("expected 2 messages, got %+v", receivedBody.Context)
	}
}

func TestProcessProxyEvent_TextFlow(t *testing.T) {
	partial := &ai.AssistantMessage{
		Role:    "assistant",
		Content: []ai.ContentBlock{},
	}
	accum := &toolCallAccumulator{partialJSON: make(map[int]string)}

	e := processProxyEvent(&ProxyAssistantMessageEvent{Type: "start"}, partial, accum)
	if e == nil || e.Type != "start" {
		t.Fatal("expected start event")
	}

	e = processProxyEvent(&ProxyAssistantMessageEvent{Type: "text_start", ContentIndex: 0}, partial, accum)
	if e == nil || e.Type != "text_start" {
		t.Fatal("expected text_start event")
	}

	e = processProxyEvent(&ProxyAssistantMessageEvent{Type: "text_delta", ContentIndex: 0, Delta: "hi"}, partial, accum)
	if e == nil || e.Type != "text_delta" {
		t.Fatal("expected text_delta event")
	}
	if partial.Content[0].Text != "hi" {
		t.Errorf("expected 'hi', got %q", partial.Content[0].Text)
	}

	sig := "sig1"
	e = processProxyEvent(&ProxyAssistantMessageEvent{Type: "text_end", ContentIndex: 0, ContentSignature: &sig}, partial, accum)
	if e == nil || e.Type != "text_end" {
		t.Fatal("expected text_end event")
	}
	if partial.Content[0].TextSignature == nil || *partial.Content[0].TextSignature != "sig1" {
		t.Error("expected text signature")
	}
}

func TestProcessProxyEvent_DoneWithUsage(t *testing.T) {
	partial := &ai.AssistantMessage{
		Role:    "assistant",
		Content: []ai.ContentBlock{},
	}

	accum := &toolCallAccumulator{partialJSON: make(map[int]string)}
	e := processProxyEvent(&ProxyAssistantMessageEvent{
		Type:   "done",
		Reason: "stop",
		Usage:  &ai.Usage{Input: 100, Output: 50},
	}, partial, accum)

	if e == nil || e.Type != "done" {
		t.Fatal("expected done event")
	}
	if partial.StopReason != ai.StopReasonStop {
		t.Errorf("expected stop, got %s", partial.StopReason)
	}
	if partial.Usage.Input != 100 {
		t.Errorf("expected input 100, got %d", partial.Usage.Input)
	}
}
