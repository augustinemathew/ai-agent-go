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
	listCmd, ok := cmd.(*ListDirectoryTask)
	if !ok {
		return nil, fmt.Errorf("invalid command type: expected *ListDirectoryTask, got %T", cmd)
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
		startTime := time.Now()
		var finalErr error
		var directoryListing string

		// Defer closing the channel *after* the status send defer runs
		defer close(results)

		// Defer sending the final status message
		defer func() {
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
				default:
					// Context is still valid
				}
			}

			// Determine final status
			if effectiveErr != nil {
				finalStatus = StatusFailed
				errMsg = effectiveErr.Error()
				if errors.Is(effectiveErr, context.Canceled) {
					message = "Directory listing cancelled."
				} else if errors.Is(effectiveErr, context.DeadlineExceeded) {
					message = "Directory listing timed out."
				} else {
					message = fmt.Sprintf("Directory listing failed: %v", effectiveErr)
				}
			} else {
				finalStatus = StatusSucceeded
				errMsg = ""
				message = fmt.Sprintf("Successfully listed directory '%s' in %v.", listCmd.Parameters.Path, duration.Round(time.Millisecond))
			}

			// Send final result
			results <- OutputResult{
				TaskID:     listCmd.TaskId,
				Status:     finalStatus,
				Message:    message,
				Error:      errMsg,
				ResultData: directoryListing, // Include listing data on success
			}
		}()

		// Check for immediate cancellation before starting work
		select {
		case <-ctx.Done():
			finalErr = ctx.Err()
			return
		default:
			// Continue processing
		}

		// Get absolute path
		absPath, err := filepath.Abs(listCmd.Parameters.Path)
		if err != nil {
			finalErr = fmt.Errorf("failed to get absolute path for '%s': %w", listCmd.Parameters.Path, err)
			return
		}

		// Check context again before reading directory
		select {
		case <-ctx.Done():
			finalErr = ctx.Err()
			return
		default:
			// Continue processing
		}

		// Read directory entries
		entries, err := os.ReadDir(absPath)
		if err != nil {
			finalErr = fmt.Errorf("failed to read directory '%s': %w", absPath, err)
			return
		}

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
			}
			// For detail errors only, we still consider the operation successful
			// but include the warnings in the output
		}
	}()

	return results, nil
}
