package command_test

import (
	"ai-agent-v3/internal/command"
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
	registry := command.NewMapRegistry()

	testCases := []struct {
		name         command.CommandType
		expectedType string // Store expected type as string for easier comparison
	}{
		{command.CmdBashExec, "*command.BashExecExecutor"},
		{command.CmdFileRead, "*command.FileReadExecutor"},
		{command.CmdFileWrite, "*command.FileWriteExecutor"},
		{command.CmdPatchFile, "*command.PatchFileExecutor"},
		{command.CmdListDirectory, "*command.ListDirectoryExecutor"},
		{command.CmdRequestUserInput, "*command.RequestUserInputExecutor"},
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
	registry := command.NewMapRegistry()

	executor, err := registry.GetExecutor(command.CmdFileWrite)
	if err != nil {
		t.Fatalf("Failed to get FileWriteExecutor: %v", err)
	}

	// Use a command that should complete quickly and predictably (empty write)
	// Create a temporary file path
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "executor_test_empty_write.txt")

	cmd := command.FileWriteCommand{
		BaseCommand: command.BaseCommand{CommandID: "exec-test-1"},
		FilePath:    tempFile,
		Content:     "", // Empty content
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resultsChan, err := executor.Execute(ctx, cmd) // Pass by value ok for FileWrite
	if err != nil {
		t.Fatalf("Execute returned an unexpected error: %v", err)
	}
	if resultsChan == nil {
		t.Fatal("Execute returned a nil channel")
	}

	// Consume the result to ensure the goroutine finishes
	finalResult := command.CombineOutputResults(ctx, resultsChan)

	// Basic check on the final status
	if finalResult.Status != command.StatusSucceeded {
		t.Errorf("Expected final status SUCCEEDED, got %s. Msg: %s, Err: %s",
			finalResult.Status, finalResult.Message, finalResult.Error)
	}
}

// TODO: Add tests for individual executors and the executor interface.
