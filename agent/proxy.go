package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"time"

	"github.com/chinudotdev/pi-go/ai"
)

// ============================================================================
// Proxy event types
// ============================================================================

// ProxyAssistantMessageEvent represents server-sent proxy events.
// The server strips the partial field to reduce bandwidth; we reconstruct it client-side.
type ProxyAssistantMessageEvent struct {
	Type             string    `json:"type"`
	ContentIndex     int       `json:"contentIndex,omitempty"`
	Delta            string    `json:"delta,omitempty"`
	ContentSignature *string   `json:"contentSignature,omitempty"`
	ID               string    `json:"id,omitempty"`
	ToolName         string    `json:"toolName,omitempty"`
	Reason           string    `json:"reason,omitempty"`
	ErrorMessage     string    `json:"errorMessage,omitempty"`
	Usage            *ai.Usage `json:"usage,omitempty"`
}

// ProxySerializableStreamOptions contains the stream options sent to the proxy server.
type ProxySerializableStreamOptions struct {
	Temperature     *float64            `json:"temperature,omitempty"`
	MaxTokens       int                 `json:"maxTokens,omitempty"`
	Reasoning       ai.ThinkingLevel    `json:"reasoning,omitempty"`
	ThinkingBudgets *ai.ThinkingBudgets `json:"thinkingBudgets,omitempty"`
	SessionID       string              `json:"sessionId,omitempty"`
	Headers         map[string]string   `json:"headers,omitempty"`
	Metadata        map[string]string   `json:"metadata,omitempty"`
	MaxRetryDelayMs int                 `json:"maxRetryDelayMs,omitempty"`
}

// ProxyStreamOptions configures the proxy stream function.
type ProxyStreamOptions struct {
	ProxySerializableStreamOptions

	// AuthToken is the bearer token for the proxy server.
	AuthToken string
	// ProxyURL is the base URL of the proxy server (e.g., "https://genai.example.com").
	ProxyURL string
}

// ============================================================================
// StreamProxy — proxies LLM calls through a server
// ============================================================================

// StreamProxy is a stream function that proxies through a server instead of
// calling LLM providers directly. The server strips the partial field from
// delta events to reduce bandwidth; we reconstruct the partial message client-side.
//
// Use this as the StreamFn in AgentOptions when the agent needs to go through a proxy.
func StreamProxy(model *ai.Model, convCtx *ai.Context, options *ProxyStreamOptions) (*ai.EventStream, error) {
	stream := ai.NewEventStream(context.Background())

	// Initialize the partial message that we'll build up from events
	partial := &ai.AssistantMessage{
		Role:       "assistant",
		StopReason: ai.StopReasonStop,
		Content:    []ai.ContentBlock{},
		API:        model.API,
		Provider:   model.Provider,
		Model:      model.ID,
		Usage: ai.Usage{
			Cost: ai.Cost{},
		},
		Timestamp: time.Now().UnixMilli(),
	}

	// Per-stream accumulator for tool call JSON — avoids global mutable state.
	accum := &toolCallAccumulator{
		partialJSON: make(map[int]string),
	}

	go func() {

		// Build request body
		reqBody := map[string]any{
			"model":   model,
			"context": convCtx,
			"options": buildProxyRequestOptions(options),
		}
		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			pushProxyError(stream, partial, fmt.Sprintf("Failed to marshal request: %s", err))
			return
		}

		req, err := http.NewRequestWithContext(stream.Context(), "POST",
			strings.TrimRight(options.ProxyURL, "/")+"/api/stream",
			bytes.NewReader(bodyBytes))
		if err != nil {
			pushProxyError(stream, partial, fmt.Sprintf("Failed to create request: %s", err))
			return
		}
		req.Header.Set("Authorization", "Bearer "+options.AuthToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			reason := ai.StopReasonError
			if stream.Context().Err() != nil {
				reason = ai.StopReasonAborted
			}
			partial.StopReason = reason
			errMsg := err.Error()
			partial.ErrorMessage = &errMsg
			stream.Push(ai.AssistantMessageEvent{
				Type:   "error",
				Reason: reason,
				Error:  partial,
			})
			stream.Cancel()
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			errMsg := fmt.Sprintf("Proxy error: %d %s", resp.StatusCode, resp.Status)
			// Try to read error body
			var errBody struct {
				Error string `json:"error"`
			}
			if json.NewDecoder(resp.Body).Decode(&errBody) == nil && errBody.Error != "" {
				errMsg = "Proxy error: " + errBody.Error
			}
			pushProxyError(stream, partial, errMsg)
			return
		}

		// Read SSE events
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB max line

		for scanner.Scan() {
			if stream.Context().Err() != nil {
				pushProxyAbort(stream, partial)
				return
			}

			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimSpace(line[6:])
			if data == "" {
				continue
			}

			var proxyEvent ProxyAssistantMessageEvent
			if err := json.Unmarshal([]byte(data), &proxyEvent); err != nil {
				continue // Skip malformed events
			}

			event := processProxyEvent(&proxyEvent, partial, accum)
			if event != nil {
				// If this is a terminal event, end the stream
				if event.Type == "done" {
					stream.Push(*event)
					stream.Cancel()
					return
				}
				if event.Type == "error" {
					stream.Push(*event)
					stream.Cancel()
					return
				}
				stream.Push(*event)
			}
		}

		// If we get here, the stream ended without a done/error event.
		// This could be normal completion (some servers don't send done explicitly)
		// or an abort. Check context.
		if stream.Context().Err() != nil {
			partial.StopReason = ai.StopReasonAborted
			errMsg := "stream ended without done event"
			partial.ErrorMessage = &errMsg
		}
		stream.Cancel()
	}()

	return stream, nil
}

