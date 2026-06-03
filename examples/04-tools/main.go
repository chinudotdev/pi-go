// Example: Tools — choose which built-in tools to enable.
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

	// Read-only mode — agent can read and search files but cannot modify them
	result, err := sdk.CreateSession(ctx, sdk.CreateSessionOptions{
		CWD:      ".",
		ToolList: []string{"read", "grep", "find", "ls"},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create session: %v\n", err)
		os.Exit(1)
	}
	defer result.Session.Dispose(ctx)

	fmt.Println("Active tools:", result.Session.GetActiveToolNames())

	msg, err := result.Session.Prompt(ctx, "Read the contents of the current directory and summarize what this project does.", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prompt failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(extractText(msg))

	// Other tool configurations:
	//
	// Full access (default):
	//   ToolList: nil  // or omit — enables read, bash, edit, write, grep, find, ls
	//
	// Exclude specific tools:
	//   ExcludeTools: []string{"bash", "write"}  // no shell or file writing
	//
	// No tools at all:
	//   NoTools: true  // pure LLM chat
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
