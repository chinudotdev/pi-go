// Example: Sessions — multi-turn conversations with session persistence.
//
//	mkdir my-agent && cd my-agent && go mod init my-agent
//	cp main.go . && go mod tidy
//	OPENAI_API_KEY="sk-..." go run main.go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/chinudotdev/pi-go/ai"
	sdk "github.com/chinudotdev/pi-go/sdk"
)

func main() {
	ctx := context.Background()

	result, err := sdk.CreateSession(ctx, sdk.CreateSessionOptions{
		CWD: ".",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create session: %v\n", err)
		os.Exit(1)
	}
	defer result.Session.Dispose(ctx)

	// Turn 1
	fmt.Println("--- Turn 1 ---")
	msg1, err := result.Session.Prompt(ctx, "My name is Alice. Remember that.", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prompt failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Agent:", extractText(msg1))

	// Turn 2 — the agent remembers the previous turn
	fmt.Println("\n--- Turn 2 ---")
	msg2, err := result.Session.Prompt(ctx, "What is my name?", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prompt failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Agent:", extractText(msg2))

	// Turn 3 — use a tool
	fmt.Println("\n--- Turn 3 ---")
	msg3, err := result.Session.Prompt(ctx, "Read any file in the current directory and summarize it.", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prompt failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Agent:", extractText(msg3))

	// Print session statistics
	stats, err := result.Session.GetSessionStats(ctx)
	if err == nil {
		fmt.Printf("\nSession stats: id=%s user_msgs=%d assistant_msgs=%d tool_calls=%d cost=$%.4f\n",
			stats.SessionID, stats.UserMessages, stats.AssistantMessages, stats.ToolCalls, stats.Cost)
	}

	// You can also steer the agent mid-run or queue follow-ups:
	// result.Session.Steer(ctx, "Be more concise", nil)
	// result.Session.FollowUp(ctx, "Also check subdirectories", nil)
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
