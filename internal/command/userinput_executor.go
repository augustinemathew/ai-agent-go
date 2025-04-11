package command

import (
	"context"
	"fmt"
)

// RequestUserInputExecutor handles the execution of RequestUserInput.
type RequestUserInputExecutor struct {
	// Dependencies for handling user input requests can be added here.
}

// NewRequestUserInputExecutor creates a new RequestUserInputExecutor.
func NewRequestUserInputExecutor() *RequestUserInputExecutor {
	return &RequestUserInputExecutor{}
}

// Execute handles the request for user input specified in the RequestUserInput command.
// It expects the cmd argument to be of type RequestUserInput.
// The actual user interaction mechanism is assumed to be handled elsewhere;
// this executor primarily signals the need for input.
// Returns a channel for results and an error if the command type is wrong or execution setup fails.
// The passed context is currently unused but included for interface compliance.
func (e *RequestUserInputExecutor) Execute(ctx context.Context, cmd any) (<-chan OutputResult, error) {
	inputCmd, ok := cmd.(RequestUserInput)
	if !ok {
		return nil, fmt.Errorf("invalid command type: expected RequestUserInput, got %T", cmd)
	}

	results := make(chan OutputResult, 1)

	go func() {
		defer close(results)

		// Check for immediate cancellation before starting work
		select {
		case <-ctx.Done():
			results <- OutputResult{
				CommandID:   inputCmd.CommandID,
				CommandType: CmdRequestUserInput,
				Status:      StatusFailed,
				Message:     "User input request cancelled before start.",
				Error:       ctx.Err().Error(),
			}
			return
		default:
		}

		// TODO: Implement logic to trigger the user input prompt (e.g., send event).
		// The actual input collection happens elsewhere.
		// This executor might just confirm the request was sent.
		// Context check might be relevant if dispatching the request is slow.
		results <- OutputResult{
			CommandID:   inputCmd.CommandID,
			CommandType: CmdRequestUserInput,
			Status:      StatusFailed, // Or SUCCEEDED if the request dispatch is the success criteria
			Message:     "User input request handling not yet implemented.",
			Error:       "Not implemented",
		}
	}()

	return results, nil
}
