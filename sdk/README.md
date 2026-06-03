# sdk/ — Coding Agent SDK

Opinionated SDK that wires `agent/` + `ai/` into a complete coding agent session.
Owns the **which** (which tools, which auth backend, which settings) while
`agent/` owns the **how** (how to run the loop, how to stream, how to dispatch tools).

## Module

```
github.com/chinudotdev/pi-go/sdk
```

## Depends on

- `github.com/chinudotdev/pi-go/agent`
- `github.com/chinudotdev/pi-go/ai`

## Quick Start

```go
result, err := sdk.CreateSession(ctx, sdk.CreateSessionOptions{
    CWD:      "/path/to/project",
    AgentDir: config.GetAgentDir(),  // ~/.pi/agent/
})
if err != nil { ... }

session := result.Session

// Send a prompt
msg, err := session.Prompt(ctx, "fix the bug in main.go", nil)

// Model management
session.SetModel(ctx, newModel)
session.CycleModel(ctx, "forward")

// Thinking level
session.SetThinkingLevel(ctx, "high")

// Compaction
session.Compact(ctx, "keep the API discussion")

// Stats
stats, _ := session.GetSessionStats(ctx)
```

## Package Layout

```
sdk/
├── sdk.go                    ← CreateSession() + AgentSession
├── auth/storage.go           ← API key storage (file, memory backends)
├── settings/manager.go       ← Global + project settings (30+ config keys)
├── models/
│   ├── registry.go           ← Model registry (built-in + custom models.json)
│   └── resolver.go           ← Model lookup, pattern matching, session restore
├── tools/
│   ├── definitions.go        ← Tool JSON schemas
│   ├── read.go               ← File reading tool
│   ├── bash.go               ← Shell execution tool
│   ├── edit.go               ← File editing tool (LCS-based diff)
│   ├── write.go              ← File writing tool
│   ├── grep.go               ← Content search (ripgrep or Go regex)
│   ├── find.go               ← File finding (fd or filepath.WalkDir)
│   ├── ls.go                 ← Directory listing tool
│   ├── truncate.go           ← Output truncation (~10K lines / 50KB)
│   ├── output.go             ← Streaming output accumulator
│   ├── filequeue.go          ← File mutation queue (serialize edits)
│   └── path_utils.go         ← Allowed dir checks, path resolution
├── resources/loader.go       ← Discover skills, prompts, context files, system prompt
├── prompt/system.go          ← Build system prompt from parts
├── skills/skills.go          ← Load skills (YAML frontmatter), format for prompt
├── messages/messages.go      ← ConvertToLlm, message factories
├── config/
│   ├── paths.go              ← ~/.pi/agent/ paths
│   └── defaults.go           ← Default config values
├── eventbus/eventbus.go      ← Pub/sub event bus
├── sourceinfo/sourceinfo.go  ← Message source tracking
└── internal/
    ├── configvalue/resolve.go ← Resolve $ENV, default, override values
    ├── jsonutil/json.go       ← Safe JSON parsing
    ├── paths/paths.go         ← Walk parent directories
    └── shell/shell.go         ← Detect user shell
```

## Key Types

### SDK Entry Point

| Type | Purpose |
|---|---|
| `AgentSession` | Main session object — wraps harness with SDK features |
| `CreateSessionOptions` | Configure session creation (model, tools, auth, settings) |
| `CreateSessionResult` | Session + fallback message if no model available |
| `SessionStats` | Token counts, message counts, cost |

### Auth (auth/)

| Type | Purpose |
|---|---|
| `Storage` | API key storage with runtime overrides |
| `FileBackend` | Reads/writes `~/.pi/agent/auth.json` |
| `MemoryBackend` | In-memory for testing |

### Settings (settings/)

| Type | Purpose |
|---|---|
| `Manager` | 30+ typed getters/setters for all config |
| | Global scope (`~/.pi/agent/settings.json`) |
| | Project scope (`<cwd>/.pi/settings.json`) |
| | Deep merge: project overrides global |

### Models (models/)

| Type | Purpose |
|---|---|
| `Registry` | Built-in models from `ai/` + custom `models.json` |
| | `GetAvailable()` — models with configured auth |
| | `Find(provider, id)` — lookup by provider + ID |
| | `GetAPIKeyAndHeaders(model)` — auth resolution |
| | `RegisterProvider()` — dynamic provider registration |

### Tools (tools/)

All 7 tools implement `agent.Tool`:

| Tool | Operations Interface | Key Feature |
|---|---|---|
| `read` | `ReadOperations` | Multi-range reads, image support |
| `bash` | `BashOperations` | Streaming output, timeout, working dir |
| `edit` | `EditOperations` | LCS-based diff, fuzzy matching |
| `write` | `WriteOperations` | Creates parent dirs |
| `grep` | `GrepOperations` | Prefers ripgrep, falls back to Go regex |
| `find` | `FindOperations` | Prefers fd, falls back to filepath.WalkDir |
| `ls` | `ListOperations` | Dir listing with file type indicators |

`CreateAllTools(cwd, opts)` → `map[ToolName]*agent.Tool`

### Resources (resources/)

`Loader.Load()` → `LoadedResources`:
- Skills (from `~/.pi/agent/skills/` and `<cwd>/.agents/skills/`)
- Prompt templates (YAML frontmatter)
- Context files (AGENTS.md, CLAUDE.md — walks CWD → root)
- System prompt (SYSTEM.md or default)
- Append system prompt (from filesystem)

### Prompt (prompt/)

`BuildSystemPrompt(opts)` composes from:
1. Custom or default system prompt
2. Context files
3. Skills formatted for LLM
4. Tool snippets
5. Guidelines / append sections

Also: `ParseCommandArgs()`, `SubstituteArgs()`, `ExpandPromptTemplate()`

## How It Wires agent/ and ai/

The SDK injects behavior into the agent layer:

```
sdk.CreateSession()
  │
  ├── HarnessOptions.GetApiKeyAndHeadersFn  → models.Registry.GetAPIKeyAndHeaders()
  ├── HarnessOptions.CompactFn              → compaction.Compact()
  ├── HarnessOptions.PrepareCompactionFn    → compaction.PrepareCompaction()
  ├── agent.Options.StreamFn (default)      → ai.StreamSimple()
  ├── agent.Options.ConvertToLlm (default)  → agent.DefaultConvertToLlm()
  ├── []agent.Tool                          → tools.CreateAllTools()
  ├── system prompt                         → prompt.BuildSystemPrompt()
  └── ExecutionEnv                          → env.NewLocalEnv(cwd)
```

## File Locations

```
~/.pi/agent/
├── auth.json            ← API keys (auth storage)
├── settings.json        ← global settings
├── models.json          ← custom model definitions (optional)
├── sessions/            ← JSONL session logs
├── prompts/             ← custom prompt templates
├── skills/              ← user-installed skills
└── bin/                 ← binary tools

<project>/.pi/
└── settings.json        ← project-specific settings (overrides global)

<project>/.agents/
└── skills/              ← project-local skills

<project>/AGENTS.md      ← project context (walked up to git root)
<project>/CLAUDE.md      ← alternative context file
```

## Environment Variables

| Variable | Purpose |
|---|---|
| `PI_CODING_AGENT_DIR` | Override `~/.pi/agent/` base directory |
| `<PROVIDER>_API_KEY` | API key for a provider (e.g. `OPENAI_API_KEY`) |
