# Architecture: Bottom-Up (ai → agent → sdk)

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              CONSUMER / MODE LAYER                              │
│         (CLI, TUI, RPC server, editor integrations — NOT YET PORTED)            │
│                                                                                 │
│    Uses: sdk.CreateSession() → *AgentSession → .Prompt(), .Steer(), etc.        │
└──────────────────────────────────────┬──────────────────────────────────────────┘
                                       │
                                       ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              SDK LAYER  (sdk/)                                  │
│                         module: github.com/chinudotdev/pi-go/sdk                │
│                              depends on: agent, ai                              │
│                                                                                 │
│  ┌───────────────────────────────────────────────────────────────────────────┐  │
│  │ sdk.go — Entry Point                                                      │  │
│  │                                                                           │  │
│  │  CreateSession(ctx, opts) → { *AgentSession, fallbackMsg }               │  │
│  │      1. Init auth, settings, model registry                              │  │
│  │      2. Load resources (skills, prompts, context files)                  │  │
│  │      3. Resolve model + thinking level                                   │  │
│  │      4. Build tool set (allowlist/denylist)                              │  │
│  │      5. Compose system prompt                                            │  │
│  │      6. Create session storage (JSONL)                                   │  │
│  │      7. Wire up harness with all dependencies                            │  │
│  │      8. Return AgentSession                                              │  │
│  │                                                                           │  │
│  │  AgentSession wraps AgentHarness:                                        │  │
│  │      Prompt(), Steer(), FollowUp()  → delegate to harness                │  │
│  │      SetModel(), CycleModel()      → model management                    │  │
│  │      SetThinkingLevel(), Cycle()   → thinking level management           │  │
│  │      Compact()                     → context compaction                  │  │
│  │      GetSessionStats()             → usage/token statistics              │  │
│  └───────────────────────────────────────────────────────────────────────────┘  │
│                                                                                 │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐                  │
│  │   sdk/auth/      │  │  sdk/settings/   │  │  sdk/models/    │                  │
│  │                  │  │                  │  │                 │                  │
│  │  Storage         │  │  Manager         │  │  Registry       │                  │
│  │  FileBackend     │  │  Get/Set 30+    │  │  GetAvailable() │                  │
│  │  MemoryBackend   │  │   config keys   │  │  Find()         │                  │
│  │                  │  │  Global+Project  │  │  GetAPIKey..()  │                  │
│  │  ~/.pi/auth.json │  │  scopes          │  │  Resolver       │                  │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘                  │
│                                                                                 │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐                  │
│  │  sdk/resources/  │  │  sdk/prompt/     │  │  sdk/skills/    │                  │
│  │                  │  │                  │  │                 │                  │
│  │  Loader          │  │  BuildSystem     │  │  LoadSkills()   │                  │
│  │   .Load() →      │  │   Prompt()       │  │  YAML front-    │                  │
│  │   LoadedResources│  │  ParseArgs()     │  │   matter        │                  │
│  │                  │  │  SubstituteArgs() │  │  FormatFor      │                  │
│  │  Discovers:      │  │  ExpandTemplate() │  │   Prompt()      │                  │
│  │   - AGENTS.md    │  │                  │  │                 │                  │
│  │   - CLAUDE.md    │  │  Composes:       │  │  <cwd>/.agents/ │                  │
│  │   - skills/      │  │   system prompt  │  │   skills/       │                  │
│  │   - prompts/     │  │   from parts     │  │  ~/.pi/agent/   │                  │
│  │   - SYSTEM.md    │  │                  │  │   skills/       │                  │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘                  │
│                                                                                 │
│  ┌───────────────────────────────────────────────────────────────────────────┐  │
│  │ sdk/tools/ — 7 Coding Agent Tools                                         │  │
│  │                                                                           │  │
│  │  CreateAllTools(cwd, opts) → map[ToolName]*agent.Tool                    │  │
│  │                                                                           │  │
│  │  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐        │  │
│  │  │ Read │ │ Bash │ │ Edit │ │Write │ │ Grep │ │ Find │ │  Ls  │        │  │
│  │  └──────┘ └──────┘ └──────┘ └──────┘ └──────┘ └──────┘ └──────┘        │  │
│  │                                                                           │  │
│  │  Shared infrastructure:                                                    │  │
│  │    OutputAccumulator (streaming bash output, temp file spillover)         │  │
│  │    FileMutationQueue  (serialize file edits across tools)                  │  │
│  │    Truncator          (enforce output limits, ~10K lines/50KB)            │  │
│  │    PathUtils          (allowed dir checks, relative path resolution)       │  │
│  └───────────────────────────────────────────────────────────────────────────┘  │
│                                                                                 │
│  ┌─────────────────┐  ┌─────────────────────────────┐                          │
│  │  sdk/messages/   │  │  sdk/config/ + internal/     │                          │
│  │                  │  │                              │                          │
│  │  ConvertToLlm()  │  │  paths.go    ~/.pi/agent/   │                          │
│  │  Factories       │  │  defaults.go                 │                          │
│  │  FormatBash()    │  │  configvalue/ resolve $ENV  │                          │
│  └─────────────────┘  │  jsonutil/  safe JSON        │                          │
│                        │  paths/     find in parents  │                          │
│                        │  shell/     detect shell     │                          │
│                        │  sourceinfo/ message source  │                          │
│                        └─────────────────────────────┘                          │
└──────────────────────────────────────┬──────────────────────────────────────────┘
                                       │
                                       │  sdk imports agent + ai
                                       │  tools are agent.Tool structs
                                       ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           AGENT LAYER  (agent/)                                 │
