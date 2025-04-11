package command

import (
	"context"
	"strings"
	"testing"
)

// MockExecutor is a simple mock for testing registry functionality.
type MockExecutor struct {
	Executed bool
}

func (m *MockExecutor) Execute(ctx context.Context, cmd any) (<-chan OutputResult, error) {
	m.Executed = true
	results := make(chan OutputResult)
	close(results) // Immediately close for simplicity in registry tests
	return results, nil
}

func TestMapRegistry_NewMapRegistry(t *testing.T) {
	r := NewMapRegistry()
	if r == nil {
		t.Fatal("NewMapRegistry returned nil")
	}
	if r.executors == nil {
		t.Fatal("NewMapRegistry did not initialize the executors map")
	}

	// After refactoring, the registry should be initialized with standard executors.
	expectedCount := 6 // Bash, FileRead, FileWrite, PatchFile, ListDir, RequestUserInput
	if len(r.executors) != expectedCount {
		t.Errorf("Expected initial executors map to contain %d standard executors, got size %d", expectedCount, len(r.executors))
	}
}

func TestMapRegistry_RegisterAndGetExecutor(t *testing.T) {
	r := NewMapRegistry() // Use the standard constructor which now pre-populates
	mockExec := &MockExecutor{}
	testCmdType := CommandType("TEST_CMD_OVERWRITE") // Use a unique name to avoid collision

	// Check initial state (optional, depends on test intent)
	initialCount := len(r.executors)

	// Test getting before registering (should fail for this specific type)
	_, err := r.GetExecutor(testCmdType)
	if err == nil {
		t.Errorf("Expected error when getting unregistered executor '%s', got nil", testCmdType)
	}

	// Register the executor
	r.Register(testCmdType, mockExec)

	// Verify count increased
	if len(r.executors) != initialCount+1 {
		t.Errorf("Expected executor count to be %d after registering, got %d", initialCount+1, len(r.executors))
	}

	// Test getting after registering
	executor, err := r.GetExecutor(testCmdType)
	if err != nil {
		t.Fatalf("GetExecutor failed for '%s' after registering: %v", testCmdType, err)
	}
	if executor == nil {
		t.Fatal("GetExecutor returned nil executor after registering")
	}

	// Check if it's the correct executor (using the mock's behavior)
	mockReturned, ok := executor.(*MockExecutor)
	if !ok {
		t.Fatalf("GetExecutor returned wrong type, expected *MockExecutor, got %T", executor)
	}

	// Verify it's the same instance (optional, but good for mocks)
	if mockReturned != mockExec {
		t.Error("GetExecutor returned a different instance than registered")
	}

	// Test overwriting
	newMockExec := &MockExecutor{}
	r.Register(testCmdType, newMockExec)    // Overwrite the previous mock
	if len(r.executors) != initialCount+1 { // Count should remain the same
		t.Errorf("Expected executor count to remain %d after overwriting, got %d", initialCount+1, len(r.executors))
	}

	executor, err = r.GetExecutor(testCmdType)
	if err != nil {
		t.Fatalf("GetExecutor failed for '%s' after re-registering: %v", testCmdType, err)
	}
	if executor != newMockExec {
		t.Error("GetExecutor did not return the overwritten executor instance")
	}
}

func TestMapRegistry_GetExecutor_Unregistered(t *testing.T) {
	r := NewMapRegistry()                                    // Use standard constructor
	unknownCmdType := CommandType("______UNKNOWN_CMD______") // Make it unlikely to collide

	_, err := r.GetExecutor(unknownCmdType)
	if err == nil {
		t.Fatal("Expected an error when getting an unregistered executor, but got nil")
	}
	// Optionally check the error message content
	expectedErrorSubstr := "no executor registered"
	if !strings.Contains(err.Error(), expectedErrorSubstr) {
		t.Errorf("Expected error message containing '%s', got '%s'", expectedErrorSubstr, err.Error())
	}
}
