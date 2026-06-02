// Package agent provides a stateful agent loop for building AI assistants.
//
// The agent manages conversation state, executes tools, emits lifecycle events,
// and supports steering/follow-up message queues for interactive agent control.
//
// # Quick Start
//
// Register the faux provider for testing:
//
//	providers.RegisterBuiltInApiProviders()
//
// Create and run an agent:
//
//	agent := agent.New(agent.Options{
//	    Model: myModel,
//	})
//	agent.Subscribe(func(event agent.Event) { ... })
//	agent.Prompt(ctx, "Hello!")
//
// # Architecture
//
// The agent loop:
//  1. Transforms AgentMessage[] → ai.Message[] via ConvertToLlm
//  2. Calls the StreamFn to get an LLM response
//  3. Executes any tool calls (sequential or parallel)
//  4. Checks steering/follow-up queues
//  5. Repeats until no more work
//
// # Events
//
// The agent emits typed events: agent_start, agent_end, turn_start, turn_end,
// message_start, message_update, message_end, tool_execution_start,
// tool_execution_update, tool_execution_end.
//
// # Tool Execution
//
// Tools implement the Tool interface with PrepareArguments and Execute methods.
// Before/after hooks allow blocking, modifying results, or triggering early termination.
package agent
