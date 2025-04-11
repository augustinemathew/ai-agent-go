// Package command provides implementations for executing various types of commands
// in the AI agent backend. These commands are executed asynchronously and report
// their results through channels.
package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

// FileWriteExecutor handles the execution of FileWriteCommand.
// It manages file creation, writing content, and proper error handling.
type FileWriteExecutor struct{}

// NewFileWriteExecutor creates a new FileWriteExecutor.
func NewFileWriteExecutor() *FileWriteExecutor {
	return &FileWriteExecutor{}
}

// Execute writes the content specified in the FileWriteCommand to the target file path.
// It expects the cmd argument to be of type FileWriteCommand.
// The execution is performed asynchronously and respects cancellation signals from the
// provided context. Results are sent through the returned channel.
//
// Returns a channel for results and an error if the command type is wrong or execution setup fails.
func (e *FileWriteExecutor) Execute(ctx context.Context, cmd any) (<-chan OutputResult, error) {
	fileWriteCmd, ok := cmd.(FileWriteCommand)
	if !ok {
		return nil, fmt.Errorf("invalid command type: expected FileWriteCommand, got %T", cmd)
	}

	// Buffered channel (size 1) for the final status. No intermediate results for write.
	results := make(chan OutputResult, 1)

	go func() {
		cmdID := fileWriteCmd.CommandID
		startTime := time.Now()
		var finalErr error

		// Close the channel after sending the final result
		defer func() {
			close(results)
		}()

		// Prepare and send the final result before the channel is closed
		defer func() {
			// Use the existing error or check for context cancellation one last time
			effectiveErr := finalErr
			if effectiveErr == nil {
				if err := checkContext(ctx); err != nil {
					effectiveErr = err
				}
			}

			// Create and send the final result based on the error status
			duration := time.Since(startTime)
			finalResult := createFinalResult(cmdID, fileWriteCmd.FilePath, effectiveErr, duration)

			results <- finalResult
		}()

		// Check for immediate cancellation before starting work
		if err := checkContext(ctx); err != nil {
			finalErr = err
			return
		}

		// Write the file content
		finalErr = writeFileContent(ctx, fileWriteCmd.FilePath, fileWriteCmd.Content)
	}()

	return results, nil
}

// checkContext checks if the context is done and returns the context's error if it is.
// It returns nil if the context is still active.
func checkContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// createFinalResult constructs an OutputResult based on the error status,
// setting appropriate messages and status codes for the FileWriteCommand.
func createFinalResult(cmdID, filePath string, err error, duration time.Duration) OutputResult {
	var status ExecutionStatus
	var errMsg string
	var message string

	if err != nil {
		status = StatusFailed
		errMsg = err.Error()

		// Create specific messages based on error type
		if errors.Is(err, context.Canceled) {
			message = "File writing cancelled."
		} else if errors.Is(err, context.DeadlineExceeded) {
			message = "File writing timed out."
		} else {
			message = fmt.Sprintf("File writing failed: %v", err)
		}
	} else {
		status = StatusSucceeded
		errMsg = ""
		message = fmt.Sprintf("File writing finished successfully to '%s' in %v.",
			filePath, duration.Round(time.Millisecond))
	}

	return OutputResult{
		CommandID:   cmdID,
		CommandType: CmdFileWrite,
		Status:      status,
		Message:     message,
		Error:       errMsg,
	}
}

// writeFileContent writes the given content to a file at the specified path.
// It creates the file if it doesn't exist or truncates it if it does.
// The function checks the context before writing to handle cancellation properly.
// Returns an error if the file cannot be opened, written to, or closed properly,
// or if the context is cancelled during execution.
func writeFileContent(ctx context.Context, filePath, content string) error {
	// Open the file for writing (create if not exists, truncate if exists)
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open/create file '%s' for writing: %w", filePath, err)
	}

	// Always close the file even if writing fails
	defer func() {
		file.Close()
	}()

	// Check context before writing to handle cancellation
	if err := checkContext(ctx); err != nil {
		return err
	}

	// Write content to the file
	contentBytes := []byte(content)
	n, err := file.Write(contentBytes)
	if err != nil {
		return fmt.Errorf("failed to write content to file '%s': %w", filePath, err)
	}

	// Verify that all bytes were written
	if n != len(contentBytes) {
		return fmt.Errorf("incomplete write to file '%s': wrote %d bytes, expected %d",
			filePath, n, len(contentBytes))
	}

	return nil
}
