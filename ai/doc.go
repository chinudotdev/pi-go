// Package ai provides a provider-agnostic abstraction for interacting with
// large language models (LLMs). It handles streaming, completion, model
// cataloging, cost calculation, and thinking level management.
//
// # Core Concepts
//
// Models are identified by Provider + ID (e.g. "openai" + "gpt-4o"). Each model
// declares which API protocol it uses (e.g. "openai", "anthropic"). API providers
// are registered globally and handle the actual HTTP communication.
//
// Streaming returns an EventStream (a typed channel) of AssistantMessageEvent.
// Completion blocks until the full response is ready.
//
// # Quick Start
//
//	model, _ := ai.GetModel("openai", "gpt-4o")
//	ctx := &ai.Context{
//	    System:   "You are helpful.",
//	    Messages: []ai.Message{ai.NewUserMessage("hello")},
//	}
//	stream, _ := ai.StreamSimple(ctx, model, context, nil)
//	for event := range stream.Iterate() {
//	    fmt.Print(event.Text)
//	}
//
// See package documentation for the full API.
package ai