│                    module: github.com/chinudotdev/pi-go/agent                    │
│                            depends on: ai                                       │
│                                                                                 │
│  ┌───────────────────────────────────────────────────────────────────────────┐  │
│  │ agent/ — Core Agent Engine                                                 │  │
│  │                                                                           │  │
│  │  Agent (state machine)                                                    │  │
│  │    Prompt(ctx, text)           → run agent loop                           │  │
│  │    Steer(msg)                  → queue mid-stream message                 │  │
│  │    FollowUp(msg)               → queue post-stream message                │  │
│  │    Abort()                     → cancel current run                       │  │
│  │    WaitForIdle()               → block until done                         │  │
│  │    Subscribe(listener)         → event stream                             │  │
│  │    SetModel/SetTools/..        → mutate config                            │  │
│  │                                                                           │  │
│  │  RunAgentLoop()               → the main loop:                            │  │
│  │    1. Convert messages → ai.Message[] (via ConvertToLlm)                  │  │
│  │    2. Call streamFn(model, context, opts) → EventStream                   │  │
│  │    3. Accumulate AssistantMessage from events                             │  │
│  │    4. Execute tool calls → collect results                                │  │
│  │    5. Check steering queue → inject or continue                           │  │
│  │    6. Repeat until no more tool calls or stop reason                      │  │
│  │                                                                           │  │
│  │  Key types:                                                               │  │
│  │    AgentMessage  = ai.Message      (unified message type)                 │  │
│  │    Tool           { Name, Execute, Parameters, ... }                      │  │
│  │    Event          { Type, Message, ToolCall, ... }                        │  │
│  │    LoopConfig     { hooks: BeforeToolCall, AfterToolCall,                 │  │
│  │                      ShouldStopAfterTurn, PrepareNextTurn }               │  │
│  └───────────────────────────────────────────────────────────────────────────┘  │
│                                                                                 │
│  ┌───────────────────────────────────────────────────────────────────────────┐  │
│  │ agent/harness/ — Session-Aware Harness                                     │  │
│  │                                                                           │  │
│  │  AgentHarness                                                              │  │
│  │    Wraps Agent with: session persistence, phase management,               │  │
│  │    system prompt composition, compaction, tree navigation                 │  │
│  │                                                                           │  │
│  │    Phase machine:  IDLE → TURN → (COMPACT) → IDLE                         │  │
│  │                                                                           │  │
│  │    Prompt(ctx, text, images)    → full turn cycle                         │  │
│  │    Compact(ctx, instructions)   → compress conversation                   │  │
│  │    NavigateTree(ctx, targetID)  → branch navigation                       │  │
│  │    SetModel/SetTools/..         → live reconfiguration                    │  │
│  │                                                                           │  │
│  │  HarnessOptions (pluggable deps):                                         │  │
│  │    GetApiKeyAndHeadersFn  → resolve auth per model                        │  │
│  │    CompactFn              → compaction algorithm                          │  │
│  │    PrepareCompactionFn    → prepare entries for compaction                │  │
│  │    SessionProvider        → session read/write interface                  │  │
│  │    ExecutionEnv           → filesystem + shell abstraction                │  │
│  │                                                                           │  │
│  │  Session (session/)                                                       │  │
│  │    JSONL-based append-only log                                            │  │
│  │    BuildContext(ctx) → full message history                               │  │
│  │    AppendModelChange, AppendCompaction, AppendUserMessage, ...            │  │
│  │                                                                           │  │
│  │  Compaction (compaction/)                                                 │  │
│  │    PrepareCompaction()  → identify entries to compress                    │  │
│  │    Compact()            → LLM-powered summarization                       │  │
│  │    Branch summarization → merge divergent branches                        │  │
│  │                                                                           │  │
│  │  Environment (env/)                                                       │  │
│  │    LocalEnv  → real filesystem + shell (production)                       │  │
│  │    ExecutionEnv interface → abstracted for testing                        │  │
│  └───────────────────────────────────────────────────────────────────────────┘  │
│                                                                                 │
│  ┌───────────────────────────────────────────────────────────────────────────┐  │
│  │ agent/proxy.go — Remote Proxy Streaming                                    │  │
│  │  StreamProxy() → forward requests to remote agent server                  │  │
│  │  Used by RPC mode (not yet ported)                                        │  │
│  └───────────────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────┬──────────────────────────────────────────┘
                                       │
                                       │  agent imports ai
                                       │  agent.StreamFn = ai.Stream signature
                                       ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              AI LAYER  (ai/)                                    │
