package ai

import "time"

// nowMillis returns the current Unix timestamp in milliseconds.
// Exposed as a package-level variable so tests can override it.
var nowMillis = func() int64 {
	return time.Now().UnixMilli()
}

// ZeroUsage returns a Usage with all zeros.
func ZeroUsage() Usage {
	return Usage{
		Cost: ZeroCost(),
	}
}

// ZeroCost returns a Cost with all zeros.
func ZeroCost() Cost {
	return Cost{}
}

// NewAssistantOutput creates a new AssistantMessage with zeroed fields.
func NewAssistantOutput(api Api, provider Provider, modelID string) AssistantMessage {
	return AssistantMessage{
		Role:       "assistant",
		Content:    []ContentBlock{},
		API:        api,
		Provider:   provider,
		Model:      modelID,
		Usage:      ZeroUsage(),
		StopReason: StopReasonStop,
		Timestamp:  nowMillis(),
	}
}

// AssistantMessageToMessage converts an AssistantMessage to a Message.
func AssistantMessageToMessage(am AssistantMessage) Message {
	return Message{
		Role:             am.Role,
		AssistantContent: am.Content,
		API:              am.API,
		Provider:         am.Provider,
		Model:            am.Model,
		ResponseModel:    am.ResponseModel,
		ResponseID:       am.ResponseID,
		Diagnostics:      am.Diagnostics,
		Usage:            am.Usage,
		StopReason:       am.StopReason,
		ErrorMessage:     am.ErrorMessage,
		Timestamp:        am.Timestamp,
	}
}

// MakeErrorAssistantMessage creates an assistant message representing an error.
func MakeErrorAssistantMessage(model *Model, errMsg string) AssistantMessage {
	return AssistantMessage{
		Role:         "assistant",
		Content:      []ContentBlock{},
		API:          model.API,
		Provider:     model.Provider,
		Model:        model.ID,
		Usage:        ZeroUsage(),
		StopReason:   StopReasonError,
		ErrorMessage: &errMsg,
		Timestamp:    nowMillis(),
	}
}
