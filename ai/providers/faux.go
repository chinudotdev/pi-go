package providers

// Faux provider for testing.
// Generates deterministic fake responses without calling any real API.
// Useful for testing the streaming pipeline, tool calling flows, and message handling.

import (
	"context"
	"time"

	"github.com/chinudotdev/pi-go/ai"
)

// FauxModelDefinition describes a fake model for testing.
type FauxModelDefinition struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Reasoning bool   `json:"reasoning,omitempty"`
}

// DefaultFauxModel is the default fake model configuration.
var DefaultFauxModel = FauxModelDefinition{
	ID:   "faux-1",
	Name: "Faux Model",
}

// StreamFaux generates a fake streaming response for testing.
func StreamFaux(ctx context.Context, model *ai.Model, convCtx *ai.Context, options *ai.StreamOptions) (*ai.EventStream, error) {
	stream := ai.NewEventStream(ctx)

	go func() {
		output := ai.NewAssistantOutput("faux", "faux", model.ID)

		// Generate a simple text response
		text := "Hello! I am a faux model for testing purposes."

		stream.Push(ai.AssistantMessageEvent{
			Type:    "start",
			Partial: &output,
		})

		content := ai.NewTextContent(text)
		output.Content = append(output.Content, content)

		stream.Push(ai.AssistantMessageEvent{
			Type:    "text_delta",
			Delta:   &text,
			Partial: &output,
		})

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
}

// StreamSimpleFaux generates a fake streaming response with simplified options.
func StreamSimpleFaux(ctx context.Context, model *ai.Model, convCtx *ai.Context, options *ai.StreamOptions) (*ai.EventStream, error) {
	return StreamFaux(ctx, model, convCtx, options)
}
