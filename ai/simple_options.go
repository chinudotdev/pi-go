package ai

import "math"

// BuildBaseOptions extracts common stream options from SimpleStreamOptions.
func BuildBaseOptions(model *Model, options *SimpleStreamOptions, apiKey string) StreamOptions {
	opts := StreamOptions{APIKey: &apiKey}
	if options == nil {
		return opts
	}
	opts.Temperature = options.Temperature
	opts.MaxTokens = options.MaxTokens
	opts.Transport = options.Transport
	opts.CacheRetention = options.CacheRetention
	opts.SessionID = options.SessionID
	opts.Headers = options.Headers
	opts.TimeoutMs = options.TimeoutMs
	opts.WebSocketConnectTimeoutMs = options.WebSocketConnectTimeoutMs
	opts.MaxRetries = options.MaxRetries
	opts.MaxRetryDelayMs = options.MaxRetryDelayMs
	opts.Metadata = options.Metadata
	opts.OnPayload = options.OnPayload
	opts.OnResponse = options.OnResponse
	return opts
}

// ClampReasoning caps thinking level at "high" (excludes "xhigh").
func ClampReasoning(effort ThinkingLevel) ThinkingLevel {
	if effort == ThinkingXHigh {
		return ThinkingHigh
	}
	return effort
}

// AdjustMaxTokensForThinking computes max output tokens and thinking budget
// given a reasoning level and optional custom budgets.
func AdjustMaxTokensForThinking(baseMaxTokens *int, modelMaxTokens int, reasoningLevel ThinkingLevel, customBudgets *ThinkingBudgets) (maxTokens int, thinkingBudget int) {
	defaultBudgets := ThinkingBudgets{
		Minimal: intPtr(1024),
		Low:     intPtr(2048),
		Medium:  intPtr(8192),
		High:    intPtr(16384),
	}
	budgets := defaultBudgets
	if customBudgets != nil {
		if customBudgets.Minimal != nil {
			budgets.Minimal = customBudgets.Minimal
		}
		if customBudgets.Low != nil {
			budgets.Low = customBudgets.Low
		}
		if customBudgets.Medium != nil {
			budgets.Medium = customBudgets.Medium
		}
		if customBudgets.High != nil {
			budgets.High = customBudgets.High
		}
	}

	const minOutputTokens = 1024
	level := ClampReasoning(reasoningLevel)

	thinkingBudget = getThinkingBudget(level, &budgets)
	if baseMaxTokens == nil {
		maxTokens = modelMaxTokens
	} else {
		maxTokens = int(math.Min(float64(*baseMaxTokens+thinkingBudget), float64(modelMaxTokens)))
	}

	if maxTokens <= thinkingBudget {
		thinkingBudget = int(math.Max(0, float64(maxTokens-minOutputTokens)))
	}

	return maxTokens, thinkingBudget
}

func getThinkingBudget(level ThinkingLevel, budgets *ThinkingBudgets) int {
	switch level {
	case ThinkingMinimal:
		if budgets.Minimal != nil {
			return *budgets.Minimal
		}
	case ThinkingLow:
		if budgets.Low != nil {
			return *budgets.Low
		}
	case ThinkingMedium:
		if budgets.Medium != nil {
			return *budgets.Medium
		}
	case ThinkingHigh:
		if budgets.High != nil {
			return *budgets.High
		}
	}
	return 1024
}

func intPtr(v int) *int { return &v }
