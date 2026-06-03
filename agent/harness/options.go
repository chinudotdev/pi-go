package harness

import (
	"context"
	"time"

	"github.com/chinudotdev/pi-go/agent"
	"github.com/chinudotdev/pi-go/ai"
)

// Skill represents a loaded skill from a SKILL.md file.
type Skill struct {
	Name                   string `json:"name"`
	Description            string `json:"description"`
	Content                string `json:"content"`
	FilePath               string `json:"filePath"`
	DisableModelInvocation bool   `json:"disableModelInvocation,omitempty"`
}

// PromptTemplate is a named template for generating prompts.
type PromptTemplate struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Content     string `json:"content"`
}

// HarnessResources holds skills and prompt templates available to the harness.
type HarnessResources struct {
	PromptTemplates []PromptTemplate `json:"promptTemplates,omitempty"`
	Skills          []Skill          `json:"skills,omitempty"`
}

// HarnessStreamOptions holds curated provider request options.
type HarnessStreamOptions struct {
	Transport       ai.Transport       `json:"transport,omitempty"`
	TimeoutMs       *int               `json:"timeoutMs,omitempty"`
	MaxRetries      *int               `json:"maxRetries,omitempty"`
	MaxRetryDelayMs *int               `json:"maxRetryDelayMs,omitempty"`
	Headers         map[string]string  `json:"headers,omitempty"`
	Metadata        map[string]any     `json:"metadata,omitempty"`
	CacheRetention  *ai.CacheRetention `json:"cacheRetention,omitempty"`
}

// Clone returns a deep copy of the stream options.
func (o HarnessStreamOptions) Clone() HarnessStreamOptions {
	clone := o
	if o.Headers != nil {
		clone.Headers = make(map[string]string, len(o.Headers))
		for k, v := range o.Headers {
			clone.Headers[k] = v
		}
	}
	if o.Metadata != nil {
		clone.Metadata = make(map[string]any, len(o.Metadata))
		for k, v := range o.Metadata {
			clone.Metadata[k] = v
		}
	}
	return clone
}

// HarnessStreamOptionsPatch is a per-request patch for stream options.
type HarnessStreamOptionsPatch struct {
	Transport       *ai.Transport      `json:"transport,omitempty"`
	TimeoutMs       *int               `json:"timeoutMs,omitempty"`
	MaxRetries      *int               `json:"maxRetries,omitempty"`
	MaxRetryDelayMs *int               `json:"maxRetryDelayMs,omitempty"`
	CacheRetention  *ai.CacheRetention `json:"cacheRetention,omitempty"`
	Headers         map[string]*string `json:"headers,omitempty"`
	Metadata        map[string]any     `json:"metadata,omitempty"`
}

// ApplyStreamOptionsPatch applies a patch to base stream options.
func ApplyStreamOptionsPatch(base HarnessStreamOptions, patch *HarnessStreamOptionsPatch) HarnessStreamOptions {
	result := base.Clone()
	if patch == nil {
		return result
	}

	if patch.Transport != nil {
		result.Transport = *patch.Transport
	}
	if patch.TimeoutMs != nil {
		result.TimeoutMs = patch.TimeoutMs
	}
	if patch.MaxRetries != nil {
		result.MaxRetries = patch.MaxRetries
	}
	if patch.MaxRetryDelayMs != nil {
		result.MaxRetryDelayMs = patch.MaxRetryDelayMs
	}
	if patch.CacheRetention != nil {
		result.CacheRetention = patch.CacheRetention
	}

	if patch.Headers != nil {
		if result.Headers == nil {
			result.Headers = make(map[string]string)
		}
		for k, v := range patch.Headers {
			if v == nil {
				delete(result.Headers, k)
			} else {
				result.Headers[k] = *v
			}
		}
		if len(result.Headers) == 0 {
			result.Headers = nil
		}
	}

	if patch.Metadata != nil {
		if result.Metadata == nil {
			result.Metadata = make(map[string]any)
		}
		for k, v := range patch.Metadata {
			if v == nil {
				delete(result.Metadata, k)
			} else {
				result.Metadata[k] = v
			}
		}
		if len(result.Metadata) == 0 {
			result.Metadata = nil
		}
	}

	return result
}

// PendingSessionWrite represents a deferred session mutation that is flushed
// when the harness transitions back to idle.
type PendingSessionWrite struct {
	Type string `json:"type"`

	// message
	Message ai.Message `json:"message,omitempty"`

	// model_change
	Provider string `json:"provider,omitempty"`
	ModelID  string `json:"modelId,omitempty"`

	// thinking_level_change
	ThinkingLevel string `json:"thinkingLevel,omitempty"`

	// active_tools_change
	ActiveToolNames []string `json:"activeToolNames,omitempty"`

	// custom
	CustomType string `json:"customType,omitempty"`
	Data       any    `json:"data,omitempty"`

	// custom_message
	Content any  `json:"content,omitempty"`
	Display bool `json:"display,omitempty"`
	Details any  `json:"details,omitempty"`

	// label
	TargetID string  `json:"targetId,omitempty"`
	Label    *string `json:"label,omitempty"`

	// session_info
	Name string `json:"name,omitempty"`
}

