// Package command provides implementations for executing various types of commands
// in the AI agent backend. These commands are executed asynchronously and report
// their results through channels.
package task

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Error constants for BashExecExecutor
const (
	// Command validation errors
	errBashInvalidCommandType = "invalid command type: expected BashExecCommand, got %T"

	// Execution setup errors
	errBashStdoutPipe   = "failed to get stdout pipe: %w"
	errBashStderrPipe   = "failed to get stderr pipe: %w"
	errBashStartCommand = "Failed to start command: %v"

	// Status messages
	msgBashCancelled = "Command execution cancelled."
	msgBashTimedOut  = "Command execution timed out after %v."
	msgBashFailed    = "Command failed with exit code %d: %v"
	msgBashSucceeded = "Command completed successfully in %v."
)

// BashExecExecutor handles the execution of BashExecCommand.
// It implements the CommandExecutor interface for shell command execution.
type BashExecExecutor struct {
	// Dependencies can be added here if needed later, e.g., logger.
}

// NewBashExecExecutor creates a new BashExecExecutor.
func NewBashExecExecutor() *BashExecExecutor {
	return &BashExecExecutor{}
}

// bashScriptTemplate is the template used to wrap user commands in a bash script.
// It sets up error handling and reporting through the EXIT trap.
// The template expects two format arguments:
// 1. A command ID to use in the temporary CWD file
// 2. The actual bash command(s) to execute
const bashScriptTemplate = `#!/bin/bash

# --- Configuration ---
set -e

# --- Trap Definition ---
report_final_cwd() {
  local exit_status=$?
  # Ensure final messages go to stderr to avoid mixing with command stdout
  echo >&2 
  echo "############################################" >&2
  echo "# Script Exiting" >&2
  echo "# Exit Status: $exit_status" >&2
  echo "# Final Working Directory: $(pwd -P)" >&2
  echo "############################################" >&2
  # Write final CWD to a temporary file for the Go process to read
  echo "$(pwd -P)" > /tmp/%s.cwd
}
trap report_final_cwd EXIT

# --- Main Script Logic ---
# Use stderr for script messages to separate from command output
echo "Starting main script execution..." >&2 
echo "Initial directory: $(pwd)" >&2
echo "---" >&2

# === YOUR BASH COMMANDS START HERE ===

%s

# === YOUR BASH COMMANDS END HERE ===
`

// Execute runs the bash command specified in the BashExecCommand, streaming output.
// It expects the cmd argument to be of type BashExecCommand.
// The execution respects cancellation signals from the passed context.Context.
//
// The process for executing bash commands is:
// 1. Set up a timeout context
// 2. Prepare the command with stdout/stderr pipes
// 3. Start the command and stream its output
// 4. Wait for completion and process the final result
//
// Returns a channel for results and an error if the command type is wrong or execution setup fails.
func (e *BashExecExecutor) Execute(ctx context.Context, cmd any) (<-chan OutputResult, error) {
	bashCmd, ok := cmd.(BashExecTask)
	if !ok {
		return nil, fmt.Errorf(errBashInvalidCommandType, cmd)
	}

	// If the task is already in a terminal state, return it as is
	terminalChan, err := HandleTerminalTask(bashCmd.TaskId, bashCmd.Status, bashCmd.Output)
	if err != nil || terminalChan != nil {
		return terminalChan, err
	}

	// Buffered channel (size 1) for streaming results + final status.
	// Buffer allows final send even if receiver isn't immediately ready.
	results := make(chan OutputResult, 1)

	// Start execution and streaming in a goroutine
	go func() {
		defer close(results)

		// Update task status to Running
		bashCmd.Status = StatusRunning

		// Setup context with timeout
		const internalTimeout = 5 * time.Minute
		execCtx, cancel := context.WithTimeout(ctx, internalTimeout)
		defer cancel() // Ensure resources associated with the timeout context are released

		// Setup command with pipes for output
		execCmd, combinedPipe, err := setupCommand(execCtx, bashCmd)
		if err != nil {
			finalResult := createErrorResult(bashCmd, err.Error())
			// Update task output
			bashCmd.Status = StatusFailed
			bashCmd.UpdateOutput(&finalResult)
			results <- finalResult
			return
		}

		// Start command execution and track time
		startTime := time.Now()
		if err := execCmd.Start(); err != nil {
			finalResult := createErrorResult(bashCmd, fmt.Sprintf(errBashStartCommand, err))
			// Update task output
			bashCmd.Status = StatusFailed
			bashCmd.UpdateOutput(&finalResult)
			results <- finalResult
			return
		}

		// Stream command output to results channel
		var readerWg sync.WaitGroup
		streamCommandOutput(execCtx, combinedPipe, bashCmd, results, &readerWg)

		// Wait for reader goroutine to finish, respecting context cancellation
		waitErr := waitGroupWithContext(execCtx, &readerWg)
		if waitErr != nil {
			// If waiting was interrupted by context cancellation, handle it
			// The rest of the function will use execCtx.Err() to detect this
		}

		// Wait for command completion and process final status
		waitErr = execCmd.Wait() // This will return an error if the context caused termination
		duration := time.Since(startTime)

		// Send final result
		finalResult := processFinalResult(execCtx, execCmd, bashCmd, waitErr, duration, internalTimeout)

		// Update task status and output
		bashCmd.Status = finalResult.Status
		bashCmd.UpdateOutput(&finalResult)

		results <- finalResult
	}()

	return results, nil
}

