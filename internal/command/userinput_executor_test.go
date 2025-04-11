package command

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestUserInputExecutor_Execute(t *testing.T) {
	executor := NewRequestUserInputExecutor()

	tests := []struct {
		name          string
		cmd           RequestUserInput
		expectError   bool
		expectMessage string
	}{
		{
			name: "basic prompt",
			cmd: RequestUserInput{
				BaseCommand: BaseCommand{
					CommandID:   "test-1",
					Description: "Test prompt",
				},
				Prompt: "Please enter your name:",
			},
			expectError:   false,
			expectMessage: "Please enter your name:",
		},
		{
			name: "empty prompt",
			cmd: RequestUserInput{
				BaseCommand: BaseCommand{
					CommandID:   "test-2",
					Description: "Empty prompt",
				},
				Prompt: "",
			},
			expectError:   false,
			expectMessage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			resultsChan, err := executor.Execute(ctx, tt.cmd)
			require.NoError(t, err, "Execute should not return an error")

			// Collect results
			var finalResult OutputResult
			for result := range resultsChan {
				finalResult = result
			}

			// Verify the result
			assert.Equal(t, tt.cmd.CommandID, finalResult.CommandID)
			assert.Equal(t, CmdRequestUserInput, finalResult.CommandType)
			assert.Equal(t, StatusSucceeded, finalResult.Status)
			assert.Equal(t, tt.expectMessage, finalResult.Message)
			assert.Empty(t, finalResult.Error)
			assert.Empty(t, finalResult.ResultData)
		})
	}
}

func TestRequestUserInputExecutor_Execute_InvalidCommandType(t *testing.T) {
	executor := NewRequestUserInputExecutor()

	// Try to execute a command of the wrong type
	resultsChan, err := executor.Execute(context.Background(), FileReadCommand{
		BaseCommand: BaseCommand{
			CommandID:   "test-invalid",
			Description: "Invalid command type",
		},
		FilePath: "test.txt",
	})

	require.Error(t, err, "Should return error for invalid command type")
	assert.Nil(t, resultsChan, "Should not return a results channel")
	assert.Contains(t, err.Error(), "invalid command type", "Error message should indicate invalid command type")
}

func TestRequestUserInputExecutor_Execute_ContextCancellation(t *testing.T) {
	executor := NewRequestUserInputExecutor()
	cmd := RequestUserInput{
		BaseCommand: BaseCommand{
			CommandID:   "test-cancel",
			Description: "Test cancellation",
		},
		Prompt: "This should be cancelled",
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	resultsChan, err := executor.Execute(ctx, cmd)
	require.NoError(t, err, "Execute should not return an error even when context is cancelled")

	// Collect results
	var finalResult OutputResult
	for result := range resultsChan {
		finalResult = result
	}

	// Verify the result
	assert.Equal(t, cmd.CommandID, finalResult.CommandID)
	assert.Equal(t, CmdRequestUserInput, finalResult.CommandType)
	assert.Equal(t, StatusSucceeded, finalResult.Status)
	assert.Equal(t, cmd.Prompt, finalResult.Message)
	assert.Empty(t, finalResult.Error)
	assert.Empty(t, finalResult.ResultData)
}
