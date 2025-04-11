package command

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"time"
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
// It expects the cmd argument to be of type FileReadCommand.
// Returns a channel for results and an error if the command type is wrong or execution setup fails.
// The execution respects cancellation signals from the passed context.Context.
func (e *FileReadExecutor) Execute(ctx context.Context, cmd any) (<-chan OutputResult, error) {
	fileReadCmd, ok := cmd.(FileReadCommand)
	if !ok {
		return nil, fmt.Errorf("invalid command type: expected FileReadCommand, got %T", cmd)
	}

	// Buffered channel (size 1) for results + final status.
	results := make(chan OutputResult, 1)

	go func() {
		cmdID := fileReadCmd.CommandID // For logging
		fmt.Printf("[%s] FileRead goroutine started\n", cmdID)
		defer func() {
			fmt.Printf("[%s] FileRead goroutine closing results channel\n", cmdID)
			close(results)
		}()
		startTime := time.Now()
		var finalErr error // Holds the primary error encountered during execution

		// Defer sending the final status message
		defer func() {
			fmt.Printf("[%s] Deferred function executing. finalErr: %v\n", cmdID, finalErr)
			duration := time.Since(startTime)
			var finalStatus ExecutionStatus
			var errMsg string
			var message string

			// Determine final status based on finalErr captured during execution
			if finalErr != nil {
				fmt.Printf("[%s] Deferred: finalErr is non-nil (%T: %v)\n", cmdID, finalErr, finalErr)
				finalStatus = StatusFailed
				errMsg = finalErr.Error()
				if errors.Is(finalErr, context.Canceled) {
					message = "File reading cancelled."
					fmt.Printf("[%s] Deferred: Detected Canceled\n", cmdID)
				} else if errors.Is(finalErr, context.DeadlineExceeded) {
					message = "File reading timed out."
					fmt.Printf("[%s] Deferred: Detected DeadlineExceeded\n", cmdID)
				} else {
					message = fmt.Sprintf("File reading failed: %v", finalErr)
					fmt.Printf("[%s] Deferred: Detected other error\n", cmdID)
				}
			} else {
				fmt.Printf("[%s] Deferred: finalErr is nil\n", cmdID)
				finalStatus = StatusSucceeded
				errMsg = "" // Ensure empty on success
				message = fmt.Sprintf("File reading finished successfully in %v.", duration.Round(time.Millisecond))
			}

			// Send final result
			fmt.Printf("[%s] Deferred: Sending final result: Status=%s, Msg='%s', Err='%s'\n", cmdID, finalStatus, message, errMsg)
			results <- OutputResult{
				CommandID:   fileReadCmd.CommandID,
				CommandType: CmdFileRead,
				Status:      finalStatus,
				Message:     message,
				Error:       errMsg,
			}
			fmt.Printf("[%s] Deferred: Final result sent\n", cmdID)
		}()

		// Check for immediate cancellation before opening file
		fmt.Printf("[%s] Checking initial context...\n", cmdID)
		select {
		case <-ctx.Done():
			finalErr = ctx.Err() // Record error for deferred final message
			fmt.Printf("[%s] Initial context check DONE. finalErr set to: %v\n", cmdID, finalErr)
			return // Exit the goroutine
		default:
			fmt.Printf("[%s] Initial context check OK.\n", cmdID)
			// Continue if context is not done
		}

		// Validate line numbers
		if fileReadCmd.StartLine < 0 {
			finalErr = fmt.Errorf("invalid start line: %d (must be >= 0)", fileReadCmd.StartLine)
			return
		}
		if fileReadCmd.EndLine < 0 {
			finalErr = fmt.Errorf("invalid end line: %d (must be >= 0)", fileReadCmd.EndLine)
			return
		}
		if fileReadCmd.StartLine > 0 && fileReadCmd.EndLine > 0 && fileReadCmd.StartLine > fileReadCmd.EndLine {
			finalErr = fmt.Errorf("invalid line range: start line %d is after end line %d", fileReadCmd.StartLine, fileReadCmd.EndLine)
			return
		}

		// Open the file
		fmt.Printf("[%s] Opening file: %s\n", cmdID, fileReadCmd.FilePath)
		file, err := os.Open(fileReadCmd.FilePath)
		if err != nil {
			finalErr = fmt.Errorf("failed to open file '%s': %w", fileReadCmd.FilePath, err)
			fmt.Printf("[%s] File open failed. finalErr set to: %v\n", cmdID, finalErr)
			return
		}
		defer file.Close()
		fmt.Printf("[%s] File opened successfully.\n", cmdID)

		// Create a scanner to read line by line
		scanner := bufio.NewScanner(file)
		currentLine := 1 // Start from line 1 (1-based indexing)

		// Read lines until we reach the start line
		for currentLine < fileReadCmd.StartLine && scanner.Scan() {
			currentLine++
		}

		// Check if we reached EOF before start line
		if currentLine < fileReadCmd.StartLine {
			finalErr = fmt.Errorf("file has fewer lines than start line %d", fileReadCmd.StartLine)
			return
		}

		// Now read the requested lines
		for {
			// Check for cancellation before each line
			select {
			case <-ctx.Done():
				finalErr = ctx.Err()
				return
			default:
			}

			if !scanner.Scan() {
				break // EOF reached
			}

			line := scanner.Text() + "\n" // Add newline back since scanner strips it

			// Check if we've reached the end line
			if fileReadCmd.EndLine > 0 && currentLine > fileReadCmd.EndLine {
				break
			}

			// Send the line
			select {
			case <-ctx.Done():
				finalErr = ctx.Err()
				return
			case results <- OutputResult{
				CommandID:   fileReadCmd.CommandID,
				CommandType: CmdFileRead,
				Status:      StatusRunning,
				ResultData:  line,
			}:
			}

			currentLine++
		}

		if err := scanner.Err(); err != nil {
			finalErr = fmt.Errorf("error scanning file '%s': %w", fileReadCmd.FilePath, err)
			return
		}
	}()

	return results, nil
}
