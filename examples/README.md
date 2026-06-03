# SDK Examples

Standalone Go programs demonstrating the pi-go SDK. Copy any example into a new project and run it — no need to clone this repo.

## Prerequisites

1. **Go 1.24+**
2. **An API key** — set as an environment variable or in `~/.pi/agent/auth.json`:
   ```bash
   # Option A: Environment variable
   export OPENAI_API_KEY="sk-..."

   # Option B: Auth file (~/.pi/agent/auth.json)
   # {"openai": {"apiKey": "sk-..."}}
   ```

## Quick Start

```bash
# Create a new project
mkdir my-agent && cd my-agent
go mod init my-agent

# Copy an example
cp /path/to/examples/01-minimal/main.go .

# Add the dependency
go mod tidy

# Run
go run main.go
```

## Examples

| # | File | Description |
|---|------|-------------|
| 01 | `01-minimal/` | Simplest usage — one-shot prompt with all defaults |
| 02 | `02-custom-model/` | Select a specific model and thinking level |
| 03 | `03-custom-prompt/` | Replace or append to the system prompt |
| 04 | `04-tools/` | Choose which built-in tools to enable |
| 05 | `05-streaming-events/` | Subscribe to real-time agent events |
| 06 | `06-sessions/` | Multi-turn conversations with session persistence |
| 07 | `07-no-tools/` | Pure LLM chat with no file tools |
| 08 | `08-low-level-agent/` | Use the agent package directly (no SDK) |
| 09 | `09-custom-tool/` | Define and register your own tool |
| 10 | `10-full-control/` | Override everything — no auto-discovery |

## Quick Reference

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/chinudotdev/pi-go/ai"
    "github.com/chinudotdev/pi-go/agent"
    "github.com/chinudotdev/pi-go/agent/harness"
    sdk "github.com/chinudotdev/pi-go/sdk"
    "github.com/chinudotdev/pi-go/sdk/auth"
    "github.com/chinudotdev/pi-go/sdk/models"
    "github.com/chinudotdev/pi-go/sdk/resources"
    "github.com/chinudotdev/pi-go/sdk/settings"
)

func main() {
    ctx := context.Background()

    // Minimal — all defaults, auto-discovers from ~/.pi/agent and cwd
    result, _ := sdk.CreateSession(ctx, sdk.CreateSessionOptions{CWD: "."})
    defer result.Session.Dispose(ctx)
    msg, _ := result.Session.Prompt(ctx, "Hello!", nil)

    // Custom model
    model, _ := ai.GetModel("openai", "gpt-4o")
    result, _ = sdk.CreateSession(ctx, sdk.CreateSessionOptions{
        CWD:           ".",
        Model:         model,
        ThinkingLevel: "high", // off, minimal, low, medium, high
    })

    // Custom prompt
    customPrompt := "You are helpful. Be concise."
    loader := resources.NewLoader(resources.LoaderOptions{
        CWD:                ".",
        NoSkills:           true,
        NoContextFiles:     true,
        NoPrompts:          true,
        SystemPromptSource: &customPrompt,
    })
    result, _ = sdk.CreateSession(ctx, sdk.CreateSessionOptions{
        CWD:            ".",
        ResourceLoader: loader,
    })

    // Read-only tools
    result, _ = sdk.CreateSession(ctx, sdk.CreateSessionOptions{
        CWD:      ".",
        ToolList: []string{"read", "grep", "find", "ls"},
    })

    // No tools (pure chat)
    result, _ = sdk.CreateSession(ctx, sdk.CreateSessionOptions{
        CWD:     ".",
        NoTools: true,
    })

    // Full control — no filesystem discovery
    authStorage := auth.InMemory(nil)
    authStorage.SetRuntimeOverride("openai", os.Getenv("OPENAI_API_KEY"))
    modelReg := models.InMemory(authStorage)
    settingsMgr := settings.InMemory()

    result, _ = sdk.CreateSession(ctx, sdk.CreateSessionOptions{
        CWD:            ".",
        Model:          model,
        AuthStorage:    authStorage,
        ModelRegistry:  modelReg,
        SettingsMgr:    settingsMgr,
        ResourceLoader: loader,
        ToolList:       []string{"read", "bash"},
    })

    // Subscribe to events
    h := result.Session.Harness()
    h.On("before_agent_start", func(event harness.HarnessEvent) (any, error) {
        fmt.Println("Starting...")
        return nil, nil
    })
    h.On(agent.EventToolExecutionStart, func(event harness.HarnessEvent) (any, error) {
        fmt.Printf("Tool: %s\n", event.ToolName)
        return nil, nil
    })
    h.On("settled", func(event harness.HarnessEvent) (any, error) {
        fmt.Println("Done")
        return nil, nil
    })

    // Multi-turn
    result.Session.Prompt(ctx, "Remember my name is Alice.", nil)
    result.Session.Prompt(ctx, "What is my name?", nil)

    // Stats
    stats, _ := result.Session.GetSessionStats(ctx)
    fmt.Printf("Cost: $%.4f\n", stats.Cost)
}
```

## Options

`sdk.CreateSessionOptions` configures session creation:

| Field | Default | Description |
|-------|---------|-------------|
| `CWD` | `os.Getwd()` | Working directory for file tools |
| `AgentDir` | `~/.pi/agent` | Config directory |
| `Model` | From settings / first available | `*ai.Model` to use |
| `ThinkingLevel` | From settings / `"off"` | `off`, `minimal`, `low`, `medium`, `high` |
| `NoTools` | `false` | Disable all tools (pure chat) |
| `ToolList` | `nil` (all built-ins) | Allowlist of tool names: `read`, `bash`, `edit`, `write`, `grep`, `find`, `ls` |
| `ExcludeTools` | `nil` | Denylist of tool names |
| `ResourceLoader` | Auto-discovery | `*resources.Loader` for prompts, skills, context files |
| `AuthStorage` | `~/.pi/agent/auth.json` | `*auth.Storage` for API keys |
| `SettingsMgr` | File-backed | `*settings.Manager` for overrides |
| `ModelRegistry` | Built-in catalog + `models.json` | `*models.Registry` for model lookup |
| `SessionDir` | `~/.pi/agent/sessions` | Session storage directory |

## Events

Subscribe via `session.Harness().On(eventType, handler)`:

```go
h := result.Session.Harness()