│                      module: github.com/chinudotdev/pi-go/ai                    │
│                           depends on: nothing (only openai-go)                  │
│                                                                                 │
│  ┌───────────────────────────────────────────────────────────────────────────┐  │
│  │ Core Types                                                                 │  │
│  │                                                                           │  │
│  │  Model         { Provider, ID, Name, API, BaseURL, Cost, ... }           │  │
│  │  Message       { Role, Content, Details, Timestamp }                     │  │
│  │  ContentBlock  { Type, Text, ToolCall, Image, ... }                      │  │
│  │  Context       { Messages, Tools, System, MaxTokens }                    │  │
│  │  Usage         { Input, Output, CacheRead, CacheWrite, Cost }            │  │
│  │  AssistantMessage  (streaming accumulator)                                │  │
│  │  EventStream       (channel of AssistantMessageEvent)                    │  │
│  └───────────────────────────────────────────────────────────────────────────┘  │
│                                                                                 │
│  ┌───────────────────────────────────────────────────────────────────────────┐  │
│  │ Streaming / Completion                                                     │  │
│  │                                                                           │  │
│  │  Stream(ctx, model, context, opts)  → EventStream                        │  │
│  │  Complete(ctx, model, context, opts) → AssistantMessage                   │  │
│  │  StreamSimple(ctx, model, context, opts)  → EventStream                  │  │
│  │  CompleteSimple(ctx, model, context, opts) → AssistantMessage             │  │
│  │                                                                           │  │
│  │  Internally:                                                              │  │
│  │    1. ResolveApiProvider(model.API) → find registered provider            │  │
│  │    2. ResolveAPIKey(model, opts)    → env vars or provided key            │  │
│  │    3. Call provider.Stream(ctx, model, context, opts)                     │  │
│  │    4. Return EventStream (typed channel)                                  │  │
│  └───────────────────────────────────────────────────────────────────────────┘  │
│                                                                                 │
│  ┌───────────────────────────────────────────────────────────────────────────┐  │
│  │ API Provider Registry                                                      │  │
│  │                                                                           │  │
│  │  RegisterApiProvider(ApiProvider, sourceID)                               │  │
│  │  GetApiProvider(api)  → provider with Stream + Complete fns              │  │
│  │                                                                           │  │
│  │  ApiProvider {                                                            │  │
│  │    API          string           // "openai", "anthropic", etc.          │  │
│  │    Stream       StreamFunction   // (ctx, model, ctx, opts) → stream    │  │
│  │    Complete     CompleteFunction // (ctx, model, ctx, opts) → msg       │  │
│  │    Models       ModelsJSON       // provider-specific model catalog      │  │
│  │  }                                                                        │  │
│  │                                                                           │  │
│  │  Currently registered: openai-compat (covers OpenAI, Groq, etc.)         │  │
│  │  TODO: anthropic-native, gemini, bedrock, azure, ollama                  │  │
│  └───────────────────────────────────────────────────────────────────────────┘  │
│                                                                                 │
│  ┌───────────────────────────────────────────────────────────────────────────┐  │
│  │ Model Catalog                                                              │  │
│  │                                                                           │  │
│  │  Embedded models.json (from pi/packages/ai/models.json)                   │  │
│  │  GetProviders()      → []string  {"openai","anthropic","google",...}     │  │
│  │  GetModels(provider) → []*Model  (with costs, limits, thinking support)  │  │
│  │  GetModel(p, id)     → *Model                                             │  │
│  │  RegisterModel()     → add dynamically                                   │  │
│  │  CalculateCost()     → compute $ from usage                              │  │
│  │  ClampThinkingLevel() → enforce model limits                             │  │
│  └───────────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

