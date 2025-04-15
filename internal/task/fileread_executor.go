package task

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"ai-agent-v3/internal/task/fileutils"
)

const (
	// Error messages
	errInvalidCommandType = "invalid command type: expected FileReadCommand, got %T"
	errInvalidStartLine   = "invalid start line: %d (must be >= 0)"
	errInvalidEndLine     = "invalid end line: %d (must be >= 0)"
	errInvalidLineRange   = "invalid line range: start line %d is after end line %d"
	errFileOpenFailed     = "failed to open file '%s': %w"
	errFileTooShort       = "file has fewer lines than start line %d"
	errScanFailed         = "error scanning file: %w"

	// Status messages
	msgReadingCancelled = "File reading cancelled."
	msgReadingTimedOut  = "File reading timed out."
	msgReadingFailed    = "File reading failed: %v"
	msgReadingSucceeded = "File reading finished successfully in %v."
)

// FileReadExecutor handles the execution of FileReadCommand.
type FileReadExecutor struct {
	// Dependencies for reading files can be added here.
}

// NewFileReadExecutor creates a new FileReadExecutor.
func NewFileReadExecutor() *FileReadExecutor {
	return &FileReadExecutor{}
}

// Execute reads the file specified in the FileReadCommand, streaming its content.
// It expects the cmd argument to be of type *FileReadTask.
// Returns a channel for results and an error if the command type is wrong or execution setup fails.
// The execution respects cancellation signals from the passed context.Context.
func (e *FileReadExecutor) Execute(ctx context.Context, fileReadCmd *Task) (<-chan OutputResult, error) {
	if fileReadCmd.Type != TaskFileRead {
		return nil, fmt.Errorf(errInvalidCommandType, fileReadCmd)
	}

	// If the task is already in a terminal state, return it as is
	terminalChan, err := HandleTerminalTask(fileReadCmd.TaskId, fileReadCmd.Status, fileReadCmd.Output)
	if err != nil || terminalChan != nil {
		return terminalChan, err
	}

	results := make(chan OutputResult, 1)
	go e.executeFileRead(ctx, fileReadCmd, results)
	return results, nil
}

// executeFileRead handles the actual file reading process in a separate goroutine.
func (e *FileReadExecutor) executeFileRead(ctx context.Context, cmd *Task, results chan<- OutputResult) {
	defer close(results)

	// Update task status to Running
	cmd.Status = StatusRunning

	startTime := time.Now()
	var finalErr error

	defer func() {
		finalResult := e.createFinalResult(cmd, startTime, finalErr)

		// Update the task status and output
		cmd.Status = finalResult.Status
		cmd.UpdateOutput(&finalResult)

		// Send the result
		results <- finalResult
	}()

	if err := ctx.Err(); err != nil {
		finalErr = fmt.Errorf("context error before execution: %w", err)
		return
	}

	if err := validateLineNumbers(cmd.Parameters.(FileReadParameters)); err != nil {
		finalErr = fmt.Errorf("line number validation failed: %w", err)
		return
	}

	// Resolve the file path
	absPath, err := fileutils.ResolveFilePath(cmd.Parameters.(FileReadParameters).FilePath, cmd.Parameters.(FileReadParameters).WorkingDirectory)
	if err != nil {
		finalErr = fmt.Errorf("file path resolution failed: %w", err)
		return
	}

	file, err := os.Open(absPath)
	if err != nil {
		finalErr = fmt.Errorf(errFileOpenFailed, absPath, err)
		return
	}
	defer file.Close()

	if err := e.readAndStreamFile(ctx, cmd, file, results); err != nil {
		finalErr = fmt.Errorf("file reading failed: %w", err)
	}
}

// validateLineNumbers checks if the line number parameters are valid.
func validateLineNumbers(params FileReadParameters) error {
	if params.StartLine < 0 {
		return fmt.Errorf(errInvalidStartLine, params.StartLine)
	}
	if params.EndLine < 0 {
		return fmt.Errorf(errInvalidEndLine, params.EndLine)
	}
	if params.StartLine > 0 && params.EndLine > 0 && params.StartLine > params.EndLine {
		return fmt.Errorf(errInvalidLineRange, params.StartLine, params.EndLine)
	}
	return nil
}

// readAndStreamFile reads the file and streams its content to the results channel.
func (e *FileReadExecutor) readAndStreamFile(ctx context.Context, cmd *Task, file *os.File, results chan<- OutputResult) error {
	scanner := bufio.NewScanner(file)
	currentLine := 1

	// Skip to start line
	for currentLine < cmd.Parameters.(FileReadParameters).StartLine && scanner.Scan() {
		currentLine++
	}

	if currentLine < cmd.Parameters.(FileReadParameters).StartLine {
		return fmt.Errorf(errFileTooShort, cmd.Parameters.(FileReadParameters).StartLine)
	}

	// Read and stream lines
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context error during reading: %w", err)
		}

		if !scanner.Scan() {
			break
		}

		line := scanner.Text() + "\n"

		if cmd.Parameters.(FileReadParameters).EndLine > 0 && currentLine > cmd.Parameters.(FileReadParameters).EndLine {
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			results <- OutputResult{
				TaskID:     cmd.TaskId,
				Status:     StatusRunning,
				ResultData: line,
			}
		}

		currentLine++
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf(errScanFailed, err)
	}

	return nil
}

// createFinalResult creates the final OutputResult with appropriate status and message.
func (e *FileReadExecutor) createFinalResult(cmd *Task, startTime time.Time, finalErr error) OutputResult {
	var status TaskStatus
	var message string
	var errMsg string

	if finalErr != nil {
		status = StatusFailed
		errMsg = finalErr.Error()
		switch {
		case errors.Is(finalErr, context.Canceled):
			message = msgReadingCancelled
		case errors.Is(finalErr, context.DeadlineExceeded):
			message = msgReadingTimedOut
		default:
			message = fmt.Sprintf(msgReadingFailed, finalErr)
		}
	} else {
		status = StatusSucceeded
		message = fmt.Sprintf(msgReadingSucceeded, time.Since(startTime).Round(time.Millisecond))
	}

	return OutputResult{
		TaskID:  cmd.TaskId,
		Status:  status,
		Message: message,
		Error:   errMsg,
	}
}
