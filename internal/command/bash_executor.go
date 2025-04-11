package command

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

// BashExecExecutor handles the execution of BashExecCommand.
type BashExecExecutor struct {
	// Dependencies can be added here if needed later, e.g., logger.
}

// NewBashExecExecutor creates a new BashExecExecutor.
func NewBashExecExecutor() *BashExecExecutor {
	return &BashExecExecutor{}
}

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
// Returns a channel for results and an error if the command type is wrong or execution setup fails.
func (e *BashExecExecutor) Execute(ctx context.Context, cmd any) (<-chan OutputResult, error) {
	bashCmd, ok := cmd.(BashExecCommand)
	if !ok {
		return nil, fmt.Errorf("invalid command type: expected BashExecCommand, got %T", cmd)
	}

	// Buffered channel (size 1) for streaming results + final status.
	// Buffer allows final send even if receiver isn't immediately ready.
	results := make(chan OutputResult, 1)

	// Start execution and streaming in a goroutine
	go func() {
		defer close(results)

		// --- Setup Context with Timeout ---
		// Create a context that respects both the parent context (ctx) and the internal 5-minute timeout.
		const internalTimeout = 5 * time.Minute
		execCtx, cancel := context.WithTimeout(ctx, internalTimeout)
		defer cancel() // Ensure resources associated with the timeout context are released

		// --- Construct the full script ---
		fullScript := fmt.Sprintf(bashScriptTemplate, bashCmd.CommandID, bashCmd.Command)

		// --- Prepare Command for Streaming ---
		// Use the derived execution context (execCtx) which includes the timeout.
		execCmd := exec.CommandContext(execCtx, "/bin/bash", "-c", fullScript)

		stdoutPipe, err := execCmd.StdoutPipe()
		if err != nil {
			results <- createErrorResult(bashCmd, fmt.Sprintf("Failed to get stdout pipe: %v", err))
			return
		}
		stderrPipe, err := execCmd.StderrPipe()
		if err != nil {
			results <- createErrorResult(bashCmd, fmt.Sprintf("Failed to get stderr pipe: %v", err))
			return
		}

		// Combine stdout and stderr for reading
		combinedPipe := io.MultiReader(stdoutPipe, stderrPipe)

		// --- Start Command Execution ---
		startTime := time.Now()
		if err := execCmd.Start(); err != nil {
			results <- createErrorResult(bashCmd, fmt.Sprintf("Failed to start command: %v", err))
			return
		}

		// --- Goroutine to Stream Output ---
		var readerWg sync.WaitGroup
		readerWg.Add(1)
		go func() {
			defer readerWg.Done()
			scanner := bufio.NewScanner(combinedPipe)
			for scanner.Scan() {
				line := scanner.Text()
				// Check if the parent context was cancelled before sending the next line
				select {
				case <-execCtx.Done():
					// If context is cancelled (timeout or external), stop sending lines.
					// The error will be handled in the main goroutine after Wait().
					return
				default:
					// Context still active, send the result
					results <- OutputResult{
						CommandID:   bashCmd.CommandID,
						CommandType: CmdBashExec,
						Status:      StatusRunning,
						ResultData:  line + "\n", // Add newline back as scanner strips it
					}
				}
			}
			scannerErr := scanner.Err()
			if scannerErr != nil {
				// Don't send error if context was cancelled, as that's the primary error.
				if execCtx.Err() == nil {
					results <- createErrorResult(bashCmd, fmt.Sprintf("Error reading command output: %v", scannerErr))
				}
			}
		}()

		readerWg.Wait()

		// --- Wait for Command Completion and Process Final Status ---
		waitErr := execCmd.Wait() // This will return an error if the context caused termination
		duration := time.Since(startTime)

		finalStatus := StatusSucceeded // Assume success initially
		errMsg := ""
		message := fmt.Sprintf("Command finished in %v.", duration.Round(time.Millisecond))

		// Check context error first, as it overrides waitErr
		contextErr := execCtx.Err()
		if contextErr == context.DeadlineExceeded {
			finalStatus = StatusFailed
			// Report the actual timeout duration that caused the deadline
			// This requires knowing if it was the internal or parent context deadline.
			// For simplicity, we'll report the internal one, but acknowledge parent could cause it.
			errMsg = fmt.Sprintf("Command timed out (internal deadline: %v)", internalTimeout)
			message = "Command execution timed out."
		} else if contextErr == context.Canceled {
			finalStatus = StatusFailed
			errMsg = "Command execution cancelled by parent context."
			message = "Command execution cancelled."
		} else if waitErr != nil {
			// Context was okay, so this is a command execution error (like non-zero exit)
			finalStatus = StatusFailed
			if exitErr, ok := waitErr.(*exec.ExitError); ok {
				errMsg = fmt.Sprintf("Command failed with exit code %d: %s", exitErr.ExitCode(), waitErr.Error())
			} else {
				// Other errors (e.g., I/O problems reported by Wait)
				errMsg = fmt.Sprintf("Command execution failed after wait: %v", waitErr)
			}
			message = "Command execution failed."
		}

		// Read CWD file (attempt even on error/cancel, might have been written before kill)
		cwdFilePath := fmt.Sprintf("/tmp/%s.cwd", bashCmd.CommandID)
		cwdBytes, readErr := os.ReadFile(cwdFilePath)
		if readErr == nil {
			finalCwd := strings.TrimSpace(string(cwdBytes))
			message += fmt.Sprintf(" Final CWD: %s.", finalCwd)
		} else {
			// Only report CWD read error if the command didn't fail due to context cancellation
			if contextErr == nil {
				message += " (Could not read final CWD)."
			}
		}

		// --- Send Final Result ---
		results <- OutputResult{
			CommandID:   bashCmd.CommandID,
			CommandType: CmdBashExec,
			Status:      finalStatus,
			Message:     message,
			Error:       errMsg,
			// ResultData is empty for the final status message
		}
	}()

	return results, nil
}

// Helper to create a standardized error result
func createErrorResult(cmd BashExecCommand, errMsg string) OutputResult {
	return OutputResult{
		CommandID:   cmd.CommandID,
		CommandType: CmdBashExec,
		Status:      StatusFailed,
		Message:     "Command execution failed.",
		Error:       errMsg,
	}
}

// CreateErrorResult creates an error result for a failed command execution
func (e *BashExecExecutor) CreateErrorResult(cmd BashExecCommand, err error) OutputResult {
	var errMsg string
	if err != nil {
		errMsg = err.Error()
	}
	return OutputResult{
		CommandID:   cmd.CommandID,
		CommandType: CmdBashExec,
		Status:      StatusFailed,
		Message:     fmt.Sprintf("Command execution failed: %v", err),
		Error:       errMsg,
	}
}
