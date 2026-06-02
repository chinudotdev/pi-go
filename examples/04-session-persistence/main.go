package main

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/chinudotdev/pi-go/agent"
	"github.com/chinudotdev/pi-go/agent/harness"
	"github.com/chinudotdev/pi-go/agent/harness/session"
	"github.com/chinudotdev/pi-go/ai"
)

func main() {
	// Register a mock provider for this example
	apiName := "example-session-api"
	var callCount atomic.Int64
	ai.RegisterApiProvider(ai.ApiProvider{
		API: ai.Api(apiName),
		Stream: func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.StreamOptions) (*ai.EventStream, error) {
			n := callCount.Add(1)
			stream := ai.NewEventStream(ctx)
			go func() {
				output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
				stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})
				text := fmt.Sprintf("[response %d] I remember our conversation! (turn %d)", n, n)
				output.Content = append(output.Content, ai.NewTextContent(text))
				output.StopReason = ai.StopReasonStop
				output.Timestamp = time.Now().UnixMilli()
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
				stream.End(output)
			}()
			return stream, nil
		},
		StreamSimple: func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.StreamOptions) (*ai.EventStream, error) {
			p, _ := ai.GetApiProvider(ai.Api(apiName))
			return p.Stream(ctx, model, convCtx, opts)
		},
	})
	defer ai.UnregisterApiProviders(apiName)

	// Create in-memory session storage
	storage := session.NewInMemorySessionStorage(&session.InMemoryStorageOptions{
		Metadata: &harness.SessionMetadata{ID: "demo-session"},
	})
	sess := session.NewSession(storage)

	model := &ai.Model{
		ID:       "example-model",
		Provider: "example",
		API:      apiName,
	}

	h := harness.NewAgentHarness(harness.HarnessOptions{
		Model: model,
		Tools: []agent.Tool{},
		GetApiKeyAndHeaders: func(m *ai.Model) (*harness.AuthInfo, error) {
			return &harness.AuthInfo{APIKey: "mock-key"}, nil
		},
	}, sess)

	// Subscribe to harness events
	h.Subscribe(func(e harness.HarnessEvent) (any, error) {
		switch e.Type {
		case "before_agent_start":
			fmt.Printf("🚀 Starting turn: %q\n", e.Prompt)
		case "settled":
			fmt.Println("✅ Turn settled")
		}
		return nil, nil
	})

	// Turn 1
	fmt.Println("=== Turn 1 ===")
	result, err := h.Prompt(context.Background(), "What is 2+2?", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("🤖 %s\n", extractText(*result))

	// Turn 2 — context is preserved in session
	fmt.Println("\n=== Turn 2 ===")
	result, err = h.Prompt(context.Background(), "What did I just ask?", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("🤖 %s\n", extractText(*result))

	entries, _ := storage.GetEntries(context.Background())
	fmt.Printf("\n📝 Session has %d entries\n", len(entries))
}

func extractText(m ai.Message) string {
	switch c := m.Content.(type) {
	case string:
		return c
	case []ai.ContentBlock:
		var b strings.Builder
		for _, block := range c {
			if block.Type == "text" && block.Text != "" {
				b.WriteString(block.Text)
			}
		}
		return b.String()
	}
	if len(m.AssistantContent) > 0 {
		var b strings.Builder
		for _, block := range m.AssistantContent {
			if block.Type == "text" && block.Text != "" {
				b.WriteString(block.Text)
			}
		}
		return b.String()
	}
	return ""
}
