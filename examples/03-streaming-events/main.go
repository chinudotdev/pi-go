package main

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/chinudotdev/pi-go/agent"
	"github.com/chinudotdev/pi-go/ai"
)

// streamingMock simulates streaming text with delta events.
func streamingMock() agent.StreamFn {
	var called atomic.Int64
	words := []string{"Hello", " ", "from", " ", "the", " ", "stream", "!"}

	return func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
		called.Add(1)
		stream := ai.NewEventStream(ctx)

		go func() {
			output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
			stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})

			// Stream word by word
			for _, word := range words {
				output.Content = append(output.Content, ai.NewTextContent(word))
				word := word
				stream.Push(ai.AssistantMessageEvent{Type: "text_delta", Delta: &word, Partial: &output})
				time.Sleep(100 * time.Millisecond) // simulate latency
			}

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
		StreamFn: streamingMock(),
	})

	fmt.Println("Listening to stream events:\n")

	ag.Subscribe(func(e agent.Event) error {
		switch e.Type {
		case "agent_start":
			fmt.Println("🚀 Agent started")

		case "message_start":
			role := e.Msg.Role
			fmt.Printf("\n[%s] ", role)

		case "message_update":
			if e.StreamEvent != nil && e.StreamEvent.Delta != nil {
				fmt.Print(*e.StreamEvent.Delta)
			}

		case "message_end":
			fmt.Println()

		case "turn_start":
			fmt.Printf("  ↳ Turn started\n")

		case "turn_end":
			fmt.Printf("  ↳ Turn ended\n")

		case "agent_end":
			fmt.Println("\n🏁 Agent finished")
		}
		return nil
	})

	err := ag.Prompt(context.Background(), "Say hello!")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
