// Example: Custom Tool — define and register your own tool.
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
	"github.com/chinudotdev/pi-go/ai"
	sdk "github.com/chinudotdev/pi-go/sdk"
	"github.com/chinudotdev/pi-go/sdk/resources"
)

func main() {
	ctx := context.Background()

	// Define a custom tool
	getTimeTool := &agent.Tool{
		Name:        "get_current_time",
		Description: "Get the current date and time",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"timezone": map[string]any{
					"type":        "string",
					"description": "Timezone name (e.g. 'UTC', 'America/New_York')",
				},
			},
		},
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate agent.ToolUpdateCallback) (*agent.ToolResult, error) {
			tz := "UTC"
			if tzVal, ok := params["timezone"].(string); ok && tzVal != "" {
				tz = tzVal
			}
			return &agent.ToolResult{
				Content: []ai.ContentBlock{
					ai.NewTextContent(fmt.Sprintf("Current time in %s: 2025-01-15 12:00:00 (demo)", tz)),
				},
			}, nil
		},
	}

	customPrompt := "You are a time assistant. Use the get_current_time tool when asked about time."

	loader := resources.NewLoader(resources.LoaderOptions{
		CWD:                ".",
		NoSkills:           true,
		NoContextFiles:     true,
		NoPrompts:          true,
		SystemPromptSource: &customPrompt,
	})

	result, err := sdk.CreateSession(ctx, sdk.CreateSessionOptions{
		CWD:            ".",
		ResourceLoader: loader,
		NoTools:        true, // disable built-in tools
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create session: %v\n", err)
		os.Exit(1)
	}
	defer result.Session.Dispose(ctx)

	// Add the custom tool to the harness
	h := result.Session.Harness()
	h.SetTools(ctx, []agent.Tool{*getTimeTool}, []string{"get_current_time"})

	msg, err := result.Session.Prompt(ctx, "What time is it right now?", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prompt failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(extractText(msg))

	// Alternative: combine built-in tools with your custom tool
	// builtInTools := sdktools.CreateAllTools(".", nil)
	// allTools := []agent.Tool{*getTimeTool}
	// for _, t := range builtInTools {
	// 	allTools = append(allTools, *t)
	// }
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
