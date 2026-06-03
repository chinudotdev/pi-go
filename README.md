# chinu-ai

Go SDK for building AI-powered agents with multi-turn conversations, tool calling, session persistence, and streaming — provider-agnostic and production-ready.

## Quick Start

### Installation

```bash
go get github.com/chinudotdev/pi-go/agent
```

The `agent` package depends on `github.com/chinudotdev/pi-go/ai`. If working from source:

```
go.work
├── ai/       → github.com/chinudotdev/pi-go/ai
└── agent/    → github.com/chinudotdev/pi-go/agent
```

### Minimal Agent

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/chinudotdev/pi-go/agent"
    "github.com/chinudotdev/pi-go/ai"
    _ "github.com/chinudotdev/pi-go/ai/providers" // registers OpenAI + built-ins
)

func main() {
    // Register an OpenAI-compatible model
    model := &ai.Model{
        ID:       "gpt-4o",
        Provider: "openai",
        API:      "openai-chat-completions",
    }

    ag := agent.New(agent.Options{
        InitialState: &agent.InitialState{
            Model:        model,
            SystemPrompt: "You are a helpful assistant.",
        },
    })

    // Subscribe to events
    ag.Subscribe(func(e agent.Event) error {
        if e.Type == agent.EventMessageEnd && e.Msg.Role == "assistant" {
            fmt.Println("Assistant:", extractText(e.Msg))
        }
        return nil
    })

    // Run a prompt
    err := ag.Prompt(context.Background(), "Hello, what is 2+2?")
    if err != nil {
        log.Fatal(err)
    }
}

func extractText(m ai.Message) string {
    if s, ok := m.Content.(string); ok {
        return s
    }
    return ""
}
```

### Agent with Tools

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/chinudotdev/pi-go/agent"
    "github.com/chinudotdev/pi-go/ai"
)

var weatherTool = &agent.Tool{
    Name:        "get_weather",
    Description: "Get the current weather for a city",
    Parameters: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "city": map[string]any{"type": "string", "description": "City name"},
        },
        "required": []any{"city"},
    },
    Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate agent.ToolUpdateCallback) (*agent.ToolResult, error) {
        city := params["city"].(string)
        return &agent.ToolResult{
            Content: []ai.ContentBlock{
                ai.NewTextContent(fmt.Sprintf("Weather in %s: sunny, 72°F", city)),
            },
        }, nil
    },
}

func main() {
    model := &ai.Model{
        ID:       "gpt-4o",
        Provider: "openai",
        API:      "openai-chat-completions",
    }

    ag := agent.New(agent.Options{
        InitialState: &agent.InitialState{
            Model:        model,
            SystemPrompt: "You are a weather assistant. Use the get_weather tool.",
            Tools:        []*agent.Tool{weatherTool},
        },
    })

    err := ag.Prompt(context.Background(), "What's the weather in Tokyo?")
    if err != nil {
        log.Fatal(err)
    }
}
```

### Session Persistence with Harness

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/chinudotdev/pi-go/agent"
    "github.com/chinudotdev/pi-go/agent/harness"
    "github.com/chinudotdev/pi-go/agent/harness/session"
    "github.com/chinudotdev/pi-go/ai"
)

func main() {
    // Create in-memory session storage
    storage := session.NewInMemorySessionStorage(
        session.SessionMetadata{ID: "my-session"},
        nil,
    )
    sess, _ := session.NewSession(storage)

    model := &ai.Model{
        ID:       "gpt-4o",
        Provider: "openai",
        API:      "openai-chat-completions",
    }

    h := harness.NewAgentHarness(harness.HarnessOptions{
        Model:     model,
        Tools:     []agent.Tool{},
        GetApiKeyAndHeaders: func(m *ai.Model) (*harness.AuthInfo, error) {
            return &harness.AuthInfo{APIKey: "your-api-key"}, nil
        },
    }, sess)

    // Run a prompt (persists to session)
    result, err := h.Prompt(context.Background(), "Hello!", nil)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("Response:", extractText(*result))

    // Continue the conversation
    result, err = h.Prompt(context.Background(), "What did I just say?", nil)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("Response:", extractText(*result))
}

func extractText(m ai.Message) string {
    if s, ok := m.Content.(string); ok {
        return s
    }
    return ""
}
```

### JSONL Session Persistence (disk-backed)

```go
storage, err := session.CreateJsonlSession(ctx, env, sessionPath, "my-session", map[string]any{
    "cwd": "/home/user/project",
})
sess, _ := session.NewSession(storage)

// ... use with harness as above

