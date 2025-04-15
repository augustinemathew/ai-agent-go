package task_test

import (
	"ai-agent-v3/internal/task"
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// TestExecutorRegistrationAndRetrieval verifies that real executors can be
// registered and retrieved via the registry.
func TestExecutorRegistrationAndRetrieval(t *testing.T) {
	// Create registry - executors are now registered automatically.
	registry := task.NewMapRegistry()

	testCases := []struct {
		name         task.TaskType
		expectedType string // Store expected type as string for easier comparison
	}{
		{task.TaskBashExec, "*task.BashExecExecutor"},
		{task.TaskFileRead, "*task.FileReadExecutor"},
		{task.TaskFileWrite, "*task.FileWriteExecutor"},
		{task.TaskPatchFile, "*task.PatchFileExecutor"},
		{task.TaskListDirectory, "*task.ListDirectoryExecutor"},
		{task.TaskRequestUserInput, "*task.RequestUserInputExecutor"},
	}

	for _, tc := range testCases {
		t.Run(string(tc.name), func(t *testing.T) {
			executor, err := registry.GetExecutor(tc.name)
			if err != nil {
				t.Fatalf("GetExecutor failed for type %s: %v", tc.name, err)
			}
			if executor == nil {
				t.Fatalf("GetExecutor returned nil for type %s", tc.name)
			}

			// Basic type check
			actualType := fmt.Sprintf("%T", executor)
			if actualType != tc.expectedType {
				t.Errorf("Expected executor type %s, got %s", tc.expectedType, actualType)
			}
		})
	}
}

// TestBasicExecutorExecution is a very basic integration test to ensure
// Execute can be called via the interface without immediate panics or errors
// for a known simple command (like an empty FileWrite or similar).
// It does NOT thoroughly test the executor logic.
func TestBasicExecutorExecution(t *testing.T) {
	// Create registry - executors are now registered automatically.
	registry := task.NewMapRegistry()

	executor, err := registry.GetExecutor(task.TaskFileWrite)
	if err != nil {
		t.Fatalf("Failed to get FileWriteExecutor: %v", err)
	}

	// Use a command that should complete quickly and predictably (empty write)
	// Create a temporary file path
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "executor_test_empty_write.txt")

	cmd := task.NewFileWriteTask("exec-test-1", "Test File Write", task.FileWriteParameters{
		FilePath: tempFile,
		Content:  "", // Empty content
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resultsChan, err := executor.Execute(ctx, cmd) // Pass by pointer for FileWrite
	if err != nil {
		t.Fatalf("Execute returned an unexpected error: %v", err)
	}
	if resultsChan == nil {
		t.Fatal("Execute returned a nil channel")
	}

	// Consume the result to ensure the goroutine finishes
	finalResult := task.CombineOutputResults(ctx, resultsChan)

	// Basic check on the final status
	if finalResult.Status != task.StatusSucceeded {
		t.Errorf("Expected final status SUCCEEDED, got %s. Msg: %s, Err: %s",
			finalResult.Status, finalResult.Message, finalResult.Error)
	}

	// Verify that the task's status was updated
	if cmd.Status != task.StatusSucceeded {
		t.Errorf("Task status was not updated: expected %s, got %s",
			task.StatusSucceeded, cmd.Status)
	}

	// Verify that the Output field was populated
	if cmd.Output.TaskID != cmd.TaskId {
		t.Errorf("Task Output.TaskID was not populated: expected %s, got %s",
			cmd.TaskId, cmd.Output.TaskID)
	}
}