## Data Flow: A Single Prompt

```
User types: "fix the bug in main.go"
        │
        ▼
┌─── SDK: AgentSession.Prompt(ctx, "fix the bug in main.go", nil) ───┐
│                                                                      │
│  1. Expand prompt template (if slash command)                        │
│  2. Call harness.Prompt(ctx, expanded, images)                       │
│                                                                      │
└──────────────────────────┬───────────────────────────────────────────┘
                           │
                           ▼
┌─── AGENT/HARNESS: AgentHarness.Prompt() ────────────────────────────┐
│                                                                      │
│  1. Phase check (must be IDLE)                                       │
│  2. Set phase → TURN                                                 │
│  3. Build user message (ai.Message with images)                      │
│  4. Append to session (JSONL)                                        │
│  5. Call agent.PromptMessages(ctx, messages)                         │
│     └── or queue if agent busy                                       │
│  6. Wait for completion                                              │
│  7. Set phase → IDLE                                                 │
│  8. Return final ai.Message                                          │
│                                                                      │
└──────────────────────────┬───────────────────────────────────────────┘
                           │
                           ▼
┌─── AGENT: RunAgentLoop() ──────────────────────────────────────────┐
│                                                                      │
│  LOOP:                                                               │
│  1. Convert messages → ai.Message[] (DefaultConvertToLlm)           │
│  2. Build ai.Context { messages, tools, system prompt }             │
│  3. Call streamFn(ctx, model, context, opts)                        │
│     │                                                                │
│     ▼                                                                │
│  ┌─── AI: Stream(ctx, model, context, opts) ─────────────────┐      │
│  │                                                            │      │
│  │  1. ResolveApiProvider(model.API) → provider              │      │
│  │  2. ResolveAPIKey(model, opts) → api key                  │      │
│  │  3. provider.Stream(ctx, model, context, opts)            │      │
│  │     └── HTTP POST to model.BaseURL                        │      │
│  │  4. Parse SSE events → AssistantMessageEvent channel      │      │
│  │                                                            │      │
│  └────────────────────────────────────────────────────────────┘      │
│     │                                                                │
│     ▼ (stream of AssistantMessageEvents)                             │
│  4. Accumulate into AssistantMessage                                 │
│     - text blocks, tool calls, thinking blocks                       │
│  5. Emit events to subscribers (streaming text, tool calls)          │
│                                                                      │
│  IF tool calls in response:                                          │
│  6. For each tool call:                                              │
│     a. Find tool by name in tool registry                            │
│     b. Execute tool.Execute(ctx, params)                             │
│        └── e.g. Read tool reads file, Bash tool runs command         │
│     c. Collect ToolResult                                            │
│     d. Append tool result to messages                                │
│  7. Check steering queue → inject user steering if present           │
│  8. Go to LOOP                                                       │
│                                                                      │
│  IF no tool calls (stop reason "end_turn"):                          │
│  9. Return final assistant message                                   │
│                                                                      │
└──────────────────────────┬───────────────────────────────────────────┘
                           │
                           ▼
┌─── AGENT/HARNESS: Post-turn ────────────────────────────────────────┐
│                                                                      │
│  1. Append assistant message to session (JSONL)                      │
│  2. Check compaction threshold                                       │
│     └── if exceeded: Compact() → LLM summarizes old messages        │
│  3. Return ai.Message to SDK                                         │
│                                                                      │
└──────────────────────────┬───────────────────────────────────────────┘
                           │
                           ▼
┌─── SDK: Return to caller ───────────────────────────────────────────┐
│                                                                      │
│  AgentSession.Prompt() returns (*ai.Message, error)                  │
│  Consumer reads: message.Content, message.Role, usage stats          │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

## Dependency Graph (module level)

```
    ┌─────┐
    │ ai  │    ← zero external deps (only openai-go for compat)
    │     │    ← types: Model, Message, ContentBlock, Context, EventStream
    └──┬──┘    ← functions: Stream(), Complete(), GetModel(), CalculateCost()
       │
       ▼
    ┌─────┐
    │agent│    ← depends on: ai
    │     │    ← Agent (state machine), RunAgentLoop()
    └──┬──┘    ← Harness (session, compaction, tree nav)
       │       ← Tool execution, message conversion
       ▼
    ┌─────┐
    │ sdk │    ← depends on: agent, ai
    │     │    ← CreateSession(), AgentSession
    └─────┘    ← auth, settings, models, tools, resources, prompts, skills
                ← self-contained: knows nothing about modes/CLI/TUI
