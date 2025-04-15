package task

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
		prompt        string
		taskId        string
		description   string
		expectError   bool
		expectMessage string
	}{
		{
			name:          "basic prompt",
			prompt:        "Please enter your name:",
			taskId:        "test-1",
			description:   "Test prompt",
			expectError:   false,
			expectMessage: "Please enter your name:",
		},
		{
			name:          "empty prompt",
			prompt:        "",
			taskId:        "test-2",
			description:   "Empty prompt",
			expectError:   false,
			expectMessage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Create the command with the prompt from the test case
			cmd := NewRequestUserInputTask(tt.taskId, tt.description, RequestUserInputParameters{
				Prompt: tt.prompt,
			})

			resultsChan, err := executor.Execute(ctx, cmd)
			require.NoError(t, err, "Execute should not return an error")

			// Collect results
			var finalResult OutputResult
			for result := range resultsChan {
				finalResult = result
			}

			// Verify the result
			assert.Equal(t, cmd.TaskId, finalResult.TaskID)
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
	resultsChan, err := executor.Execute(context.Background(), NewFileReadTask("test-invalid", "Invalid command type", FileReadParameters{
		FilePath: "test.txt",
	}))

	require.Error(t, err, "Should return error for invalid command type")
	assert.Nil(t, resultsChan, "Should not return a results channel")
	assert.Contains(t, err.Error(), "invalid command type", "Error message should indicate invalid command type")
}

func TestRequestUserInputExecutor_Execute_ContextCancellation(t *testing.T) {
	executor := NewRequestUserInputExecutor()
	cmd := NewRequestUserInputTask("test-cancel", "Test cancellation", RequestUserInputParameters{
		Prompt: "This should be cancelled",
	})

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
	assert.Equal(t, cmd.TaskId, finalResult.TaskID)
	assert.Equal(t, StatusSucceeded, finalResult.Status)
	assert.Equal(t, cmd.Parameters.(RequestUserInputParameters).Prompt, finalResult.Message)
	assert.Empty(t, finalResult.Error)
	assert.Empty(t, finalResult.ResultData)
}

func TestRequestUserInputExecutor_Execute_TerminalTaskHandling(t *testing.T) {
	executor := NewRequestUserInputExecutor()

	testCases := []struct {
		name           string
		status         TaskStatus
		expectedStatus TaskStatus
	}{
		{
			name:           "Already succeeded task",
			status:         StatusSucceeded,
			expectedStatus: StatusSucceeded,
		},
		{
			name:           "Already failed task",
			status:         StatusFailed,
			expectedStatus: StatusFailed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a task that's already in a terminal state
			cmd := NewRequestUserInputTask("terminal-userinput-test", "Terminal userinput task test", RequestUserInputParameters{
				Prompt: "This prompt should not be shown",
			})

			cmd.Status = tc.status
			cmd.UpdateOutput(&OutputResult{
				TaskID:  cmd.TaskId,
				Status:  tc.status,
				Message: "Pre-existing terminal state",
			})

			resultsChan, err := executor.Execute(context.Background(), cmd)
			require.NoError(t, err, "Execute should not return an error for terminal tasks")
			require.NotNil(t, resultsChan, "Result channel should not be nil")

			// Get the result from the channel
			var finalResult OutputResult
			select {
			case result, ok := <-resultsChan:
				require.True(t, ok, "Channel closed without receiving a result")
				finalResult = result
			case <-time.After(1 * time.Second):
				t.Fatal("Timed out waiting for result from terminal task")
			}

			// Check the result
			assert.Equal(t, cmd.TaskId, finalResult.TaskID, "TaskID should match")
			assert.Equal(t, tc.expectedStatus, finalResult.Status, "Status should remain unchanged")
			assert.Equal(t, "Pre-existing terminal state", finalResult.Message, "Message should be preserved")

			// Ensure the channel is closed
			_, ok := <-resultsChan
			assert.False(t, ok, "Channel should be closed after sending the result")
		})
	}
}
