// Example: Full Control — override everything, no auto-discovery.
//
// Demonstrates creating a session with explicit auth, model, settings,
// resources, and tools — no filesystem discovery at all.
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
	"github.com/chinudotdev/pi-go/sdk/auth"
	"github.com/chinudotdev/pi-go/sdk/models"
	"github.com/chinudotdev/pi-go/sdk/resources"
	"github.com/chinudotdev/pi-go/sdk/settings"
)

func main() {
	ctx := context.Background()

	// Custom auth storage (in-memory)
	authStorage := auth.InMemory(nil)
	// Set API key at runtime (not persisted to disk)
	authStorage.SetRuntimeOverride("openai", os.Getenv("OPENAI_API_KEY"))

	// Custom model registry (in-memory, no models.json from disk)
	modelReg := models.InMemory(authStorage)

	// Pick a model from the built-in catalog
	model, ok := ai.GetModel("openai", "gpt-4o")
	if !ok {
		fmt.Fprintln(os.Stderr, "model not found")
		os.Exit(1)
	}

	// Custom settings (in-memory)
	settingsMgr := settings.InMemory()

	// Custom resources — no filesystem discovery
	customPrompt := "You are a minimal assistant. Be concise."
	resLoader := resources.NewLoader(resources.LoaderOptions{
		CWD:                ".",
		NoSkills:           true,
		NoPrompts:          true,
		NoContextFiles:     true,
		SystemPromptSource: &customPrompt,
	})

	result, err := sdk.CreateSession(ctx, sdk.CreateSessionOptions{
		CWD:            ".",
		Model:          model,
		ThinkingLevel:  "off",
		ToolList:       []string{"read", "bash"},
		AuthStorage:    authStorage,
		SettingsMgr:    settingsMgr,
		ModelRegistry:  modelReg,
		ResourceLoader: resLoader,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create session: %v\n", err)
		os.Exit(1)
	}
	defer result.Session.Dispose(ctx)

	msg, err := result.Session.Prompt(ctx, "List the files in the current directory.", nil)
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
