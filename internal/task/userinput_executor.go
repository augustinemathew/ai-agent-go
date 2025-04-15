package task

import (
	"context"
	"fmt"
)

// Error constants for RequestUserInputExecutor
const (
	errUserInputInvalidCommandType = "invalid command type for RequestUserInputExecutor: %T"
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
// It expects the cmd argument to be of type *RequestUserInputTask.
// The actual user interaction mechanism is assumed to be handled elsewhere;
// this method just returns the prompt message.
func (e *RequestUserInputExecutor) Execute(ctx context.Context, userInputCmd *Task) (<-chan OutputResult, error) {
	// Type assertion to ensure we have a RequestUserInputTask command
	if userInputCmd.Type != TaskRequestUserInput {
		return nil, fmt.Errorf(errUserInputInvalidCommandType, userInputCmd)
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

		// Send the prompt message as the result, regardless of context state
		// Context cancellation is not really applicable for user input prompts
		// as they are essentially just messages being passed
		results <- OutputResult{
			TaskID:  userInputCmd.TaskId,
			Status:  StatusSucceeded,
			Message: userInputCmd.Parameters.(RequestUserInputParameters).Prompt,
		}
	}()

	return results, nil
}
