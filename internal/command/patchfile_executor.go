package command

import (
	"context"
	"fmt"
)

// PatchFileExecutor handles the execution of PatchFileCommand.
type PatchFileExecutor struct {
	// Dependencies for patching files can be added here.
}

// NewPatchFileExecutor creates a new PatchFileExecutor.
func NewPatchFileExecutor() *PatchFileExecutor {
	return &PatchFileExecutor{}
}

// Execute applies a patch to the file specified in the PatchFileCommand.
// It expects the cmd argument to be of type PatchFileCommand.
// Returns a channel for results and an error if the command type is wrong or execution setup fails.
// The passed context is currently unused but included for interface compliance.
func (e *PatchFileExecutor) Execute(ctx context.Context, cmd any) (<-chan OutputResult, error) {
	patchCmd, ok := cmd.(PatchFileCommand)
	if !ok {
		return nil, fmt.Errorf("invalid command type: expected PatchFileCommand, got %T", cmd)
	}

	results := make(chan OutputResult, 1)

	go func() {
		defer close(results)

		// Check for immediate cancellation before starting work
		select {
		case <-ctx.Done():
			results <- OutputResult{
				CommandID:   patchCmd.CommandID,
				CommandType: CmdPatchFile,
				Status:      StatusFailed,
				Message:     "File patching cancelled before start.",
				Error:       ctx.Err().Error(),
			}
			return
		default:
		}

		// TODO: Implement actual file patching logic here.
		// Should ideally check ctx.Done() periodically during patching if it's complex.
		results <- OutputResult{
			CommandID:   patchCmd.CommandID,
			CommandType: CmdPatchFile,
			Status:      StatusFailed,
			Message:     "File patching not yet implemented.",
			Error:       "Not implemented",
		}
	}()

	return results, nil
}
