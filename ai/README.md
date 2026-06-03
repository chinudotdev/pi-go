# ai/ — LLM Provider Abstraction

Zero-dependency (except openai-go) package for interacting with language models.

## Module

```
github.com/chinudotdev/pi-go/ai
```

## Core Types

| Type | Purpose |
|---|---|
| `Model` | Provider, ID, Name, API, BaseURL, cost info, thinking support |
| `Message` | Role + Content + Details + Timestamp — the universal message unit |
| `ContentBlock` | Polymorphic: text, tool_call, tool_result, image, thinking |
| `Context` | Messages + Tools + System prompt + MaxTokens — sent to LLM |
| `AssistantMessage` | Streaming accumulator (builds incrementally from SSE events) |
| `EventStream` | Channel of `AssistantMessageEvent` — the streaming interface |
| `Usage` | Input/Output/CacheRead/CacheWrite tokens + Cost |
| `ApiProvider` | Registered LLM backend with Stream + Complete functions |

## Key Functions

### Streaming & Completion

```go
Stream(ctx, model, context, opts)      → *EventStream, error    // raw streaming
Complete(ctx, model, context, opts)     → *AssistantMessage, error  // block until done
StreamSimple(ctx, model, context, opts) → *EventStream, error    // simplified opts
CompleteSimple(ctx, model, context, opts) → *AssistantMessage, error
```

### Model Catalog

```go
GetProviders()              → []string              // "openai", "anthropic", "google", ...
GetModels(provider)         → []*Model              // full catalog
GetModel(provider, id)      → *Model, bool           // specific model
RegisterModel(model)                                 // add dynamically
CalculateCost(model, usage) → Cost                   // compute $
ClampThinkingLevel(model, level) → ModelThinkingLevel // enforce model limits
```

### API Provider Registry

```go
RegisterApiProvider(ApiProvider, sourceID)   // register a backend
GetApiProvider(api)     → *ApiProvider, bool  // lookup by API string
ResolveApiProvider(api) → *ApiProvider, error // lookup or error
```

### Auth

```go
ResolveAPIKey(model, opts) → string, error   // from opts or env vars
GetEnvApiKey(provider)     → string, bool     // check env vars
FindEnvKeys(provider)      → []string         // possible env var names
```

## ContentBlock Factories

```go
NewTextContent(text)              → ContentBlock{Type: "text"}
NewThinkingContent(thinking)      → ContentBlock{Type: "thinking"}
NewToolCallContent(id, name, args) → ContentBlock{Type: "toolCall"}
content.AsText()                  → (string, bool)
content.AsThinking()              → (string, *string, bool, bool)
content.AsToolCall()              → (string, string, map[string]any, bool)
```

## Architecture

```
Stream(ctx, model, context, opts)
  │
  ├── ResolveApiProvider(model.API)  → find registered provider
  ├── ResolveAPIKey(model, opts)     → get API key
  └── provider.Stream(ctx, ...)      → HTTP POST, parse SSE → EventStream
```

## Currently Registered Providers

- **openai-compat** — covers OpenAI, Groq, Together, OpenRouter, any OpenAI-compatible endpoint

## TODO

- Anthropic native API (different message format, thinking blocks)
- Google Gemini
- AWS Bedrock (IAM auth)
- Azure OpenAI
- Ollama (local models)
