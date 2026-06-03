// Example: Streaming Events — subscribe to real-time agent events.
//
//	mkdir my-agent && cd my-agent && go mod init my-agent
//	cp main.go . && go mod tidy
//	OPENAI_API_KEY="sk-..." go run main.go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/chinudotdev/pi-go/agent"
	"github.com/chinudotdev/pi-go/agent/harness"
	"github.com/chinudotdev/pi-go/ai"
	sdk "github.com/chinudotdev/pi-go/sdk"
)

func main() {
	ctx := context.Background()

	result, err := sdk.CreateSession(ctx, sdk.CreateSessionOptions{
		CWD:      ".",
		ToolList: []string{"read", "bash"},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create session: %v\n", err)
		os.Exit(1)
	}
	defer result.Session.Dispose(ctx)

	h := result.Session.Harness()

	// Listen for agent lifecycle events
	h.On("before_agent_start", func(event harness.HarnessEvent) (any, error) {
		fmt.Println("--- Agent starting ---")
		return nil, nil
	})

	// Track tool calls
	h.On("tool_call", func(event harness.HarnessEvent) (any, error) {
		fmt.Printf("\n[Calling tool: %s (%s)]\n", event.ToolName, event.ToolCallID)
		return nil, nil
	})

	// Track tool results
	h.On("tool_result", func(event harness.HarnessEvent) (any, error) {
		fmt.Printf("[Tool %s done]\n", event.ToolName)
		return nil, nil
	})

	// Track agent event forwarding (message_end, turn_end, agent_end, etc.)
	h.On(agent.EventMessageEnd, func(event harness.HarnessEvent) (any, error) {
		fmt.Println("[Message complete]")
		return nil, nil
	})

	h.On(agent.EventAgentEnd, func(event harness.HarnessEvent) (any, error) {
		fmt.Println("[Agent loop complete]")
		return nil, nil
	})

	h.On("settled", func(event harness.HarnessEvent) (any, error) {
		fmt.Println("--- Agent settled ---")
		return nil, nil
	})

	msg, err := result.Session.Prompt(ctx, "List the files in the current directory.", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prompt failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nFinal response:", extractText(msg))

	// Print session stats
	stats, _ := result.Session.GetSessionStats(ctx)
	fmt.Printf("\nStats: %d user msgs, %d assistant msgs, %d tool calls, $%.4f\n",
		stats.UserMessages, stats.AssistantMessages, stats.ToolCalls, stats.Cost)
}

func extractText(m *ai.Message) string {
	if m == nil {
		return ""
	}
	if s, ok := m.Content.(string); ok {
		return s
	}
	var text string
	for _, block := range m.AssistantContent {
		if block.Type == "text" {
			text += block.Text
		}
	}
	return text
}
