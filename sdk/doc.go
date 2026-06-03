// Package sdk provides the top-level entry point for the pi coding agent SDK.
//
// It wires together authentication, settings, model registry, resource loading,
// tools, and the agent harness into a complete coding agent session.
//
// # Quick Start
//
//	result, err := sdk.CreateSession(ctx, sdk.CreateSessionOptions{
//	    CWD:      "/path/to/project",
//	    AgentDir: config.GetAgentDir(),
//	})
//	session := result.Session
//	msg, err := session.Prompt(ctx, "fix the bug in main.go", nil)
//
// # Architecture
//
// The SDK sits above two lower-level packages:
//
//	ai/     → LLM provider abstraction (streaming, models, costs)
//	agent/  → Generic agentic loop (tool dispatch, message queues, session)
//
// The SDK is the "opinionated" layer that decides:
//   - Which tools are available (read, bash, edit, write, grep, find, ls)
//   - How auth is stored (file-based, env vars)
//   - How settings work (global + project scopes)
//   - How the system prompt is composed
//   - How resources are discovered (skills, AGENTS.md, prompts)
//
// # Sub-Packages
//
//	auth/      → API key storage backends
//	settings/  → Global + project settings manager
//	models/    → Model registry and resolver
//	tools/     → 7 coding agent tools
//	resources/ → Skill, prompt, and context file discovery
//	prompt/    → System prompt composition
//	skills/    → Skill loading and formatting
//	messages/  → Message conversion utilities
//	config/    → Path defaults and configuration values
package sdk
