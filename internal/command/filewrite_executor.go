package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

// FileWriteExecutor handles the execution of FileWriteCommand.
type FileWriteExecutor struct{}

// NewFileWriteExecutor creates a new FileWriteExecutor.
func NewFileWriteExecutor() *FileWriteExecutor {
	return &FileWriteExecutor{}
}

// Execute writes the content specified in the FileWriteCommand to the target file path.
// It expects the cmd argument to be of type FileWriteCommand.
// Returns a channel for results and an error if the command type is wrong or execution setup fails.
// The execution respects cancellation signals from the passed context.Context.
func (e *FileWriteExecutor) Execute(ctx context.Context, cmd any) (<-chan OutputResult, error) {
	fileWriteCmd, ok := cmd.(FileWriteCommand)
	if !ok {
		return nil, fmt.Errorf("invalid command type: expected FileWriteCommand, got %T", cmd)
	}

	// Buffered channel (size 1) for the final status. No intermediate results for write.
	results := make(chan OutputResult, 1)

	go func() {
		cmdID := fileWriteCmd.CommandID // For logging
		fmt.Printf("[%s] FileWrite goroutine started for path: %s\n", cmdID, fileWriteCmd.FilePath)
		startTime := time.Now()
		var finalErr error // Holds the primary error encountered during execution

		// Defer closing the channel *after* the status send defer runs
		defer func() {
			fmt.Printf("[%s] FileWrite goroutine closing results channel\n", cmdID)
			close(results)
		}()

		// Defer sending the final status message (this runs *before* the channel close)
		defer func() {
			fmt.Printf("[%s] Deferred function executing. finalErr (before final check): %v\n", cmdID, finalErr)
			duration := time.Since(startTime)
			var finalStatus ExecutionStatus
			var errMsg string
			var message string

			// Determine the primary error from the execution body
			effectiveErr := finalErr

			// If no primary error occurred during execution, perform a final check on the context
			// right before sending the result. This helps catch very fast timeouts/cancellations.
			if effectiveErr == nil {
				select {
				case <-ctx.Done():
					effectiveErr = ctx.Err() // Context became done just now
					fmt.Printf("[%s] Deferred: Context detected as done *during* defer final check. Error: %v\n", cmdID, effectiveErr)
				default:
					// Context still not done, proceed with success
					fmt.Printf("[%s] Deferred: Context check within defer OK.\n", cmdID)
				}
			}

			// Determine final status based on effectiveErr (potentially updated by context check)
			if effectiveErr != nil {
				fmt.Printf("[%s] Deferred: effectiveErr is non-nil (%T: %v)\n", cmdID, effectiveErr, effectiveErr)
				finalStatus = StatusFailed
				errMsg = effectiveErr.Error()
				if errors.Is(effectiveErr, context.Canceled) {
					message = "File writing cancelled."
					fmt.Printf("[%s] Deferred: Detected Canceled\n", cmdID)
				} else if errors.Is(effectiveErr, context.DeadlineExceeded) {
					message = "File writing timed out."
					fmt.Printf("[%s] Deferred: Detected DeadlineExceeded\n", cmdID)
				} else {
					message = fmt.Sprintf("File writing failed: %v", effectiveErr)
					fmt.Printf("[%s] Deferred: Detected other error\n", cmdID)
				}
			} else {
				fmt.Printf("[%s] Deferred: effectiveErr is nil, reporting SUCCEEDED\n", cmdID)
				finalStatus = StatusSucceeded
				errMsg = "" // Ensure empty on success
				message = fmt.Sprintf("File writing finished successfully to '%s' in %v.", fileWriteCmd.FilePath, duration.Round(time.Millisecond))
			}

			// Send final result
			fmt.Printf("[%s] Deferred: Sending final result: Status=%s, Msg='%s', Err='%s'\n", cmdID, finalStatus, message, errMsg)
			// Send directly. If the receiver isn't ready, it might block, but the channel close defer will eventually run.
			// The test has its own timeout for receiving.
			results <- OutputResult{
				CommandID:   fileWriteCmd.CommandID,
				CommandType: CmdFileWrite,
				Status:      finalStatus,
				Message:     message,
				Error:       errMsg,
				// No ResultData for final write status
			}
			fmt.Printf("[%s] Deferred: Final result sent (or attempted)\n", cmdID)
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
		}

		// Open the file for writing (create if not exists, truncate if exists)
		// Using 0644 permissions (owner read/write, group read, other read)
		fmt.Printf("[%s] Opening/Creating file for writing: %s\n", cmdID, fileWriteCmd.FilePath)
		file, err := os.OpenFile(fileWriteCmd.FilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			finalErr = fmt.Errorf("failed to open/create file '%s' for writing: %w", fileWriteCmd.FilePath, err)
			fmt.Printf("[%s] File open/create failed. finalErr set to: %v\n", cmdID, finalErr)
			return
		}
		// Ensure file is closed even if write fails
		closeErrLogged := false
		defer func() {
			fmt.Printf("[%s] Closing file: %s\n", cmdID, fileWriteCmd.FilePath)
			closeErr := file.Close()
			if closeErr != nil && finalErr == nil {
				// Only record close error if no other error occurred previously
				finalErr = fmt.Errorf("failed to close file '%s': %w", fileWriteCmd.FilePath, closeErr)
				fmt.Printf("[%s] File close failed. finalErr set to: %v\n", cmdID, finalErr)
				closeErrLogged = true
			} else if closeErr != nil {
				// Log close error but don't overwrite the original finalErr
				fmt.Printf("[%s] File close failed (original error prevails): %v\n", cmdID, closeErr)
			} else if !closeErrLogged {
				fmt.Printf("[%s] File closed successfully.\n", cmdID)
			}
		}()
		fmt.Printf("[%s] File opened successfully for writing.\n", cmdID)

		// Check context again right before writing
		fmt.Printf("[%s] Checking context before writing...\n", cmdID)
		select {
		case <-ctx.Done():
			finalErr = ctx.Err()
			fmt.Printf("[%s] Context check DONE before write. finalErr set to: %v\n", cmdID, finalErr)
			return
		default:
			fmt.Printf("[%s] Context check OK before write.\n", cmdID)
		}

		// Write the content
		contentBytes := []byte(fileWriteCmd.Content)
		fmt.Printf("[%s] Attempting to write %d bytes...\n", cmdID, len(contentBytes))
		n, err := file.Write(contentBytes)
		if err != nil {
			finalErr = fmt.Errorf("failed to write content to file '%s': %w", fileWriteCmd.FilePath, err)
			fmt.Printf("[%s] File write failed after writing %d bytes. finalErr set to: %v\n", cmdID, n, finalErr)
			return // Exit, finalErr is set
		}
		if n != len(contentBytes) {
			finalErr = fmt.Errorf("incomplete write to file '%s': wrote %d bytes, expected %d", fileWriteCmd.FilePath, n, len(contentBytes))
			fmt.Printf("[%s] File write incomplete. finalErr set to: %v\n", cmdID, finalErr)
			return // Exit, finalErr is set
		}

		fmt.Printf("[%s] Successfully wrote %d bytes.\n", cmdID, n)
		// finalErr remains nil for successful write
	}()

	return results, nil
}
