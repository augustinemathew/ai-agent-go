package task

import (
	"testing"
)

func TestBaseTask_UpdateOutput(t *testing.T) {
	tests := []struct {
		name           string
		taskStatus     TaskStatus
		outputStatus   TaskStatus
		expectedStatus TaskStatus
	}{
		{
			name:           "Same Status",
			taskStatus:     StatusSucceeded,
			outputStatus:   StatusSucceeded,
			expectedStatus: StatusSucceeded,
		},
		{
			name:           "Status Mismatch",
			taskStatus:     StatusFailed,
			outputStatus:   StatusSucceeded,
			expectedStatus: StatusFailed, // Output should be updated to match the task
		},
		{
			name:           "Task Pending",
			taskStatus:     StatusPending,
			outputStatus:   StatusRunning,
			expectedStatus: StatusPending, // Output should be updated to match the task
		},
		{
			name:           "Task Running",
			taskStatus:     StatusRunning,
			outputStatus:   StatusSucceeded,
			expectedStatus: StatusRunning, // Output should be updated to match the task
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			baseTask := &BaseTask{
				TaskId: "test-task",
				Status: tt.taskStatus,
				Type:   TaskBashExec,
			}

			output := &OutputResult{
				TaskID:  "test-task",
				Status:  tt.outputStatus,
				Message: "Test message",
			}

			// Act
			baseTask.UpdateOutput(output)

			// Assert
			if baseTask.Output.Status != tt.expectedStatus {
				t.Errorf("BaseTask.UpdateOutput() - Output status = %v, want %v", baseTask.Output.Status, tt.expectedStatus)
			}

			// Verify other fields are copied correctly
			if baseTask.Output.TaskID != "test-task" || baseTask.Output.Message != "Test message" {
				t.Errorf("BaseTask.UpdateOutput() - Other fields not copied correctly")
			}
		})
	}

	// Test nil output handling
	t.Run("Nil Output", func(t *testing.T) {
		baseTask := &BaseTask{
			TaskId: "test-task",
			Status: StatusRunning,
		}

		// This should not panic
		baseTask.UpdateOutput(nil)

		// Output should remain unchanged (empty)
		if baseTask.Output.TaskID != "" {
			t.Errorf("BaseTask.UpdateOutput(nil) modified the output when it shouldn't have")
		}
	})
}
