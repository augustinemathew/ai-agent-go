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
// this method just returns the prompt message.
func (e *RequestUserInputExecutor) Execute(ctx context.Context, cmd any) (<-chan OutputResult, error) {
	// Type assertion to ensure we have a RequestUserInput command
	userInputCmd, ok := cmd.(RequestUserInput)
	if !ok {
		return nil, fmt.Errorf("invalid command type: %T", cmd)
	}

	// Create a channel to receive the result
	results := make(chan OutputResult, 1)

	// Start a goroutine to handle the command execution
	go func() {
		defer close(results)

		// Check if context is already cancelled
		select {
		case <-ctx.Done():
			results <- OutputResult{
				CommandID:   userInputCmd.CommandID,
				CommandType: CmdRequestUserInput,
				Status:      StatusSucceeded,
				Message:     userInputCmd.Prompt,
			}
			return
		default:
		}

		// Return the prompt message as the result
		results <- OutputResult{
			CommandID:   userInputCmd.CommandID,
			CommandType: CmdRequestUserInput,
			Status:      StatusSucceeded,
			Message:     userInputCmd.Prompt,
		}
	}()

	return results, nil
}
