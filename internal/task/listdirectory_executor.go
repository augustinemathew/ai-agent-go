package task

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ListDirectoryExecutor handles the execution of ListDirectoryCommand.
type ListDirectoryExecutor struct{}

// NewListDirectoryExecutor creates a new ListDirectoryExecutor.
func NewListDirectoryExecutor() *ListDirectoryExecutor {
	return &ListDirectoryExecutor{}
}

// Execute lists the contents of the directory specified in the ListDirectoryCommand.
// It expects the cmd argument to be of type ListDirectoryCommand.
// Returns a channel for results and an error if the command type is wrong or execution setup fails.
func (e *ListDirectoryExecutor) Execute(ctx context.Context, cmd any) (<-chan OutputResult, error) {
	listCmd, ok := cmd.(ListDirectoryTask)
	if !ok {
		return nil, fmt.Errorf("invalid command type: expected ListDirectoryCommand, got %T", cmd)
	}

	// Check if task is already in a terminal state
	terminalChan, err := HandleTerminalTask(listCmd.TaskId, listCmd.Status, listCmd.Output)
	if err != nil {
		return nil, err
	}
	if terminalChan != nil {
		return terminalChan, nil
	}

	results := make(chan OutputResult, 1) // Buffered channel for the single final result

	go func() {
		cmdID := listCmd.TaskId // For logging
		fmt.Printf("[%s] ListDirectory goroutine started for path: %s\n", cmdID, listCmd.Parameters.Path)
		startTime := time.Now()
		var finalErr error
		var directoryListing string

		// Defer closing the channel *after* the status send defer runs
		defer func() {
			fmt.Printf("[%s] ListDirectory goroutine closing results channel\n", cmdID)
			close(results)
		}()

		// Defer sending the final status message (runs *before* the channel close)
		defer func() {
			fmt.Printf("[%s] Deferred function executing. finalErr (before final check): %v\n", cmdID, finalErr)
			duration := time.Since(startTime)
			var finalStatus TaskStatus
			var errMsg string
			var message string
			effectiveErr := finalErr

			// Final context check, prioritizing context error if no primary error occurred
			if effectiveErr == nil {
				select {
				case <-ctx.Done():
					effectiveErr = ctx.Err()
					fmt.Printf("[%s] Deferred: Context detected as done *during* defer final check. Error: %v\n", cmdID, effectiveErr)
				default:
					fmt.Printf("[%s] Deferred: Context check within defer OK.\n", cmdID)
				}
			}

			// Determine final status
			if effectiveErr != nil {
				fmt.Printf("[%s] Deferred: effectiveErr is non-nil (%T: %v)\n", cmdID, effectiveErr, effectiveErr)
				finalStatus = StatusFailed
				errMsg = effectiveErr.Error()
				if errors.Is(effectiveErr, context.Canceled) {
					message = "Directory listing cancelled."
					fmt.Printf("[%s] Deferred: Detected Canceled\n", cmdID)
				} else if errors.Is(effectiveErr, context.DeadlineExceeded) {
					message = "Directory listing timed out."
					fmt.Printf("[%s] Deferred: Detected DeadlineExceeded\n", cmdID)
				} else {
					message = fmt.Sprintf("Directory listing failed: %v", effectiveErr)
					fmt.Printf("[%s] Deferred: Detected other error\n", cmdID)
				}
			} else {
				fmt.Printf("[%s] Deferred: effectiveErr is nil, reporting SUCCEEDED\n", cmdID)
				finalStatus = StatusSucceeded
				errMsg = ""
				message = fmt.Sprintf("Successfully listed directory '%s' in %v.", listCmd.Parameters.Path, duration.Round(time.Millisecond))
			}

			// Send final result
			fmt.Printf("[%s] Deferred: Sending final result: Status=%s, Msg='%s', Err='%s', DataLen=%d\n", cmdID, finalStatus, message, errMsg, len(directoryListing))
			results <- OutputResult{
				TaskID:     listCmd.TaskId,
				Status:     finalStatus,
				Message:    message,
				Error:      errMsg,
				ResultData: directoryListing, // Include listing data on success
			}
			fmt.Printf("[%s] Deferred: Final result sent (or attempted)\n", cmdID)
		}()

		// Check for immediate cancellation before starting work
		fmt.Printf("[%s] Checking initial context...\n", cmdID)
		select {
		case <-ctx.Done():
			finalErr = ctx.Err()
			fmt.Printf("[%s] Initial context check DONE. finalErr set to: %v\n", cmdID, finalErr)
			return
		default:
			fmt.Printf("[%s] Initial context check OK.\n", cmdID)
		}

		// Get absolute path
		absPath, err := filepath.Abs(listCmd.Parameters.Path)
		if err != nil {
			finalErr = fmt.Errorf("failed to get absolute path for '%s': %w", listCmd.Parameters.Path, err)
			fmt.Printf("[%s] Error getting absolute path. finalErr set to: %v\n", cmdID, finalErr)
			return
		}
		fmt.Printf("[%s] Absolute path resolved to: %s\n", cmdID, absPath)

		// Check context again before reading directory
		fmt.Printf("[%s] Checking context before reading directory...\n", cmdID)
		select {
		case <-ctx.Done():
			finalErr = ctx.Err()
			fmt.Printf("[%s] Context check DONE before read dir. finalErr set to: %v\n", cmdID, finalErr)
			return
		default:
			fmt.Printf("[%s] Context check OK before read dir.\n", cmdID)
		}

		// Read directory entries
		fmt.Printf("[%s] Reading directory entries for: %s\n", cmdID, absPath)
		entries, err := os.ReadDir(absPath)
		if err != nil {
			finalErr = fmt.Errorf("failed to read directory '%s': %w", absPath, err)
			fmt.Printf("[%s] Error reading directory. finalErr set to: %v\n", cmdID, finalErr)
			return
		}
		fmt.Printf("[%s] Successfully read %d directory entries.\n", cmdID, len(entries))

		// Format the listing
		var builder strings.Builder
		builder.WriteString(fmt.Sprintf("Listing for %s:\n", absPath))
		var detailErrors []string // Collect errors getting file info
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				detailErr := fmt.Sprintf("  [ERROR] %s: %v\n", entry.Name(), err)
				builder.WriteString(detailErr)
				detailErrors = append(detailErrors, detailErr)
				continue // Skip processing this entry further
			}

			entryType := "FILE"
			if entry.IsDir() {
				entryType = "DIR " // Add space for alignment
			}

			// Format: [TYPE] Permissions ModTime Size Name
			modTimeStr := info.ModTime().Format(time.RFC3339) // Consistent time format
			builder.WriteString(fmt.Sprintf("  [%s] %-10s %s %10d %s\n",
				entryType,
				info.Mode().String(), // Permissions (e.g., -rw-r--r--)
				modTimeStr,
				info.Size(), // Size in bytes
				entry.Name(),
			))
		}
		directoryListing = builder.String()

		// If any errors occurred while getting details, append them to finalErr
		if len(detailErrors) > 0 {
			warningMsg := fmt.Sprintf("encountered %d error(s) while getting file details: %s", len(detailErrors), strings.Join(detailErrors, "; "))
			if finalErr != nil {
				finalErr = fmt.Errorf("%w; additionally, %s", finalErr, warningMsg) // Append to existing error
			} else {
				// Treat detail errors as a warning if the main directory read succeeded
				// but still report the issue clearly in the message/data.
				// Alternatively, could set finalErr here to make it a failure.
				fmt.Printf("[%s] Warning: %s\n", cmdID, warningMsg)
				// Optionally append warning to ResultData or Message? For now, just log.
			}
		}

		// Operation completed successfully, finalErr remains nil (unless detail errors are treated as fatal)
		fmt.Printf("[%s] Directory listing formatted successfully.\n", cmdID)
	}()

	return results, nil
}