// Sessions survive across restarts:
storage2, _ := session.OpenJsonlSession(ctx, env, sessionPath)
```

### Custom Stream Function (no real API needed)

```go
// For testing or custom backends
streamFn := func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
    stream := ai.NewEventStream(ctx)
    go func() {
        output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
        stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})
        output.Content = append(output.Content, ai.NewTextContent("custom response"))
        output.StopReason = ai.StopReasonStop
        output.Timestamp = time.Now().UnixMilli()
        stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
        stream.End(output)
    }()
    return stream, nil
}

ag := agent.New(agent.Options{
    StreamFn: streamFn,
    // ...
})
```

## Architecture

```
┌─────────────────────────────────────────────────┐
│  Your Application                               │
│  ┌────────────┐  ┌────────────────────────────┐ │
│  │  agent.Agnt│  │  harness.AgentHarness      │ │
│  │            │  │  ┌──────────────────────┐   │ │
│  │  Prompt()  │──│──│  agent loop          │   │ │
│  │  Continue()│  │  │  tool execution      │   │ │
│  │  Subscribe │  │  │  steering/follow-up  │   │ │
│  │  Steer()   │  │  └──────────────────────┘   │ │
│  └────────────┘  │  ┌──────────────────────┐   │ │
│                  │  │  session persistence │   │ │
│                  │  │  compaction          │   │ │
│                  │  │  skills/templates    │   │ │
│                  │  └──────────────────────┘   │ │
│                  └────────────────────────────┘ │
│  ┌────────────────────────────────────────────┐ │
│  │  ai — Provider Registry                    │ │
│  │  ┌─────────┐ ┌──────────┐ ┌─────────────┐ │ │
│  │  │ OpenAI  │ │ Anthropic│ │ Custom/Faux │ │ │
│  │  │ (done)  │ │ (stub)   │ │ (any)       │ │ │
│  │  └─────────┘ └──────────┘ └─────────────┘ │ │
│  └────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────┘
```

## Package Overview

| Package | Description |
|---------|-------------|
| `ai` | Core types, streaming, provider registry, models |
| `ai/providers` | Provider implementations (OpenAI complete, others as stubs) |
| `agent` | Agent loop, tool execution, lifecycle management |
| `agent/harness` | Multi-turn orchestrator with sessions, compaction, hooks |
| `agent/harness/session` | Session persistence (in-memory + JSONL file-backed) |
| `agent/harness/compaction` | Auto-compaction and branch summarization |
| `agent/harness/env` | Local filesystem and shell execution environment |

## Key Features

- **Provider-agnostic** — swap models/providers without changing agent code
- **Streaming-first** — SSE streaming via `ai.EventStream`
- **Tool calling** — sequential or parallel execution, with before/after hooks
- **Argument validation** — JSON Schema validation with automatic type coercion
- **Session persistence** — in-memory or JSONL file-backed, with tree navigation
- **Auto-compaction** — context window management with summarization
- **Skills** — load SKILL.md files from filesystem
- **Prompt templates** — parameterized prompts with shell-style arg substitution
- **Steering** — inject messages mid-run to guide the agent
- **Follow-up queues** — queue messages for processing after the agent would stop
- **Abort support** — cancel runs via `context.Context`

## Providers

| Provider | API Value | Status |
|----------|-----------|--------|
| OpenAI / compatible | `openai-chat-completions` | ✅ Fully implemented |
| Anthropic | `anthropic-messages` | 🟡 Stub |
| OpenAI Responses | `openai-responses` | 🟡 Stub |
| OpenAI Codex | `openai-codex-responses` | 🟡 Stub |
| Azure OpenAI | `azure-openai-responses` | 🟡 Stub |
| Google | `google` | 🟡 Stub |
| Google Vertex | `google-vertex` | 🟡 Stub |
| Amazon Bedrock | `amazon-bedrock` | 🟡 Stub |
| Mistral | `mistral` | 🟡 Stub |
| Faux (testing) | `faux` | ✅ Testing mock |

### Using OpenAI-Compatible Providers

DeepSeek, xAI, OpenRouter, Together, and any OpenAI-compatible API work with the existing provider:

```go
model := &ai.Model{
    ID:       "deepseek-chat",
    Provider: "deepseek",
    API:      "openai-chat-completions",
    BaseURL:  "https://api.deepseek.com/v1",
}

// Set your API key via OPENAI_API_KEY env or GetApiKeyAndHeaders
```

### Registering a Custom Provider

```go
ai.RegisterApiProvider(ai.ApiProvider{
    API: "my-custom-api",
    Stream: func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.StreamOptions) (*ai.EventStream, error) {
        // Your streaming implementation
    },
    StreamSimple: func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.StreamOptions) (*ai.EventStream, error) {
        // Same or simplified streaming
    },
})
```

## License

[MIT](LICENSE) © chinudotdev
