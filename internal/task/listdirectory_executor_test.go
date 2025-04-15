package task

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Re-use readFinalResult helper (defined in filewrite_executor_test.go or common test utils)

func TestListDirectoryExecutor_Execute_Success(t *testing.T) {
	executor := NewListDirectoryExecutor()
	tempDir := t.TempDir()

	// Create a subdirectory and a file inside the temp directory
	subDirName := "subdir1"
	fileName := "testfile.txt"
	filePath := filepath.Join(tempDir, fileName)
	subDirPath := filepath.Join(tempDir, subDirName)

	err := os.Mkdir(subDirPath, 0755)
	require.NoError(t, err, "Failed to create subdirectory")
	err = os.WriteFile(filePath, []byte("hello"), 0644)
	require.NoError(t, err, "Failed to create test file")

	// Get absolute path for assertion comparison
	absTempDir, err := filepath.Abs(tempDir)
	require.NoError(t, err)

	cmd := NewListDirectoryTask("test-list-success-1", "Test List Directory", ListDirectoryParameters{
		Path: tempDir,
	})

	resultsChan, err := executor.Execute(context.Background(), cmd)
	require.NoError(t, err, "Execute setup failed")

	finalResult, received := readFinalResult(t, resultsChan, 5*time.Second)
	require.True(t, received, "Did not receive final result")

	assert.Equal(t, cmd.TaskId, finalResult.TaskID)
	assert.Equal(t, StatusSucceeded, finalResult.Status)
	assert.Empty(t, finalResult.Error, "Expected no error message")
	assert.Contains(t, finalResult.Message, "Successfully listed directory")
	assert.Contains(t, finalResult.Message, tempDir) // Check original path in message

	// Verify listing data format and content
	require.NotEmpty(t, finalResult.ResultData, "ResultData should contain the listing")
	listing := finalResult.ResultData
	assert.Contains(t, listing, fmt.Sprintf("Listing for %s:", absTempDir)) // Check absolute path in listing header
	// Check for file entry details (flexible check for permissions/time/size)
	assert.Regexp(t, fmt.Sprintf(`\s*\[FILE\]\s+[-rwx]{10}\s+.*?\s+\d+\s+%s`, fileName), listing)
	// Check for dir entry details (flexible check for permissions/time/size)
	assert.Regexp(t, fmt.Sprintf(`\s*\[DIR\s+\]\s+d[-rwx]{9}\s+.*?\s+\d+\s+%s`, subDirName), listing)
}

func TestListDirectoryExecutor_Execute_EmptyDir(t *testing.T) {
	executor := NewListDirectoryExecutor()
	tempDir := t.TempDir() // Empty directory
	absTempDir, err := filepath.Abs(tempDir)
	require.NoError(t, err)

	cmd := NewListDirectoryTask("test-list-empty-1", "Test List Directory", ListDirectoryParameters{
		Path: tempDir,
	})

	resultsChan, err := executor.Execute(context.Background(), cmd)
	require.NoError(t, err, "Execute setup failed")

	finalResult, received := readFinalResult(t, resultsChan, 5*time.Second)
	require.True(t, received, "Did not receive final result")

	assert.Equal(t, StatusSucceeded, finalResult.Status)
	assert.Empty(t, finalResult.Error)
	assert.Contains(t, finalResult.Message, "Successfully listed directory")

	// Verify listing data shows only the header
	require.NotEmpty(t, finalResult.ResultData)
	lines := strings.Split(strings.TrimSpace(finalResult.ResultData), "\n")
	assert.Equal(t, 1, len(lines), "Expected only the header line for an empty directory")
	assert.Contains(t, lines[0], fmt.Sprintf("Listing for %s:", absTempDir))
}

func TestListDirectoryExecutor_Execute_NotFound(t *testing.T) {
	executor := NewListDirectoryExecutor()
	nonExistentPath := filepath.Join(t.TempDir(), "does_not_exist")

	cmd := NewListDirectoryTask("test-list-notfound-1", "Test List Directory", ListDirectoryParameters{
		Path: nonExistentPath,
	})

	resultsChan, err := executor.Execute(context.Background(), cmd)
	require.NoError(t, err, "Execute setup failed")

	finalResult, received := readFinalResult(t, resultsChan, 5*time.Second)
	require.True(t, received, "Did not receive final result")

	assert.Equal(t, StatusFailed, finalResult.Status)
	assert.NotEmpty(t, finalResult.Error)
	assert.Contains(t, finalResult.Error, "failed to read directory")
	// Note: os.ReadDir returns the initial error from getting abs path if that fails first.
	// So we check for either that or the read error.
	assert.Contains(t, finalResult.Error, "no such file or directory")
	assert.Contains(t, finalResult.Message, "Directory listing failed")
	assert.Empty(t, finalResult.ResultData)
}

