package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chinudotdev/pi-go/agent"
	"github.com/chinudotdev/pi-go/ai"
)

// BashToolOptions configures the bash tool.
type BashToolOptions struct {
	Operations    BashOperations
	CommandPrefix string
	ShellPath     string
}

// BashOperations are pluggable operations for the bash tool.
type BashOperations interface {
	Exec(command, cwd string, opts BashExecOptions) (BashExecResult, error)
}

// BashExecOptions contains options for command execution.
type BashExecOptions struct {
	OnData  func(data []byte)
	Signal  context.Context
	Timeout int // seconds
	Env     []string
}

// BashExecResult contains the result of command execution.
type BashExecResult struct {
	ExitCode int
}

// localBashOps is the default local shell execution backend.
type localBashOps struct {
	shellPath string
}

func (l *localBashOps) Exec(command, cwd string, opts BashExecOptions) (BashExecResult, error) {
	// Check working directory exists
	if _, err := os.Stat(cwd); os.IsNotExist(err) {
		return BashExecResult{}, fmt.Errorf("working directory does not exist: %s\nCannot execute bash commands", cwd)
	}

	shell := l.shellPath
	if shell == "" {
		shell = os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/bash"
		}
	}

	var args []string
	if strings.HasSuffix(filepath.Base(shell), "fish") {
		args = []string{"-c", command}
	} else {
		args = []string{"-c", command}
	}

	ctx := opts.Signal
	if ctx == nil {
		ctx = context.Background()
	}

	cmd := exec.CommandContext(ctx, shell, args...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), opts.Env...)
	cmd.Stdin = nil

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return BashExecResult{}, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return BashExecResult{}, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return BashExecResult{}, fmt.Errorf("failed to start command: %w", err)
	}

	// Stream output
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 && opts.OnData != nil {
				data := make([]byte, n)
				copy(data, buf[:n])
				opts.OnData(data)
			}
			if err != nil {
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 && opts.OnData != nil {
				data := make([]byte, n)
				copy(data, buf[:n])
				opts.OnData(data)
			}
			if err != nil {
				return
			}
		}
	}()

	wg.Wait()

	err = cmd.Wait()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return BashExecResult{}, err
		}
	}

	return BashExecResult{ExitCode: exitCode}, nil
}

// CreateBashTool creates the bash tool.
func CreateBashTool(cwd string, opts *BashToolOptions) *agent.Tool {
	if opts == nil {
		opts = &BashToolOptions{}
	}

	ops := opts.Operations
	if ops == nil {
		ops = &localBashOps{shellPath: opts.ShellPath}
	}
	commandPrefix := opts.CommandPrefix

	return &agent.Tool{
		Name:        "bash",
		Label:       "bash",
		Description: ToolDescriptions[ToolBash],
		Parameters:  ToolSchemas[ToolBash],
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate agent.ToolUpdateCallback) (*agent.ToolResult, error) {
			command, _ := params["command"].(string)
			if command == "" {
				return nil, fmt.Errorf("command is required")
			}

			var timeout int
			if v, ok := params["timeout"]; ok {
				if f, ok := v.(float64); ok && f > 0 {
					timeout = int(f)
				}
			}

			resolvedCommand := command
			if commandPrefix != "" {
				resolvedCommand = commandPrefix + "\n" + command
			}

			output := NewOutputAccumulator()
			var mu sync.Mutex
			updateTimer := time.NewTimer(0)
			updateTimer.Stop()
			updateDirty := false

			emitUpdate := func() {
				mu.Lock()
				if !updateDirty || onUpdate == nil {
					mu.Unlock()
					return
				}
				updateDirty = false
				mu.Unlock()

				snapshot := output.Snapshot(true)
				details := map[string]any{}
				if snapshot.Truncation.Truncated {
					details["truncation"] = snapshot.Truncation
					details["fullOutputPath"] = snapshot.FullOutputPath
				}
				if onUpdate != nil {
					onUpdate(agent.ToolResult{
						Content: []ai.ContentBlock{newTextBlock(snapshot.Content)},
						Details: details,
					})
				}
			}

			scheduleUpdate := func() {
				mu.Lock()
				updateDirty = true
				mu.Unlock()
				updateTimer.Reset(100 * time.Millisecond)
			}

			handleData := func(data []byte) {
				output.Append(data)
				scheduleUpdate()
			}

			// Background update timer
			done := make(chan struct{})
			go func() {
				for {
					select {
					case <-updateTimer.C:
						emitUpdate()
					case <-done:
						return
					}
				}
			}()
			defer close(done)

			// Execute with timeout context
			execCtx := ctx
			if timeout > 0 {
				var cancel context.CancelFunc
				execCtx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
				defer cancel()
			}

			result, err := ops.Exec(resolvedCommand, cwd, BashExecOptions{
				OnData:  handleData,
				Signal:  execCtx,
				Timeout: timeout,
			})

			output.Finish()
			updateTimer.Stop()
			emitUpdate()
			output.CloseTempFile()

			if err != nil {
				snapshot := output.Snapshot()
				errText := snapshot.Content
				if execCtx.Err() == context.DeadlineExceeded {
					return nil, fmt.Errorf("%s\n\nCommand timed out after %d seconds", errText, timeout)
				}
				if execCtx.Err() == context.Canceled || ctx.Err() == context.Canceled {
					return nil, fmt.Errorf("%s\n\nCommand aborted", errText)
				}
				return nil, fmt.Errorf("%s\n\n%w", errText, err)
			}

			snapshot := output.Snapshot(true)

			// Build result
			text := snapshot.Content
			if text == "" {
				text = "(no output)"
			}

			details := map[string]any{}
			if snapshot.Truncation.Truncated {
				startLine := snapshot.Truncation.TotalLines - snapshot.Truncation.OutputLines + 1
				endLine := snapshot.Truncation.TotalLines

				var truncationText string
				if snapshot.Truncation.LastLinePartial {
					lastLineSize := FormatSize(output.GetLastLineBytes())
					truncationText = fmt.Sprintf("[Showing last %s of line %d (line is %s). Full output: %s]",
						FormatSize(snapshot.Truncation.OutputBytes), endLine, lastLineSize, snapshot.FullOutputPath)
				} else if snapshot.Truncation.TruncatedBy == "lines" {
					truncationText = fmt.Sprintf("[Showing lines %d-%d of %d. Full output: %s]",
						startLine, endLine, snapshot.Truncation.TotalLines, snapshot.FullOutputPath)
				} else {
					truncationText = fmt.Sprintf("[Showing lines %d-%d of %d (%s limit). Full output: %s]",
						startLine, endLine, snapshot.Truncation.TotalLines, FormatSize(DefaultMaxBytes), snapshot.FullOutputPath)
				}
				text += "\n\n" + truncationText

				details["truncation"] = snapshot.Truncation
				details["fullOutputPath"] = snapshot.FullOutputPath
			}

			if result.ExitCode != 0 {
				return nil, fmt.Errorf("%s\n\nCommand exited with code %d", text, result.ExitCode)
			}

			return &agent.ToolResult{
				Content: []ai.ContentBlock{newTextBlock(text)},
				Details: details,
			}, nil
		},
	}
}
