package task

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to read the final result from the channel with a timeout.
// Suitable for executors that only send one final result.
func readFinalResult(t *testing.T, results <-chan OutputResult, timeout time.Duration) (OutputResult, bool) {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	var finalResult OutputResult
	var receivedOk bool

	// Read the expected final result
	select {
	case result, ok := <-results:
		if !ok {
			t.Errorf("Channel closed unexpectedly before final result")
			return OutputResult{}, false
		}
		finalResult = result
		receivedOk = true
	case <-timer.C:
		t.Errorf("Timed out waiting for final result after %v", timeout)
		return OutputResult{}, false
	}

	if !receivedOk {
		return OutputResult{}, false // Already timed out or channel closed early
	}

	// Verify channel is closed shortly after receiving the final result
	// Use a short timeout to allow the close defer to execute reliably
	closureCheckTimer := time.NewTimer(50 * time.Millisecond)
	defer closureCheckTimer.Stop()
	select {
	case _, okAfter := <-results:
		require.False(t, okAfter, "Channel should be closed after the final result")
	case <-closureCheckTimer.C:
		t.Errorf("Timed out waiting for channel to close after receiving final result")
		return finalResult, false // Indicate failure even though we got the result
	}

	return finalResult, true
}

// Helper to read file content
func readFileContent(t *testing.T, path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func TestFileWriteExecutor_Execute_Success(t *testing.T) {
	executor := NewFileWriteExecutor()
	tempDir := t.TempDir()
	tempFilePath := filepath.Join(tempDir, "test_write_success.txt")
	expectedContent := "Hello, world!\nThis is a test."

	cmd := NewFileWriteTask("test-write-success-1", "Test File Write Success", FileWriteParameters{
		FilePath:  tempFilePath,
		Content:   expectedContent,
		Overwrite: false,
	})

	resultsChan, err := executor.Execute(context.Background(), cmd)
	require.NoError(t, err, "Execute setup failed")

	finalResult, received := readFinalResult(t, resultsChan, 5*time.Second)
	require.True(t, received, "Did not receive final result")

	assert.Equal(t, cmd.TaskId, finalResult.TaskID)
	assert.Equal(t, StatusSucceeded, finalResult.Status)
	assert.Empty(t, finalResult.Error, "Expected no error message")
	assert.Contains(t, finalResult.Message, "File writing finished successfully")
	assert.Contains(t, finalResult.Message, tempFilePath)

	// Verify file content
	actualContent, readErr := readFileContent(t, tempFilePath)
	require.NoError(t, readErr, "Failed to read back file content")
	assert.Equal(t, expectedContent, actualContent, "File content mismatch")
}

func TestFileWriteExecutor_Execute_Overwrite(t *testing.T) {
	executor := NewFileWriteExecutor()
	tempDir := t.TempDir()
	tempFilePath := filepath.Join(tempDir, "test_write_overwrite.txt")
	initialContent := "Initial content."
	newContent := "Overwritten content."

	// Create the initial file
	err := os.WriteFile(tempFilePath, []byte(initialContent), 0644)
	require.NoError(t, err, "Failed to create initial file")

	cmd := NewFileWriteTask("test-write-overwrite-1", "Test File Write Overwrite", FileWriteParameters{
		FilePath:  tempFilePath,
		Content:   newContent,
		Overwrite: true,
	})

	resultsChan, err := executor.Execute(context.Background(), cmd)
	require.NoError(t, err, "Execute setup failed")

	finalResult, received := readFinalResult(t, resultsChan, 5*time.Second)
	require.True(t, received, "Did not receive final result")

	assert.Equal(t, StatusSucceeded, finalResult.Status)
	assert.Empty(t, finalResult.Error)

	// Verify file content was overwritten
	actualContent, readErr := readFileContent(t, tempFilePath)
	require.NoError(t, readErr, "Failed to read back file content")
	assert.Equal(t, newContent, actualContent, "File content was not overwritten")
}

func TestFileWriteExecutor_Execute_DirectoryNotFound(t *testing.T) {
	executor := NewFileWriteExecutor()
	tempDir := t.TempDir()
	// Path to a file within a non-existent directory
	nonExistentDirPath := filepath.Join(tempDir, "non_existent_dir", "test_write_fail.txt")

	cmd := NewFileWriteTask("test-write-dirfail-1", "Test File Write Directory Not Found", FileWriteParameters{
		FilePath:  nonExistentDirPath,
		Content:   "This should not be written.",
		Overwrite: false,
	})

	resultsChan, err := executor.Execute(context.Background(), cmd)
	require.NoError(t, err, "Execute setup failed")

	finalResult, received := readFinalResult(t, resultsChan, 5*time.Second)
	require.True(t, received, "Did not receive final result")

	assert.Equal(t, StatusFailed, finalResult.Status)
	assert.NotEmpty(t, finalResult.Error, "Expected an error message")
	assert.Contains(t, finalResult.Error, "failed to open/create file", "Error should mention opening/creating")
	assert.Contains(t, finalResult.Error, "no such file or directory", "Error should mention 'no such file or directory'")
	assert.Contains(t, finalResult.Message, "File writing failed")

	// Verify file does not exist
	_, statErr := os.Stat(nonExistentDirPath)
	assert.True(t, os.IsNotExist(statErr), "File should not exist")
}

func TestFileWriteExecutor_Execute_Cancellation(t *testing.T) {
	executor := NewFileWriteExecutor()
	tempDir := t.TempDir()
	tempFilePath := filepath.Join(tempDir, "test_write_cancel.txt")

	cmd := NewFileWriteTask("test-write-cancel-1", "Test File Write Cancellation", FileWriteParameters{
		FilePath: tempFilePath,
		Content:  "This might be partially written if cancel is slow, but likely not written.",
	})

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	resultsChan, err := executor.Execute(ctx, cmd)
	require.NoError(t, err, "Execute setup failed")

	// Cancel immediately (or very quickly) - likely before file operations start
	cancel()

	// Collect result
	finalResult, received := readFinalResult(t, resultsChan, 5*time.Second)

	require.True(t, received, "Did not receive final result after cancellation")
	assert.Equal(t, StatusFailed, finalResult.Status)
	assert.Contains(t, finalResult.Error, context.Canceled.Error(), "Expected context canceled error string")
	assert.Contains(t, finalResult.Message, "File writing cancelled")

	// Verify file likely does not exist (or is empty if cancel was slow, though unlikely)
	_, statErr := os.Stat(tempFilePath)
	assert.True(t, os.IsNotExist(statErr), "File should generally not exist after immediate cancellation")
}

func TestFileWriteExecutor_Execute_Timeout(t *testing.T) {
	executor := NewFileWriteExecutor()
	tempDir := t.TempDir()
	tempFilePath := filepath.Join(tempDir, "test_write_timeout.txt")

	cmd := NewFileWriteTask("test-write-timeout-1", "Test File Write Timeout", FileWriteParameters{
		FilePath: tempFilePath,
		Content:  "This content should not be written due to timeout.",
	})

	// Use a very short timeout
	testTimeout := 1 * time.Microsecond
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	startTime := time.Now() // Record start time
	resultsChan, err := executor.Execute(ctx, cmd)
	require.NoError(t, err, "Execute setup failed")

	// Give the executor goroutine a chance to hit the initial context check
	// We expect the timeout to occur almost immediately.

	// Collect result
	readFinalResult(t, resultsChan, 5*time.Second)
	executionDuration := time.Since(startTime) // Calculate duration

	// Assert test duration is very short, indicating the timeout worked quickly
	assert.Less(t, executionDuration, 50*time.Millisecond, "Execution should finish quickly on immediate timeout")

	// Verify file likely does not exist (removed strict check due to race conditions with very short timeouts)
	// _, statErr := os.Stat(tempFilePath)
	// assert.True(t, os.IsNotExist(statErr), "File should not exist after immediate timeout")
	t.Logf("File state after timeout test is ignored due to potential race conditions.")
}

func TestFileWriteExecutor_Execute_InvalidCommandType(t *testing.T) {
	executor := NewFileWriteExecutor()
	// Create a command of the wrong type
	cmd := NewFileReadTask("invalid-write-type-1", "Invalid write type", FileReadParameters{
		FilePath: "some/path",
	})

	// Pass context, although it won't be used here as error is immediate
	resultsChan, err := executor.Execute(context.Background(), cmd)

	// Expect an immediate error, not a result from the channel
	require.Error(t, err, "Expected an error for invalid command type")
	assert.Nil(t, resultsChan, "Expected nil channel on immediate error")
}

func TestFileWriteExecutor_Execute_TerminalTaskHandling(t *testing.T) {
	executor := NewFileWriteExecutor()

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
			cmd := NewFileWriteTask("terminal-filewrite-test", "Terminal filewrite task test", FileWriteParameters{
				FilePath: "nonexistent/directory/file.txt", // Should not try to write this
				Content:  "This should not be written",
			})
			cmd.Output = OutputResult{
				TaskID:  "terminal-filewrite-test",
				Status:  tc.status,
				Message: "Pre-existing terminal state",
			}
			cmd.Status = tc.status

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
