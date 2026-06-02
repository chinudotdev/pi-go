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

// mockToolCallStream simulates a model that calls a tool and then responds.
func mockToolCallStream(toolName, finalResponse string) agent.StreamFn {
	var call atomic.Int64
	return func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
		n := call.Add(1)
		stream := ai.NewEventStream(ctx)

		go func() {
			output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
			stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})

			if n == 1 {
				// First call: model requests a tool
				output.Content = append(output.Content, ai.ContentBlock{
					Type:              "toolCall",
					ToolCallID:        "call_1",
					ToolCallName:      toolName,
					ToolCallArguments: map[string]any{"city": "Tokyo"},
				})
				output.StopReason = ai.StopReasonToolUse
				output.Timestamp = time.Now().UnixMilli()
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonToolUse, Message: &output})
				stream.End(output)
			} else {
				// Second call: model gives final answer
				output.Content = append(output.Content, ai.NewTextContent(finalResponse))
				output.StopReason = ai.StopReasonStop
				output.Timestamp = time.Now().UnixMilli()
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
				stream.End(output)
			}
		}()
		return stream, nil
	}
}

// weatherTool is a simple tool that returns mock weather data.
var weatherTool = &agent.Tool{
	Name:        "get_weather",
	Description: "Get the current weather for a city",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"city": map[string]any{
				"type":        "string",
				"description": "City name",
			},
		},
		"required": []any{"city"},
	},
	Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate agent.ToolUpdateCallback) (*agent.ToolResult, error) {
		city := params["city"].(string)
		result := fmt.Sprintf("Weather in %s: ☀️ Sunny, 72°F (22°C)", city)
		return &agent.ToolResult{
			Content: []ai.ContentBlock{
				ai.NewTextContent(result),
			},
		}, nil
	},
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
			SystemPrompt: "You are a weather assistant. Use the get_weather tool when asked about weather.",
			Tools:        []*agent.Tool{weatherTool},
		},
		StreamFn: mockToolCallStream(
			"get_weather",
			"Based on the weather data, Tokyo is currently sunny at 72°F (22°C). Great day to be outside!",
		),
	})

	// Track events
	ag.Subscribe(func(e agent.Event) error {
		switch e.Type {
		case "tool_execution_start":
			fmt.Printf("🔧 Calling tool: %s\n", e.ToolName)
			fmt.Printf("   Args: %v\n", e.Args)
		case "tool_execution_end":
			fmt.Printf("✅ Tool completed: %s\n", e.ToolName)
		case "message_end":
			if e.Msg.Role == "assistant" {
				text := extractText(e.Msg)
				if text != "" {
					fmt.Printf("🤖 Assistant: %s\n", text)
				}
			}
		}
		return nil
	})

	err := ag.Prompt(context.Background(), "What's the weather in Tokyo?")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("\n--- Final State ---")
	state := ag.State()
	fmt.Printf("Messages: %d\n", len(state.Messages))
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
