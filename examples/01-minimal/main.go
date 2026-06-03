// Example: Minimal — one-shot prompt with all defaults.
//
// Copy this file into a new Go project:
//
//	mkdir my-agent && cd my-agent
//	go mod init my-agent
//	cp main.go .
//	go mod tidy
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

	if result.ModelFallbackMessage != "" {
		fmt.Fprintln(os.Stderr, result.ModelFallbackMessage)
		os.Exit(1)
	}

	msg, err := result.Session.Prompt(ctx, "What files are in the current directory? List them briefly.", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prompt failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(extractText(msg))
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
