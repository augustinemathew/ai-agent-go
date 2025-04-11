package command

import "context"

// CommandExecutor defines the interface for executing a specific type of command.
// Each command type (like BashExec, FileRead, etc.) will have its own implementation
// of this interface. The Execute method takes a command struct (as an any type, requiring type assertion
// within the implementation) and returns a channel that streams OutputResult updates.
// It returns an error immediately if the command cannot be initiated (e.g., invalid type).
type CommandExecutor interface {
	// Execute starts the command execution process.
	// It accepts a command struct (type `any`) which should be cast to the specific
	// command type the executor handles.
	// It returns a channel (`<-chan OutputResult`) through which execution status
	// and results are reported asynchronously.
	// An error is returned immediately if the command is invalid or cannot be started.
	Execute(ctx context.Context, cmd any) (<-chan OutputResult, error)
}