// ============================================================================
// Event processing
// ============================================================================

func processProxyEvent(proxy *ProxyAssistantMessageEvent, partial *ai.AssistantMessage, accum *toolCallAccumulator) *ai.AssistantMessageEvent {
	switch proxy.Type {
	case "start":
		return &ai.AssistantMessageEvent{
			Type:    "start",
			Partial: partial,
		}

	case "text_start":
		ensureContentIndex(partial, proxy.ContentIndex)
		partial.Content[proxy.ContentIndex] = ai.ContentBlock{
			Type: "text",
			Text: "",
		}
		return &ai.AssistantMessageEvent{
			Type:         "text_start",
			ContentIndex: proxyIntPtr(proxy.ContentIndex),
			Partial:      partial,
		}

	case "text_delta":
		if proxy.ContentIndex < len(partial.Content) && partial.Content[proxy.ContentIndex].Type == "text" {
			partial.Content[proxy.ContentIndex].Text += proxy.Delta
			return &ai.AssistantMessageEvent{
				Type:         "text_delta",
				ContentIndex: proxyIntPtr(proxy.ContentIndex),
				Delta:        &proxy.Delta,
				Partial:      partial,
			}
		}

	case "text_end":
		if proxy.ContentIndex < len(partial.Content) && partial.Content[proxy.ContentIndex].Type == "text" {
			partial.Content[proxy.ContentIndex].TextSignature = proxy.ContentSignature
			text := partial.Content[proxy.ContentIndex].Text
			return &ai.AssistantMessageEvent{
				Type:         "text_end",
				ContentIndex: proxyIntPtr(proxy.ContentIndex),
				Content:      &text,
				Partial:      partial,
			}
		}

	case "thinking_start":
		ensureContentIndex(partial, proxy.ContentIndex)
		partial.Content[proxy.ContentIndex] = ai.ContentBlock{
			Type:     "thinking",
			Thinking: "",
		}
		return &ai.AssistantMessageEvent{
			Type:         "thinking_start",
			ContentIndex: proxyIntPtr(proxy.ContentIndex),
			Partial:      partial,
		}

	case "thinking_delta":
		if proxy.ContentIndex < len(partial.Content) && partial.Content[proxy.ContentIndex].Type == "thinking" {
			partial.Content[proxy.ContentIndex].Thinking += proxy.Delta
			return &ai.AssistantMessageEvent{
				Type:         "thinking_delta",
				ContentIndex: proxyIntPtr(proxy.ContentIndex),
				Delta:        &proxy.Delta,
				Partial:      partial,
			}
		}

	case "thinking_end":
		if proxy.ContentIndex < len(partial.Content) && partial.Content[proxy.ContentIndex].Type == "thinking" {
			partial.Content[proxy.ContentIndex].ThinkingSignature = proxy.ContentSignature
			thinking := partial.Content[proxy.ContentIndex].Thinking
			return &ai.AssistantMessageEvent{
				Type:         "thinking_end",
				ContentIndex: proxyIntPtr(proxy.ContentIndex),
				Content:      &thinking,
				Partial:      partial,
			}
		}

	case "toolcall_start":
		ensureContentIndex(partial, proxy.ContentIndex)
		partial.Content[proxy.ContentIndex] = ai.ContentBlock{
			Type:              "toolCall",
			ToolCallID:        proxy.ID,
			ToolCallName:      proxy.ToolName,
			ToolCallArguments: map[string]any{},
		}
		return &ai.AssistantMessageEvent{
			Type:         "toolcall_start",
			ContentIndex: proxyIntPtr(proxy.ContentIndex),
			Partial:      partial,
		}

	case "toolcall_delta":
		if proxy.ContentIndex < len(partial.Content) && partial.Content[proxy.ContentIndex].Type == "toolCall" {
			// Accumulate partial JSON and parse incrementally
			accum.partialJSON[proxy.ContentIndex] += proxy.Delta
			parsed, _ := ai.ParseStreamingJSON(accum.partialJSON[proxy.ContentIndex])
			if parsed != nil {
				partial.Content[proxy.ContentIndex].ToolCallArguments = parsed
			}
			return &ai.AssistantMessageEvent{
				Type:         "toolcall_delta",
				ContentIndex: proxyIntPtr(proxy.ContentIndex),
				Delta:        &proxy.Delta,
				Partial:      partial,
			}
		}

	case "toolcall_end":
		if proxy.ContentIndex < len(partial.Content) && partial.Content[proxy.ContentIndex].Type == "toolCall" {
			cb := partial.Content[proxy.ContentIndex]
			tc := &ai.ToolCall{
				Type:      "toolCall",
				ID:        cb.ToolCallID,
				Name:      cb.ToolCallName,
				Arguments: cb.ToolCallArguments,
			}
			return &ai.AssistantMessageEvent{
				Type:         "toolcall_end",
				ContentIndex: proxyIntPtr(proxy.ContentIndex),
				ToolCall:     tc,
				Partial:      partial,
			}
		}

	case "done":
		partial.StopReason = ai.StopReason(proxy.Reason)
		if proxy.Usage != nil {
			partial.Usage = *proxy.Usage
		}
		return &ai.AssistantMessageEvent{
			Type:    "done",
			Reason:  ai.StopReason(proxy.Reason),
			Message: partial,
		}

	case "error":
		partial.StopReason = ai.StopReason(proxy.Reason)
		errMsg := proxy.ErrorMessage
		partial.ErrorMessage = &errMsg
		if proxy.Usage != nil {
			partial.Usage = *proxy.Usage
		}
		return &ai.AssistantMessageEvent{
			Type:   "error",
			Reason: ai.StopReason(proxy.Reason),
			Error:  partial,
		}
	}

	return nil
}

