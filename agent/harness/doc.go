// Package harness provides the AgentHarness orchestrator and supporting types
// for building production-grade AI agent systems with session management,
// compaction, and execution environment abstractions.
//
// The harness layer sits on top of the core agent loop, adding:
//   - FileSystem and Shell interfaces for execution environments
//   - Session tree storage (in-memory and JSONL file-backed)
//   - Compaction and branch summarization
//   - Provider hooks and stream option patching
//   - Tool lifecycle management with active/inactive tool sets
//   - Event-driven architecture with typed hook results
//
// # Result Type
//
// All fallible operations in the harness return Result[T, E] instead of
// throwing errors. This makes error handling explicit and prevents
// uncaught exceptions from disrupting the agent loop.
//
// # Error Hierarchy
//
// The harness defines typed errors for each subsystem:
//   - FileError — filesystem operations
//   - ExecutionError — shell/process execution
//   - CompactionError — context compaction
//   - BranchSummaryError — branch summarization
//   - SessionError — session storage and tree operations
//   - AgentHarnessError — top-level orchestrator errors
//
// # Session Tree
//
// Sessions are modeled as trees of entries. Each entry has a type, ID,
// parent ID, and timestamp. The tree is navigated by following parentId
// chains from a leaf to the root. Compaction replaces a prefix of the
// tree with a summary entry, keeping recent messages intact.
package harness
