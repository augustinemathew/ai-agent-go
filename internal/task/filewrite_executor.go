// Package command provides implementations for executing various types of commands
// in the AI agent backend. These commands are executed asynchronously and report
// their results through channels.
package task

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"ai-agent-v3/internal/task/fileutils"
)

// FileWriteResult represents the result of a file write operation
type FileWriteResult struct {
	FilePath string
}

// FileWriteExecutor handles the execution of FileWriteCommand.
// It manages file creation, writing content, and proper error handling.
type FileWriteExecutor struct{}

// NewFileWriteExecutor creates a new FileWriteExecutor.
func NewFileWriteExecutor() *FileWriteExecutor {
	return &FileWriteExecutor{}
}

// Execute implements the Executor interface for FileWriteCommand.
func (e *FileWriteExecutor) Execute(ctx context.Context, cmd any) (<-chan OutputResult, error) {
	var fileWriteCmd *FileWriteTask
	switch v := cmd.(type) {
	case *FileWriteTask:
		fileWriteCmd = v
	case FileWriteTask:
		fileWriteCmd = &v
	default:
		return nil, fmt.Errorf("invalid command type for FileWriteExecutor")
	}

	// Check if task is already in a terminal state
	terminalChan, err := HandleTerminalTask(fileWriteCmd.TaskId, fileWriteCmd.Status, fileWriteCmd.Output)
	if err != nil {
		return nil, err
	}
	if terminalChan != nil {
		return terminalChan, nil
	}

	// Create a channel for results
	results := make(chan OutputResult, 1)
	go func() {
		defer close(results)
		startTime := time.Now()

		// Check context before starting
		if err := ctx.Err(); err != nil {
			results <- createFinalResult(fileWriteCmd.TaskId, "", err, time.Since(startTime))
			return
		}

		// Resolve the file path
		resolvedPath, err := fileutils.ResolveFilePath(fileWriteCmd.Parameters.FilePath, fileWriteCmd.Parameters.WorkingDirectory)
		if err != nil {
			results <- createFinalResult(fileWriteCmd.TaskId, resolvedPath, fmt.Errorf("failed to resolve file path: %w", err), time.Since(startTime))
			return
		}

		// Check context before writing file
		if err := ctx.Err(); err != nil {
			results <- createFinalResult(fileWriteCmd.TaskId, resolvedPath, err, time.Since(startTime))
			return
		}

		// Write the file
		if err := writeFileContent(ctx, resolvedPath, fileWriteCmd.Parameters.Content); err != nil {
			results <- createFinalResult(fileWriteCmd.TaskId, resolvedPath, err, time.Since(startTime))
			return
		}

		results <- createFinalResult(fileWriteCmd.TaskId, resolvedPath, nil, time.Since(startTime))
	}()

	return results, nil
}

// createFinalResult constructs an OutputResult based on the error status,
// setting appropriate messages and status codes for the FileWriteCommand.
func createFinalResult(cmdID, filePath string, err error, duration time.Duration) OutputResult {
	var status TaskStatus
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
		TaskID:  cmdID,
		Status:  status,
		Message: message,
		Error:   errMsg,
	}
}

// writeFileContent writes the given content to a file at the specified path.
// It creates the file if it doesn't exist or truncates it if it does.
// The function checks the context before writing to handle cancellation properly.
// Returns an error if the file cannot be opened, written to, or closed properly,
// or if the context is cancelled during execution.
func writeFileContent(ctx context.Context, filePath, content string) error {
	// Check context before opening file
	if err := ctx.Err(); err != nil {
		return err
	}

	// Open the file for writing (create if not exists, truncate if exists)
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open/create file '%s': %w", filePath, err)
	}

	// Always close the file even if writing fails
	defer func() {
		file.Close()
	}()

	// Check context before writing
	if err := ctx.Err(); err != nil {
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
