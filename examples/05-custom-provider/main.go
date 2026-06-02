package main

import (
	"context"
	"fmt"
	"time"

	"github.com/chinudotdev/pi-go/ai"
)

// This example shows how to register a custom API provider.
// Useful for integrating with your own backend or an unsupported LLM API.

func main() {
	// Register a custom provider
	ai.RegisterApiProvider(ai.ApiProvider{
		API: "my-custom-api",
		Stream: func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.StreamOptions) (*ai.EventStream, error) {
			stream := ai.NewEventStream(ctx)

			go func() {
				output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
				stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})

				// In a real provider, you'd make an HTTP request to your API here
				response := "Hello from my custom provider!"
				output.Content = append(output.Content, ai.NewTextContent(response))
				output.StopReason = ai.StopReasonStop
				output.Timestamp = time.Now().UnixMilli()

				stream.Push(ai.AssistantMessageEvent{
					Type:    "done",
					Reason:  ai.StopReasonStop,
					Message: &output,
				})
				stream.End(output)
			}()

			return stream, nil
		},
		StreamSimple: func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.StreamOptions) (*ai.EventStream, error) {
			// Reuse the Stream implementation
			p, _ := ai.GetApiProvider("my-custom-api")
			return p.Stream(ctx, model, convCtx, opts)
		},
	})

	// Now you can use models with API: "my-custom-api"
	model := &ai.Model{
		ID:       "my-model-v1",
		Provider: "custom",
		API:      "my-custom-api",
	}

	fmt.Printf("Registered custom provider for model: %s\n", model.ID)
	fmt.Printf("Provider: %s, API: %s\n", model.Provider, model.API)

	// Use with ai.StreamSimple
	ctx := context.Background()
	systemPrompt := "You are a test assistant."
	convCtx := &ai.Context{
		SystemPrompt: &systemPrompt,
		Messages:     []ai.Message{{Role: "user", Content: "Hello!"}},
	}

	apiKey := "test-key"
	stream, err := ai.StreamSimple(ctx, model, convCtx, &ai.SimpleStreamOptions{
		StreamOptions: ai.StreamOptions{
			APIKey: &apiKey,
		},
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Read events until stream ends
	for event := range stream.Iterate() {
		if event.Type == "done" && event.Message != nil {
			fmt.Printf("Response: %s\n", extractTextFromBlocks(event.Message.Content))
		}
	}

	// Clean up
	ai.UnregisterApiProviders("my-custom-api")
	fmt.Println("Provider unregistered.")
}

func extractTextFromBlocks(blocks []ai.ContentBlock) string {
	for _, b := range blocks {
		if b.Type == "text" {
			return b.Text
		}
	}
	return ""
}