h.On("before_agent_start", func(event harness.HarnessEvent) (any, error) {
    fmt.Printf("Prompt: %s\n", event.Prompt)
    return nil, nil
})

h.On(agent.EventToolExecutionStart, func(event harness.HarnessEvent) (any, error) {
    fmt.Printf("Tool started: %s (%s)\n", event.ToolName, event.ToolCallID)
    return nil, nil
})

h.On(agent.EventToolExecutionEnd, func(event harness.HarnessEvent) (any, error) {
    fmt.Printf("Tool done: %s\n", event.ToolName)
    return nil, nil
})

h.On(agent.EventAgentEnd, func(event harness.HarnessEvent) (any, error) {
    fmt.Println("Agent loop complete")
    return nil, nil
})

h.On("settled", func(event harness.HarnessEvent) (any, error) {
    fmt.Println("Harness idle")
    return nil, nil
})
```

### Harness Events

| Event | Constant | Description |
|-------|----------|-------------|
| `"before_agent_start"` | — | Before the agent loop begins. Fields: `Prompt`, `SystemPrompt`, `Resources` |
| `"settled"` | — | Harness returned to idle. Fields: `NextTurnCount` |
| `"save_point"` | — | After a turn, file mutations flushed. Fields: `HadPendingMutations` |
| `"queue_update"` | — | Steer/follow-up queue changed. Fields: `Steer`, `FollowUp`, `NextTurn` |
| `"abort"` | — | Run aborted. Fields: `ClearedSteer`, `ClearedFollowUp` |
| `"model_update"` | — | Model changed. Fields: `Model`, `PreviousModel` |
| `"tools_update"` | — | Tools changed. Fields: `ActiveToolNamesEvt`, `PreviousActiveToolNames` |
| `"thinking_level_update"` | — | Thinking level changed. Fields: `Level`, `PreviousLevel` |
| `"resources_update"` | — | Resources changed. Fields: `Resources`, `PreviousResources` |
| `"session_tree"` | — | Tree navigation. Fields: `NewLeafID`, `OldLeafID` |

### Forwarded Agent Events

Agent events are forwarded with `Type` set to the agent event constant:

| Constant | Value | Description |
|----------|-------|-------------|
| `agent.EventAgentStart` | `"agent_start"` | Agent loop started |
| `agent.EventAgentEnd` | `"agent_end"` | Agent loop ended. Fields: `Messages` |
| `agent.EventTurnStart` | `"turn_start"` | Turn began |
| `agent.EventTurnEnd` | `"turn_end"` | Turn ended |
| `agent.EventMessageStart` | `"message_start"` | Message began |
| `agent.EventMessageUpdate` | `"message_update"` | Streaming delta |
| `agent.EventMessageEnd` | `"message_end"` | Message complete |
| `agent.EventToolExecutionStart` | `"tool_execution_start"` | Tool call started |
| `agent.EventToolExecutionUpdate` | `"tool_execution_update"` | Tool partial result |
| `agent.EventToolExecutionEnd` | `"tool_execution_end"` | Tool call finished |