```

## What Each Layer Owns

| Layer | Owns | Does NOT own |
|---|---|---|
| **ai/** | Model types, streaming protocol, API provider registry, cost calculation, thinking levels | Tool execution, session state, conversation logic |
| **agent/** | Agent loop, tool dispatch, message queues, steering/follow-up, harness lifecycle, session persistence, compaction | Which tools exist, auth storage, user settings |
| **sdk/** | Tool definitions (read/bash/edit/write/grep/find/ls), auth backends, settings manager, model registry, resource discovery, prompt composition | Streaming protocol, agent loop internals |

## Key Insight: Inversion Points

The SDK injects behavior **into** the agent layer via function fields:

```
sdk.CreateSession()
  │
  ├── HarnessOptions.GetApiKeyAndHeadersFn  → how to resolve API keys
  ├── HarnessOptions.CompactFn              → how to compact (calls compaction pkg)
  ├── HarnessOptions.PrepareCompactionFn    → how to prepare compaction
  ├── agent.Options.StreamFn                → how to call LLM (defaults to ai.StreamSimple)
  ├── agent.Options.ConvertToLlm            → how to format messages
  ├── []agent.Tool                          → which tools are available
  └── system prompt                         → composed from resources
```

This means:
- **agent/** is a generic agentic loop — it doesn't know about coding tools
- **sdk/** is the coding-specific configuration — it wires up file tools, auth, settings
- **ai/** is a generic LLM client — it doesn't know about agents or tools
