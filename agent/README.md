# agent/ — Generic Agentic Loop

Framework-agnostic agent engine. Depends only on `ai/`. Knows nothing about
coding tools, file systems, or authentication.

## Module

```
github.com/chinudotdev/pi-go/agent
```

## Two Sub-Packages

| Package | Purpose |
|---|---|
| `agent` (root) | Core agent loop, tool dispatch, message queues |
| `agent/harness` | Session-aware wrapper: persistence, compaction, tree navigation |

---

## agent/ — Core Engine

### Agent (State Machine)

```go
agent := agent.New(agent.Options{
    StreamFn:     myStreamFn,           // how to call the LLM
    ConvertToLlm: agent.DefaultConvertToLlm,  // message formatting
    // ... hooks
})

agent.SetModel(model)           // set the LLM
agent.SetTools(tools)           // set available tools
agent.SetSystemPrompt(prompt)   // set system prompt

agent.Prompt(ctx, "hello")      // run a turn
agent.Steer(msg)                 // mid-stream message
agent.FollowUp(msg)              // post-stream message
agent.Abort()                    // cancel
agent.WaitForIdle()              // block until done
agent.Subscribe(func(e Event) error { ... })  // listen to events
```

### RunAgentLoop — The Core Loop

```
1. Convert messages → ai.Message[]
2. Build ai.Context { messages, tools, system prompt }
3. Call streamFn(ctx, model, context, opts) → EventStream
4. Accumulate AssistantMessage from events
5. Execute tool calls → collect results → append to messages
6. Check steering queue → inject if present
7. Repeat until no tool calls or stop reason "end_turn"
```

### Key Types

| Type | Purpose |
|---|---|
| `Agent` | State machine: idle ↔ running |
| `Tool` | Name + Parameters + Execute function |
| `Event` | Streaming events: TextDelta, ToolCall, ToolResult, TurnEnd, Error, ... |
| `AgentMessage` | Type alias for `ai.Message` |
| `LoopConfig` | Hooks: BeforeToolCall, AfterToolCall, ShouldStopAfterTurn, PrepareNextTurn |
| `QueueMode` | `QueueAll` or `QueueOneAtATime` for steering/follow-up |

### Agent Methods

| Method | Description |
|---|---|
| `New(opts)` | Create agent |
| `Prompt(ctx, text)` | Run one user turn |
| `PromptMessages(ctx, msgs)` | Run with pre-built messages |
| `Continue(ctx)` | Resume after abort |
| `Steer(msg)` | Queue mid-stream message |
| `FollowUp(msg)` | Queue post-stream message |
| `Abort()` | Cancel current run |
| `WaitForIdle()` | Block until done |
| `Subscribe(fn)` | Listen to events, returns unsubscribe fn |
| `State()` | Snapshot of current state |
| `SetModel/SetTools/SetSystemPrompt/SetThinkingLevel` | Live reconfiguration |
| `SetSteeringMode/SetFollowUpMode` | Queue drain behavior |

---

## agent/harness/ — Session-Aware Harness

Wraps `Agent` with session persistence, phase management, compaction, and tree navigation.

### AgentHarness

```go
harness := harness.NewAgentHarness(harness.HarnessOptions{...}, sessionProvider)

harness.Prompt(ctx, text, images)        → *ai.Message, error
harness.Steer(ctx, text, images)         → error
harness.FollowUp(ctx, text, images)      → error
harness.Compact(ctx, instructions)       → *CompactionResult, error
harness.NavigateTree(ctx, targetID)      → *NavigateTreeResult, error
harness.Abort(ctx)                       → *AbortResult, error

harness.GetModel() / SetModel(ctx, model)
harness.GetThinkingLevel() / SetThinkingLevel(ctx, level)
harness.GetTools() / SetTools(ctx, tools, names)
harness.GetActiveTools() / SetActiveTools(ctx, names)
```

### Phase Machine

```
IDLE → TURN → IDLE
IDLE → COMPACTION → IDLE
IDLE → TREE_NAV → IDLE
```

Only one operation at a time. `requirePhase(IDLE)` guards.

### HarnessOptions (Pluggable Dependencies)

| Field | Type | Purpose |
|---|---|---|
| `Env` | `ExecutionEnv` | Filesystem + shell abstraction |
| `Model` | `*ai.Model` | Initial model |
| `ThinkingLevel` | `string` | Initial thinking level |
| `SystemPrompt` | `string` | Initial system prompt |
| `Tools` | `[]agent.Tool` | All available tools |
| `ActiveToolNames` | `[]string` | Initially enabled tools |
| `GetApiKeyAndHeadersFn` | `func(*ai.Model) (*AuthInfo, error)` | Auth resolution |
| `CompactFn` | `func(ctx, prep, model, ...) (any, error)` | Compaction algorithm |
| `PrepareCompactionFn` | `func(entries, settings) (any, error)` | Prepare for compaction |
| `DefaultCompactionSettingsFn` | `func() any` | Default compaction config |
| `SteeringMode` | `agent.QueueMode` | How steering drains |
| `FollowUpMode` | `agent.QueueMode` | How follow-up drains |

### ExecutionEnv Interface

```go
type ExecutionEnv interface {
    FileSystem           // ReadFile, WriteFile, Stat, Glob, ...
    Shell                // Exec, ...
}
```

Implementations:
- `env.LocalEnv` — real filesystem + shell (production)

### Session (session/)

```go
sess := session.NewSession(jsonlStorage)
sess.BuildContext(ctx)          → full message history
sess.AppendModelChange(ctx, provider, id)
sess.AppendThinkingLevelChange(ctx, level)
```

JSONL-based append-only log. Implements `SessionProvider` interface.

### Compaction (compaction/)

```go
PrepareCompaction(entries, settings) → *CompactionPreparation, error
Compact(ctx, prep, model, apiKey, headers, instructions, tl) → *CompactionResult, error
```

LLM-powered conversation summarization when context grows too large.

### Key Harness Types

| Type | Purpose |
|---|---|
| `AgentHarness` | Main struct — wraps Agent with session + phases |
| `HarnessOptions` | All pluggable dependencies |
| `AuthInfo` | API key + headers for a model |
| `SessionProvider` | Interface for session read/write |
| `CompactionSettings` | Enabled, ReserveTokens, KeepRecentTokens |
| `CompactionPreparation` | Entries to compact, summary instructions |
| `CompactResult` | Pre-compaction result (for UI confirmation) |
| `CompactionResult` | Post-compaction result |
| `SessionTreeEntry` | Node in the session tree (for tree navigation) |
| `HarnessEvent` | Typed event for harness-level subscribers |
