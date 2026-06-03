package config

import "github.com/chinudotdev/pi-go/agent"

// DefaultThinkingLevel is the default reasoning effort level.
const DefaultThinkingLevel = agent.ThinkingMedium

// DefaultToolNames lists the tools enabled by default for a new session.
var DefaultToolNames = []string{
	"read",
	"bash",
	"edit",
	"write",
	"grep",
	"find",
	"ls",
}