// setupCommand prepares the exec.Command for execution with the bash script.
// It configures stdout and stderr pipes and returns the command, a combined reader for
// stdout and stderr, and any error that occurred during setup.
func setupCommand(ctx context.Context, bashCmd BashExecTask) (*exec.Cmd, io.Reader, error) {
	// Construct the full script
	fullScript := fmt.Sprintf(bashScriptTemplate, bashCmd.TaskId, bashCmd.Parameters.Command)

	// Prepare command for streaming using the execution context
	execCmd := exec.CommandContext(ctx, "/bin/bash", "-c", fullScript)

	stdoutPipe, err := execCmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf(errBashStdoutPipe, err)
	}

	stderrPipe, err := execCmd.StderrPipe()
	if err != nil {
		return nil, nil, fmt.Errorf(errBashStderrPipe, err)
	}

	// Combine stdout and stderr for reading
	combinedPipe := io.MultiReader(stdoutPipe, stderrPipe)

	return execCmd, combinedPipe, nil
}

// streamCommandOutput reads from the provided reader and sends each line to the results channel.
// The function respects context cancellation and reports errors appropriately.
// It uses the provided WaitGroup to signal when all output has been processed.
func streamCommandOutput(ctx context.Context, reader io.Reader, cmd BashExecTask,
	results chan<- OutputResult, wg *sync.WaitGroup) {

	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(reader)

		for scanner.Scan() {
			line := scanner.Text()
			// Check if the context was cancelled before sending the next line
			select {
			case <-ctx.Done():
				// If context is cancelled (timeout or external), stop sending lines.
				return
			default:
				// Context still active, send the result
				results <- OutputResult{
					TaskID:     cmd.TaskId,
					Status:     StatusRunning,
					ResultData: line + "\n", // Add newline back as scanner strips it
				}
			}
		}

		scannerErr := scanner.Err()
		if scannerErr != nil && ctx.Err() == nil {
			// Don't send error if context was cancelled, as that's the primary error
			results <- createErrorResult(cmd, fmt.Sprintf("Error reading command output: %v", scannerErr))
		}
	}()
}

// processFinalResult determines the final status of a command execution and creates
// an appropriate OutputResult. It handles various error conditions including timeouts,
// cancellations, and command execution failures.
// It also attempts to read the final working directory from the temporary file.
func processFinalResult(ctx context.Context, cmd *exec.Cmd, bashCmd BashExecTask,
	waitErr error, duration time.Duration, timeout time.Duration) OutputResult {

	finalStatus := StatusSucceeded // Assume success initially
	errMsg := ""
	message := fmt.Sprintf(msgBashSucceeded, duration.Round(time.Millisecond))

	// Check context error first, as it overrides waitErr
	contextErr := ctx.Err()
	if contextErr == context.DeadlineExceeded {
		finalStatus = StatusFailed
		errMsg = fmt.Sprintf(msgBashTimedOut, timeout)
		message = "Command execution timed out."
	} else if contextErr == context.Canceled {
		finalStatus = StatusFailed
		errMsg = msgBashCancelled
		message = "Command execution cancelled."
	} else if waitErr != nil {
		// Context was okay, so this is a command execution error (like non-zero exit)
		finalStatus = StatusFailed
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			errMsg = fmt.Sprintf(msgBashFailed, exitErr.ExitCode(), waitErr.Error())
		} else {
			// Other errors (e.g., I/O problems reported by Wait)
			errMsg = fmt.Sprintf("Command execution failed after wait: %v", waitErr)
		}
		message = "Command execution failed."
	}

	// Read CWD file (attempt even on error/cancel, might have been written before kill)
	cwdFilePath := fmt.Sprintf("/tmp/%s.cwd", bashCmd.TaskId)
	cwdBytes, readErr := os.ReadFile(cwdFilePath)
	if readErr == nil {
		finalCwd := strings.TrimSpace(string(cwdBytes))
		message += fmt.Sprintf(" Final CWD: %s.", finalCwd)
	} else if contextErr == nil {
		// Only report CWD read error if the command didn't fail due to context cancellation
		message += " (Could not read final CWD)."
	}

	return OutputResult{
		TaskID:  bashCmd.TaskId,
		Status:  finalStatus,
		Message: message,
		Error:   errMsg,
	}
}

// waitGroupWithContext waits for a WaitGroup to complete while respecting context cancellation.
// Returns nil if the WaitGroup completes normally, or the context's error if the context is
// canceled before the WaitGroup completes.
func waitGroupWithContext(ctx context.Context, wg *sync.WaitGroup) error {
	ch := make(chan struct{})

	go func() {
		wg.Wait()
		close(ch)
	}()

	select {
	case <-ch:
		return nil // WaitGroup completed normally
	case <-ctx.Done():
		return ctx.Err() // Context was canceled
	}
}

// createErrorResult creates a standardized error OutputResult for a BashExecCommand.
func createErrorResult(cmd BashExecTask, errMsg string) OutputResult {
	return OutputResult{
		TaskID:  cmd.TaskId,
		Status:  StatusFailed,
		Message: "Command execution failed.",
		Error:   errMsg,
	}
}

// CreateErrorResult creates an error result for a failed command execution.
// This is a method on BashExecExecutor to satisfy potential interface requirements.
func (e *BashExecExecutor) CreateErrorResult(cmd BashExecTask, err error) OutputResult {
	var errMsg string
	if err != nil {
		errMsg = err.Error()
	}
	return OutputResult{
		TaskID:  cmd.TaskId,
		Status:  StatusFailed,
		Message: fmt.Sprintf("Command execution failed: %v", err),
		Error:   errMsg,
	}
}
