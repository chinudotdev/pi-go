// Package agent implements a generic agentic loop that repeatedly calls an LLM,
// executes tool calls, and feeds results back until a terminal condition is met.
//
// The core agent.Agent type is a state machine that supports:
//   - Streaming LLM responses via a pluggable StreamFn
//   - Tool dispatch with before/after hooks
//   - Message queuing (steering and follow-up) for concurrent user input
//   - Abort and resume
//   - Event subscription for real-time monitoring
//
// # Agent Loop
//
// The agent loop runs:
//  1. Convert messages to LLM format
//  2. Call the LLM (via StreamFn)
//  3. Accumulate the streaming response
//  4. Execute any tool calls
//  5. If tool calls were executed, loop back to step 1
//  6. If no tool calls, the turn is complete
//
// # Harness
//
// The agent/harness sub-package adds session persistence, compaction, tree
// navigation, and phase management on top of the core agent.
//
// # Dependencies
//
// This package depends only on the ai/ package for LLM types. It does not
// know about file tools, authentication, or user settings.
package agent