// ============================================================================
// Internal helpers
// ============================================================================

func buildProxyRequestOptions(opts *ProxyStreamOptions) ProxySerializableStreamOptions {
	if opts == nil {
		return ProxySerializableStreamOptions{}
	}
	return opts.ProxySerializableStreamOptions
}

func ensureContentIndex(partial *ai.AssistantMessage, idx int) {
	for len(partial.Content) <= idx {
		partial.Content = append(partial.Content, ai.ContentBlock{})
	}
}

func pushProxyError(stream *ai.EventStream, partial *ai.AssistantMessage, message string) {
	partial.StopReason = ai.StopReasonError
	partial.ErrorMessage = &message
	stream.Push(ai.AssistantMessageEvent{
		Type:   "error",
		Reason: ai.StopReasonError,
		Error:  partial,
	})
	stream.Cancel()
}

func pushProxyAbort(stream *ai.EventStream, partial *ai.AssistantMessage) {
	partial.StopReason = ai.StopReasonAborted
	errMsg := "Request aborted by user"
	partial.ErrorMessage = &errMsg
	stream.Push(ai.AssistantMessageEvent{
		Type:   "error",
		Reason: ai.StopReasonAborted,
		Error:  partial,
	})
	stream.Cancel()
}

func proxyIntPtr(v int) *int { return &v }

// toolCallAccumulator tracks partial JSON per content index for streaming tool calls.
// It is scoped to each StreamProxy call — no global mutable state.
type toolCallAccumulator struct {
	partialJSON map[int]string // contentIndex -> accumulated JSON
}

// Reset clears the accumulator state. Call between stream reuse if needed.
func (a *toolCallAccumulator) Reset() {
	a.partialJSON = make(map[int]string)
}

// ReadBody is a helper to read an io.ReadCloser for testing.
func ReadBody(r io.ReadCloser) ([]byte, error) {
	defer r.Close()
	return io.ReadAll(r)
}