func TestListDirectoryExecutor_Execute_NotADirectory(t *testing.T) {
	executor := NewListDirectoryExecutor()
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "a_file.txt")
	err := os.WriteFile(filePath, []byte("not a dir"), 0644)
	require.NoError(t, err, "Failed to create test file")

	cmd := NewListDirectoryTask("test-list-notadir-1", "Test List Directory", ListDirectoryParameters{
		Path: filePath, // Attempt to list a file
	})

	resultsChan, err := executor.Execute(context.Background(), cmd)
	require.NoError(t, err, "Execute setup failed")

	finalResult, received := readFinalResult(t, resultsChan, 5*time.Second)
	require.True(t, received, "Did not receive final result")

	assert.Equal(t, StatusFailed, finalResult.Status)
	assert.NotEmpty(t, finalResult.Error)
	assert.Contains(t, finalResult.Error, "failed to read directory")
	// Error message might vary slightly by OS (e.g., "not a directory", "invalid argument")
	// Check for a common part
	assert.Contains(t, strings.ToLower(finalResult.Error), "not a directory", "Error message should indicate it's not a directory")
	assert.Contains(t, finalResult.Message, "Directory listing failed")
	assert.Empty(t, finalResult.ResultData)
}

func TestListDirectoryExecutor_Execute_Cancellation(t *testing.T) {
	executor := NewListDirectoryExecutor()
	tempDir := t.TempDir()

	cmd := NewListDirectoryTask("test-list-cancel-1", "Test List Directory", ListDirectoryParameters{
		Path: tempDir,
	})

	ctx, cancel := context.WithCancel(context.Background())

	resultsChan, err := executor.Execute(ctx, cmd)
	require.NoError(t, err, "Execute setup failed")

	cancel() // Cancel immediately

	finalResult, received := readFinalResult(t, resultsChan, 5*time.Second)
	require.True(t, received, "Did not receive final result after cancellation")

	// As requested, don't strictly assert status, just that it finished.
	t.Logf("Cancellation test finished with status: %s, Error: %s", finalResult.Status, finalResult.Error)
	if finalResult.Status == StatusFailed {
		assert.Contains(t, finalResult.Error, context.Canceled.Error())
	}
}

func TestListDirectoryExecutor_Execute_Timeout(t *testing.T) {
	executor := NewListDirectoryExecutor()
	tempDir := t.TempDir()

	cmd := NewListDirectoryTask("test-list-timeout-1", "Test List Directory", ListDirectoryParameters{
		Path: tempDir,
	})

	testTimeout := 1 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	resultsChan, err := executor.Execute(ctx, cmd)
	require.NoError(t, err, "Execute setup failed")

	finalResult, received := readFinalResult(t, resultsChan, 5*time.Second)
	require.True(t, received, "Did not receive final result after timeout")

	// As requested, don't strictly assert status, just that it finished.
	t.Logf("Timeout test finished with status: %s, Error: %s", finalResult.Status, finalResult.Error)
	if finalResult.Status == StatusFailed {
		assert.Contains(t, finalResult.Error, context.DeadlineExceeded.Error())
	}
}

func TestListDirectoryExecutor_Execute_InvalidCommandType(t *testing.T) {
	executor := NewListDirectoryExecutor()
	// Create a command of the wrong type
	cmd := NewFileWriteTask("test-write-1", "Test File Write", FileWriteParameters{
		FilePath: "some/path",
		Content:  "test content",
	})

	resultsChan, err := executor.Execute(context.Background(), cmd)

	require.Error(t, err, "Expected an error for invalid command type")
	assert.Nil(t, resultsChan, "Expected nil channel on immediate error")
}

func TestListDirectoryExecutor_Execute_TerminalTaskHandling(t *testing.T) {
	executor := NewListDirectoryExecutor()

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
			cmd := NewListDirectoryTask("terminal-listdir-test", "Terminal listdirectory task test", ListDirectoryParameters{
				Path: "nonexistent/directory", // Should not try to list this
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