// AuthInfo contains resolved API key and headers.
type AuthInfo struct {
	APIKey  string
	Headers map[string]string
}

// SystemPromptFn resolves the system prompt string.
type SystemPromptFn func(env ExecutionEnv, model *ai.Model, thinkingLevel string, activeTools []agent.Tool, resources HarnessResources) (string, error)

// GetApiKeyAndHeadersFn resolves API authentication for a model.
type GetApiKeyAndHeadersFn func(model *ai.Model) (*AuthInfo, error)

// HarnessEventHandler processes a harness event. May return a result to
// modify harness behavior.
type HarnessEventHandler func(event HarnessEvent) (any, error)

// SessionProvider is the interface the harness needs from a session.
// *session.Session satisfies this interface.
type SessionProvider interface {
	GetMetadata(ctx context.Context) (SessionMetadata, error)
	GetLeafID(ctx context.Context) (*string, error)
	GetEntry(ctx context.Context, id string) (*SessionTreeEntry, error)
	GetEntries(ctx context.Context) ([]SessionTreeEntry, error)
	GetBranch(ctx context.Context, fromID *string) ([]SessionTreeEntry, error)
	BuildContext(ctx context.Context) (*SessionContext, error)
	AppendMessage(ctx context.Context, message ai.Message) (string, error)
	AppendModelChange(ctx context.Context, provider, modelID string) (string, error)
	AppendThinkingLevelChange(ctx context.Context, level string) (string, error)
	AppendActiveToolsChange(ctx context.Context, activeToolNames []string) (string, error)
	AppendCompaction(ctx context.Context, summary, firstKeptEntryID string, tokensBefore int, details any, fromHook bool) (string, error)
	AppendBranchSummary(ctx context.Context, fromID, summary string, details any, fromHook bool) (string, error)
	AppendCustomEntry(ctx context.Context, customType string, data any) (string, error)
	AppendCustomMessageEntry(ctx context.Context, customType string, content any, display bool, details any) (string, error)
	AppendLabel(ctx context.Context, targetID string, label *string) (string, error)
	AppendSessionName(ctx context.Context, name string) (string, error)
	MoveTo(ctx context.Context, entryID *string, summary *BranchSummaryResult) (*string, error)
}

// CompactionFunc performs compaction and returns the result.
type CompactionFunc func(ctx context.Context, preparation any, model *ai.Model, apiKey string, headers map[string]string, customInstructions string, thinkingLevel string) (any, error)

// PrepareCompactionFunc prepares compaction from branch entries.
type PrepareCompactionFunc func(entries []SessionTreeEntry, settings any) (any, error)

// CollectBranchEntriesFunc collects entries for branch summarization.
type CollectBranchEntriesFunc func(sess SessionProvider, oldLeafID, targetID string) (entries []SessionTreeEntry, commonAncestorID string, err error)

// GenerateBranchSummaryFunc generates a summary for a branch.
type GenerateBranchSummaryFunc func(ctx context.Context, entries []SessionTreeEntry, opts any) (any, error)

// HarnessOptions configures an AgentHarness.
type HarnessOptions struct {
	Env                 ExecutionEnv
	Resources           *HarnessResources
	StreamOptions       *HarnessStreamOptions
	SystemPrompt        any // string or SystemPromptFn
	GetApiKeyAndHeaders GetApiKeyAndHeadersFn
	Tools               []agent.Tool
	ActiveToolNames     []string
	Model               *ai.Model
	ThinkingLevel       string
	SteeringMode        agent.QueueMode
	FollowUpMode        agent.QueueMode

	// Compaction/branch functions (injected to avoid import cycles).
	// Wire these from the compaction package at the call site.
	CompactFn                   CompactionFunc
	PrepareCompactionFn         PrepareCompactionFunc
	DefaultCompactionSettingsFn func() any
	CollectBranchEntriesFn      CollectBranchEntriesFunc
	GenerateBranchSummaryFn     GenerateBranchSummaryFunc
}

// ============================================================================
// Utility functions
// ============================================================================

// NowISO returns the current time as an ISO 8601 string.
var NowISO = func() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// MergeHeaders merges multiple header maps, later maps override earlier ones.
func MergeHeaders(headers ...map[string]string) map[string]string {
	merged := make(map[string]string)
	for _, h := range headers {
		for k, v := range h {
			merged[k] = v
		}
	}
	return merged
}
