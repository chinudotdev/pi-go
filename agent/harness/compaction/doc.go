// Package compaction provides conversation history summarization for session management.
//
// When an agent conversation grows too large for the model's context window, the
// compaction system summarizes older messages into a structured checkpoint, keeping
// only recent messages in full. This allows agents to maintain context across
// arbitrarily long sessions.
//
// The package also provides branch summarization — when a user navigates away from
// an exploration branch, the abandoned messages are summarized so the context isn't
// lost when returning later.
package compaction
