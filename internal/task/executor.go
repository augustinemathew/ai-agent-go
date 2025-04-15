package task

import (
	"context"
	"fmt"
)

// TaskExecutor defines the interface for executing a specific type of command.
// Each command type (like BashExec, FileRead, etc.) will have its own implementation
// of this interface. The Execute method takes a command struct (as an any type, requiring type assertion
// within the implementation) and returns a channel that streams OutputResult updates.
// It returns an error immediately if the command cannot be initiated (e.g., invalid type).
type TaskExecutor interface {
	// Execute starts the command execution process.
	// It accepts a command struct (type `any`) which should be cast to the specific
	// command type the executor handles.
	// It returns a channel (`<-chan OutputResult`) through which execution status
	// and results are reported asynchronously.
	// An error is returned immediately if the command is invalid or cannot be started.
	Execute(ctx context.Context, task *Task) (<-chan OutputResult, error)
}

// HandleTerminalTask checks if a task is in a terminal state (SUCCEEDED or FAILED)
// and if so, returns a channel with the task's Output.
// This helper function should be used by all executors to avoid executing tasks that are already complete.
// Returns:
// - A channel containing the task's Output and a nil error if the task is in a terminal state
// - nil and nil if the task is not in a terminal state and should be executed normally
func HandleTerminalTask(taskID string, status TaskStatus, output OutputResult) (<-chan OutputResult, error) {
	if status.IsTerminal() {
		// Task is already in a terminal state (SUCCEEDED or FAILED)
		// Create a channel and immediately send the existing output
		resultsChan := make(chan OutputResult, 1)

		go func() {
			defer close(resultsChan)

			// If output is empty (no TaskID), create a basic result with the task's status
			if output.TaskID == "" {
				output = OutputResult{
					TaskID:  taskID,
					Status:  status,
					Message: fmt.Sprintf("Task %s already in %s state", taskID, status),
				}
			}

			resultsChan <- output
		}()

		return resultsChan, nil
	}

	// Task is not in a terminal state, should proceed with normal execution
	return nil, nil
}
