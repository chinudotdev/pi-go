# Building Apps with pi-go SDK

This guide shows how to use `pi-go/sdk` to build applications powered by a
coding agent. The SDK handles tools, auth, settings, model management, and
session persistence — you just bring the UI.

## Installation

```bash
# Add the dependency (once published)
go get github.com/chinudotdev/pi-go/sdk

# Or with go.work for local development:
# In your go.work:
#   use ./path/to/pi-go/sdk
# In your go.mod:
#   require github.com/chinudotdev/pi-go/sdk v0.0.0
#   replace github.com/chinudotdev/pi-go/sdk => ../path/to/pi-go/sdk
```

## Prerequisites

The SDK needs **one thing** to work: an API key for at least one model provider.

Set it as an environment variable (most common):

```bash
export OPENAI_API_KEY="sk-..."
# or
export ANTHROPIC_API_KEY="sk-ant-..."
# or
export GOOGLE_API_KEY="..."
```

Or write `~/.pi/agent/auth.json`:

```json
{
  "openai": { "apiKey": "sk-..." },
  "anthropic": { "apiKey": "sk-ant-..." }
}
```

That's it. No other setup required.

---

## Example 1: Minimal — One-Shot Prompt

```go
package main

import (
    "context"
    "fmt"
    "os"

    sdk "github.com/chinudotdev/pi-go/sdk"
)

func main() {
    ctx := context.Background()

    // CreateSession with zero config — picks up env vars automatically
    result, err := sdk.CreateSession(ctx, sdk.CreateSessionOptions{
        CWD: ".", // working directory for file tools
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

    // Send a prompt
    msg, err := result.Session.Prompt(ctx, "list the files in the current directory", nil)
    if err != nil {
        fmt.Fprintf(os.Stderr, "prompt failed: %v\n", err)
        os.Exit(1)
    }

    fmt.Println(msg.Content)
}
```

This gives you a fully functional coding agent with 7 tools (read, bash, edit,
write, grep, find, ls), session persistence, and auto-compaction.

---

## Example 2: Streaming Events

For real-time UIs, subscribe to harness events:

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/chinudotdev/pi-go/agent/harness"
    sdk "github.com/chinotdev/pi-go/sdk"
)

func main() {
    ctx := context.Background()

    result, err := sdk.CreateSession(ctx, sdk.CreateSessionOptions{CWD: "."})
    if err != nil {
        panic(err)
    }
    defer result.Session.Dispose(ctx)
    sess := result.Session

    // Subscribe to streaming text
    sess.On("assistantStreamEvent", func(event harness.HarnessEvent) (any, error) {
        if ams, ok := event.Data.(*ai.AssistantMessageEvent); ok {
            if ams.Text != "" {
                fmt.Print(ams.Text) // streaming text
            }
        }
        return nil, nil
    })

    // Run prompt (blocks until complete)
    msg, err := sess.Prompt(ctx, "explain the code in main.go", nil)
    if err != nil {
        panic(err)
    }
    fmt.Println() // newline after streaming
    _ = msg
}
```

---

## Example 3: Custom Model and Tools

```go
result, err := sdk.CreateSession(ctx, sdk.CreateSessionOptions{
    CWD:           "/path/to/project",
    ThinkingLevel: "high",   // extended thinking

    // Restrict to specific tools only
    ToolList: []string{"read", "grep", "find"},

    // Or exclude tools
    // ExcludeTools: []string{"bash"},

    // Override session storage location
    SessionDir: "/tmp/my-app-sessions",
})
```

---

## Example 4: Bring Your Own Auth/Settings/Models

For full control, provide your own dependencies:

```go
// In-memory auth (no files on disk)
authStorage := auth.InMemory(map[string]auth.Credential{
    "openai": {APIKey: "sk-..."},
})

// In-memory settings
settingsMgr := settings.InMemory()
settingsMgr.SetDefaultModel("gpt-4o")
settingsMgr.SetDefaultProvider("openai")

// Registry with custom models.json path
modelReg := models.NewRegistry(authStorage, "/path/to/custom-models.json")

result, err := sdk.CreateSession(ctx, sdk.CreateSessionOptions{
    CWD:           ".",
    AuthStorage:   authStorage,
    SettingsMgr:   settingsMgr,
    ModelRegistry: modelReg,
})
```

---

## Example 5: Interactive Chat Loop

```go
package main

import (
    "bufio"
    "context"
    "fmt"
    "os"
    "strings"

    "github.com/chinudotdev/pi-go/ai"
    sdk "github.com/chinudotdev/pi-go/sdk"
)

func main() {
    ctx := context.Background()

    result, err := sdk.CreateSession(ctx, sdk.CreateSessionOptions{CWD: "."})
    if err != nil {
        panic(err)
    }
    defer result.Session.Dispose(ctx)
    sess := result.Session

    fmt.Printf("Chatting with %s (type 'quit' to exit)\n", sess.Model().Name)
    scanner := bufio.NewScanner(os.Stdin)

    for {
        fmt.Print("\n> ")
        if !scanner.Scan() {
            break
        }
        input := strings.TrimSpace(scanner.Text())
        if input == "quit" || input == "exit" {
            break
        }
        if input == "" {
            continue
        }

        // Special commands
        switch {
        case input == "/model":
            fmt.Printf("Current: %s/%s (thinking: %s)\n",
                sess.Model().Provider, sess.Model().ID, sess.ThinkingLevel())
            continue
        case input == "/next-model":
            res, _ := sess.CycleModel(ctx, "forward")
            if res != nil {
                fmt.Printf("Switched to %s/%s\n", res.Model.Provider, res.Model.ID)
            }
            continue
        case input == "/thinking":
            next := sess.CycleThinkingLevel(ctx)
            fmt.Printf("Thinking level: %s\n", next)
            continue
        case input == "/tools":
            fmt.Printf("Active tools: %v\n", sess.GetActiveToolNames())
            continue
        case input == "/stats":
            stats, _ := sess.GetSessionStats(ctx)
            fmt.Printf("Messages: %d | Tokens in: %d out: %d | Cost: $%.4f\n",
                stats.TotalMessages, stats.InputTokens, stats.OutputTokens, stats.Cost)
            continue
        case input == "/compact":
            fmt.Println("Compacting...")
            _, err := sess.Compact(ctx, "")
            if err != nil {
                fmt.Printf("Compaction failed: %v\n", err)
            } else {
                fmt.Println("Done")
            }
            continue
        }

        // Send prompt
        msg, err := sess.Prompt(ctx, input, nil)
        if err != nil {
            fmt.Printf("Error: %v\n", err)
            continue
        }

        // Extract text from response
        fmt.Println()
        _ = msg
    }
}
```

---

## Example 6: Headless / CI Automation

Use the agent programmatically without human interaction:

```go
package main

