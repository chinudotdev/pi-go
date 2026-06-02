package main

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/chinudotdev/pi-go/agent"
	"github.com/chinudotdev/pi-go/ai"
)

func mockStream(response string) agent.StreamFn {
	var called atomic.Int64
	return func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
		called.Add(1)
		stream := ai.NewEventStream(ctx)
		go func() {
			output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
			stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})
			output.Content = append(output.Content, ai.NewTextContent(response))
			output.StopReason = ai.StopReasonStop
			output.Timestamp = time.Now().UnixMilli()
			stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
			stream.End(output)
		}()
		return stream, nil
	}
}

func main() {
	model := &ai.Model{
		ID:       "test-model",
		Provider: "test",
		API:      "openai-chat-completions",
	}

	ag := agent.New(agent.Options{
		InitialState: &agent.InitialState{
			Model:        model,
			SystemPrompt: "You are a helpful assistant.",
		},
		StreamFn: mockStream("Hello! I'm a mock response. In production, connect me to a real LLM!"),
	})

	unsub := ag.Subscribe(func(e agent.Event) error {
		switch e.Type {
		case "message_end":
			if e.Msg.Role == "assistant" {
				fmt.Printf("Assistant: %s\n", extractText(e.Msg))
			}
		}
		return nil
	})
	defer unsub()

	err := ag.Prompt(context.Background(), "Hello, what is 2+2?")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	state := ag.State()
	fmt.Printf("\nMessages in conversation: %d\n", len(state.Messages))
	fmt.Printf("Streaming: %v\n", state.IsStreaming)
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
	// Assistant messages store text in AssistantContent
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
