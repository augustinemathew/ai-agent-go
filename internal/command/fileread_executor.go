package command

import (
	"context"
	"errors"
	"fmt"
	"io"
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

const fileReadChunkSize = 10 * 1024 // 10KB chunk size

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

		// Read in chunks
		buffer := make([]byte, fileReadChunkSize)
		readLoopCounter := 0
		for {
			readLoopCounter++
			fmt.Printf("[%s] Read loop iteration %d: Checking context...\n", cmdID, readLoopCounter)
			// Check for cancellation before each read
			select {
			case <-ctx.Done():
				finalErr = ctx.Err() // Record error for deferred final message
				fmt.Printf("[%s] Read loop iteration %d: Context check DONE. finalErr set to: %v\n", cmdID, readLoopCounter, finalErr)
				return
			default:
				fmt.Printf("[%s] Read loop iteration %d: Context check OK.\n", cmdID, readLoopCounter)
			}

			fmt.Printf("[%s] Read loop iteration %d: Attempting file.Read()...\n", cmdID, readLoopCounter)
			n, err := file.Read(buffer)
			fmt.Printf("[%s] Read loop iteration %d: file.Read() returned n=%d, err=%v\n", cmdID, readLoopCounter, n, err)

			if n > 0 {
				fmt.Printf("[%s] Read loop iteration %d: Read %d bytes. Checking context before send...\n", cmdID, readLoopCounter, n)
				// Send the chunk read
				// Need to check context again before sending on channel
				select {
				case <-ctx.Done():
					finalErr = ctx.Err()
					fmt.Printf("[%s] Read loop iteration %d: Context check DONE while preparing send. finalErr set to: %v\n", cmdID, readLoopCounter, finalErr)
					return
				case results <- OutputResult{
					CommandID:   fileReadCmd.CommandID,
					CommandType: CmdFileRead,
					Status:      StatusRunning,
					ResultData:  string(buffer[:n]),
				}:
					fmt.Printf("[%s] Read loop iteration %d: Sent %d bytes chunk.\n", cmdID, readLoopCounter, n)
				}
			}

			if err == io.EOF {
				fmt.Printf("[%s] Read loop iteration %d: EOF reached.\n", cmdID, readLoopCounter)
				// Successfully reached end of file
				break // Exit loop cleanly, finalErr remains nil
			}
			if err != nil {
				// Handle other read errors
				finalErr = fmt.Errorf("error reading file '%s': %w", fileReadCmd.FilePath, err)
				fmt.Printf("[%s] Read loop iteration %d: Read error. finalErr set to: %v\n", cmdID, readLoopCounter, finalErr)
				return // Exit loop, finalErr is set
			}
		}
		fmt.Printf("[%s] Exited read loop normally.\n", cmdID)
	}()

	return results, nil
}