import (
    "context"
    "fmt"
    "log"

    sdk "github.com/chinudotdev/pi-go/sdk"
)

func main() {
    ctx := context.Background()

    result, err := sdk.CreateSession(ctx, sdk.CreateSessionOptions{
        CWD:      "/path/to/repo",
        NoTools:  false,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer result.Session.Dispose(ctx)

    // Ask the agent to do work
    msg, err := result.Session.Prompt(ctx,
        "read all .go files and write a summary of the architecture to ARCHITECTURE_SUMMARY.md",
        nil,
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Agent response:", msg.Content)
}
```

---

## Example 7: Multi-Turn with Steering

Interrupt the agent mid-stream with additional context:

```go
go func() {
    time.Sleep(2 * time.Second)
    // Inject a message while the agent is still streaming
    sess.Steer(ctx, "also check the test files", nil)
}()

msg, err := sess.Prompt(ctx, "review the code in main.go", nil)
```

Queue a follow-up for after the current turn:

```go
sess.FollowUp(ctx, "now run the tests", nil)
// Agent will process this automatically after the current turn
```

---

## Configuration

### Environment Variables

| Variable | Purpose |
|---|---|
| `OPENAI_API_KEY` | OpenAI API key |
| `ANTHROPIC_API_KEY` | Anthropic API key |
| `GOOGLE_API_KEY` | Google Gemini API key |
| `PI_CODING_AGENT_DIR` | Override config dir (default `~/.pi/agent/`) |

### File-Based Config

```
~/.pi/agent/
├── auth.json          ← API keys
├── settings.json      ← global settings
├── models.json        ← custom model definitions
└── sessions/          ← conversation history (JSONL)
```

### Settings (JSON)

```json
{
  "defaultProvider": "openai",
  "defaultModel": "gpt-4o",
  "defaultThinkingLevel": "medium",
  "shellCommandPrefix": "",
  "compactionEnabled": true,
  "compactionReserveTokens": 30000,
  "compactionKeepRecentTokens": 10000
}
```

### Project-Level Override

Create `<project>/.pi/settings.json` to override settings per-project.

### Custom Models (models.json)

```json
{
  "my-company": {
    "baseUrl": "https://llm.internal.corp/v1",
    "api": "openai",
    "apiKey": "${VAULT_API_KEY}",
    "models": [
      {
        "id": "codellama-70b",
        "name": "CodeLlama 70B",
        "contextLength": 128000
      }
    ]
  }
}
```

---

## What You Get For Free

When you call `CreateSession()`, the SDK sets up:

| Feature | Details |
|---|---|
| **7 coding tools** | read, bash, edit, write, grep, find, ls |
| **Session persistence** | JSONL append-only log, survives restarts |
| **Auto-compaction** | LLM summarizes old context when it grows too large |
| **Model management** | Cycle models, thinking levels, clamped to model capabilities |
| **Auth resolution** | Env vars → auth.json → models.json → config-value resolution |
| **Settings** | Global + project scopes, deep merge, lazy persistence |
| **Resource discovery** | AGENTS.md, CLAUDE.md, skills, prompt templates |
| **System prompt** | Auto-composed from tools, skills, context files |
| **Streaming** | Full event stream for real-time UIs |
| **Steering** | Mid-stream user input injection |
| **Follow-up** | Auto-queue messages for next turn |

## What You Build

The SDK is **headless** — it has no opinions about UI. You build:

- Terminal chat apps (TUI)
- VS Code / JetBrains extensions
- Web-based coding assistants
- CI/CD automation bots
- Code review tools
- Batch refactoring scripts

## Architecture Quick Reference

```
Your App
  │
  ├── sdk.CreateSession()  →  *AgentSession
  │     │
  │     ├── .Prompt()      →  send message, run agent loop, get response
  │     ├── .Steer()       →  inject mid-stream message
  │     ├── .FollowUp()    →  queue post-turn message
  │     ├── .Compact()     →  manually compress context
  │     ├── .SetModel()    →  switch LLM
  │     ├── .On()          →  subscribe to streaming events
  │     └── .Dispose()     →  cleanup
  │
  │  Internally:
  │     sdk → agent/harness → agent loop → ai/ (LLM streaming)
  │                                  ↓
  │                           tool execution
  │                           (read, bash, edit, ...)
  │
  └── Direct access to sub-packages:
        sdk/auth       — custom auth backends
        sdk/settings   — custom settings storage
        sdk/models     — model registry customization
        sdk/tools      — individual tool configuration
        sdk/resources  — custom resource loading
        sdk/prompt     — custom system prompt composition
```
