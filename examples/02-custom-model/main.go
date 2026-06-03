// Example: Custom Model — select a specific model and thinking level.
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

	// Option 1: Use a built-in model from the catalog
	model, ok := ai.GetModel("openai", "gpt-4o")
	if !ok {
		fmt.Fprintln(os.Stderr, "model gpt-4o not found in catalog")
		os.Exit(1)
	}
	fmt.Printf("Using model: %s/%s\n", model.Provider, model.ID)

	result, err := sdk.CreateSession(ctx, sdk.CreateSessionOptions{
		CWD:           ".",
		Model:         model,
		ThinkingLevel: "off", // "off", "minimal", "low", "medium", "high"
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create session: %v\n", err)
		os.Exit(1)
	}
	defer result.Session.Dispose(ctx)

	msg, err := result.Session.Prompt(ctx, "Say hello in one sentence.", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prompt failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(extractText(msg))

	// Option 2: Use an OpenAI-compatible provider (DeepSeek, xAI, OpenRouter, etc.)
	// customModel := &ai.Model{
	// 	ID:       "deepseek-chat",
	// 	Provider: "deepseek",
	// 	API:      "openai-chat-completions",
	// 	BaseURL:  "https://api.deepseek.com/v1",
	// }
	// result2, _ := sdk.CreateSession(ctx, sdk.CreateSessionOptions{
	// 	CWD:   ".",
	// 	Model: customModel,
	// })
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
