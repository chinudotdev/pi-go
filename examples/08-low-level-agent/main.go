// Example: Low-Level Agent — use the agent package directly, no SDK.
//
// This shows the raw agent.Agent API without the SDK layer.
// Useful when you don't need file tools, sessions, or settings.
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
	_ "github.com/chinudotdev/pi-go/ai/providers" // registers all built-in providers
)

func main() {
	ctx := context.Background()

	model := &ai.Model{
		ID:       "gpt-4o",
		Provider: "openai",
		API:      "openai-chat-completions",
	}

	ag := agent.New(agent.Options{
		InitialState: &agent.InitialState{
			Model:        model,
			SystemPrompt: "You are a helpful assistant. Be concise.",
		},
	})

	// Subscribe to events
	ag.Subscribe(func(e agent.Event) error {
		switch e.Type {
		case agent.EventMessageUpdate:
			if e.StreamEvent != nil && e.StreamEvent.Delta != nil {
				fmt.Print(*e.StreamEvent.Delta)
			}
		case agent.EventToolExecutionStart:
			fmt.Printf("\n[Tool: %s]\n", e.ToolName)
		}
		return nil
	})

	fmt.Print("Response: ")
	err := ag.Prompt(ctx, "What is 2+2? Answer in one sentence.")
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nfailed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println()
}
