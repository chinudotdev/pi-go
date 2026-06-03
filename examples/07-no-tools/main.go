// Example: No Tools — pure LLM chat with no file tools.
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
	"github.com/chinudotdev/pi-go/sdk/resources"
)

func main() {
	ctx := context.Background()

	customPrompt := "You are a helpful coding assistant. Be concise and provide code examples."

	loader := resources.NewLoader(resources.LoaderOptions{
		CWD:               ".",
		NoSkills:          true,
		NoContextFiles:    true,
		NoPrompts:         true,
		SystemPromptSource: &customPrompt,
	})

	result, err := sdk.CreateSession(ctx, sdk.CreateSessionOptions{
		CWD:            ".",
		NoTools:        true,
		ResourceLoader: loader,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create session: %v\n", err)
		os.Exit(1)
	}
	defer result.Session.Dispose(ctx)

	msg, err := result.Session.Prompt(ctx, "Write a Go function that reverses a string.", nil)
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
