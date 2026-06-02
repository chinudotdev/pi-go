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

// multiResponseMock returns different responses for each call.
func multiResponseMock(responses []string) agent.StreamFn {
	var idx atomic.Int64
	return func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
		n := int(idx.Add(1)) - 1
		text := responses[n%len(responses)]

		stream := ai.NewEventStream(ctx)
		go func() {
			output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
			stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})
			output.Content = append(output.Content, ai.NewTextContent(text))
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
		StreamFn: multiResponseMock([]string{
			"I'm doing great! How can I help?",
			"Thanks for the follow-up! I'll focus on Go now.",
		}),
		SteeringMode: agent.QueueOneAtATime,
		FollowUpMode: agent.QueueOneAtATime,
	})

	// Track all events
	var eventTypes []string
	ag.Subscribe(func(e agent.Event) error {
		eventTypes = append(eventTypes, e.Type)
		switch e.Type {
		case "message_end":
			if e.Msg.Role == "user" {
				fmt.Printf("👤 User: %s\n", extractText(e.Msg))
			} else if e.Msg.Role == "assistant" {
				fmt.Printf("🤖 Assistant: %s\n", extractText(e.Msg))
			}
		case "agent_end":
			fmt.Println("\n🏁 Agent run finished")
		}
		return nil
	})

	// Queue a follow-up message (processed after agent stops)
	ag.FollowUp(ai.Message{
		Role:    "user",
		Content: "Now tell me about Go interfaces.",
	})
	fmt.Println("📥 Queued follow-up message")

	// Start the agent
	fmt.Println("=== Starting agent ===")
	err := ag.Prompt(context.Background(), "Hello! How are you?")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}

	// Check queues
	fmt.Printf("\nQueued messages remaining: %v\n", ag.HasQueuedMessages())

	// Demonstrate steering
	fmt.Println("\n=== Steering example ===")
	ag.SetSteeringMode(agent.QueueOneAtATime)
	ag.ClearAllQueues()

	fmt.Printf("Event types seen: %s\n", strings.Join(eventTypes, ", "))
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
