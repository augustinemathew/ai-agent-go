package task

import (
	"context"
	"fmt"
	"strings"
)

// CombineOutputResults reads all OutputResult messages from the provided channel
// until it closes or the provided context is cancelled.
// It returns a single OutputResult summarizing the execution.
//
// If the context is cancelled before the channel closes, it returns an OutputResult with:
// - Status: StatusFailed
// - Error: ctx.Err().Error()
// - ResultData: Concatenation of data received *before* cancellation.
// - Other fields: Copied from the *last* message received before cancellation, or zero values if none.
//
// If the channel closes normally, the returned OutputResult will have:
// - ResultData: Concatenation of all non-empty ResultData fields from all messages.
// - CommandID, CommandType, Status, Message, Error: Copied from the *last* message received.
// If no messages are received before close, a zero OutputResult is returned.
//
// This function blocks until the resultsChan is closed or the context is cancelled.
func CombineOutputResults(ctx context.Context, resultsChan <-chan OutputResult) OutputResult {
	var concatenatedData strings.Builder
	var lastMsg OutputResult
	lastMsg = OutputResult{} // Initialize for empty channel case

	for {
		select {
		case result, ok := <-resultsChan:
			if !ok {
				// Channel closed normally
				summaryResult := lastMsg
				summaryResult.ResultData = concatenatedData.String()
				return summaryResult
			}
			// Process received message
			if result.ResultData != "" {
				concatenatedData.WriteString(result.ResultData)
			}
			lastMsg = result // Keep track of the latest message

		case <-ctx.Done():
			// Context cancelled
			return OutputResult{
				TaskID:     lastMsg.TaskID, // Use ID from last message seen, if any
				Status:     StatusFailed,
				Message:    fmt.Sprintf("Result collection cancelled for command %s.", lastMsg.TaskID),
				Error:      ctx.Err().Error(),
				ResultData: concatenatedData.String(), // Include data collected so far
			}
		}
	}
}
