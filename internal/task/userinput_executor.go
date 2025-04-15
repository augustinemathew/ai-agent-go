package task

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
	userInputCmd, ok := cmd.(RequestUserInputTask)
	if !ok {
		return nil, fmt.Errorf("invalid command type: %T", cmd)
	}

	// Check if task is already in a terminal state
	terminalChan, err := HandleTerminalTask(userInputCmd.TaskId, userInputCmd.Status, userInputCmd.Output)
	if err != nil {
		return nil, err
	}
	if terminalChan != nil {
		return terminalChan, nil
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
				TaskID:  userInputCmd.TaskId,
				Status:  StatusSucceeded,
				Message: userInputCmd.Parameters.Prompt,
			}
			return
		default:
		}

		// Return the prompt message as the result
		results <- OutputResult{
			TaskID:  userInputCmd.TaskId,
			Status:  StatusSucceeded,
			Message: userInputCmd.Parameters.Prompt,
		}
	}()

	return results, nil
}
