package task

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test Helpers ---

// Reusing collectStreamingResults helper from bash_executor_test.go
// (Assuming it's either in the same package or copied/imported)
// Helper function to collect streaming results from channel with timeout
func collectStreamingResults_FileRead(t *testing.T, results <-chan OutputResult, timeout time.Duration) (finalResult OutputResult, combinedOutput string, received bool) {
	t.Helper()
	var outputBuilder strings.Builder
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case result, ok := <-results:
			if !ok {
				t.Logf("Result channel closed unexpectedly.")
				return finalResult, outputBuilder.String(), finalResult.TaskID != ""
			}
			finalResult = result
			if result.Status == StatusRunning {
				outputBuilder.WriteString(result.ResultData)
				if !timer.Stop() {
					<-timer.C // Drain timer if necessary
				}
				timer.Reset(timeout)
			} else {
				assert.Empty(t, result.ResultData, "Final result message should not contain ResultData")
				return finalResult, outputBuilder.String(), true
			}
		case <-timer.C:
			t.Errorf("Test timed out waiting for results after %v", timeout)
			return OutputResult{}, outputBuilder.String(), false
		}
	}
}

// Creates a temporary file with given content
func createTempFile(t *testing.T, content string) string {
	t.Helper()
	tempFile, err := os.CreateTemp(t.TempDir(), "testfile_*.txt")
	require.NoError(t, err)
	_, err = tempFile.WriteString(content)
	require.NoError(t, err)
	err = tempFile.Close()
	require.NoError(t, err)
	return tempFile.Name()
}

// Creates a temporary file with large content (repeating a string)
func createLargeTempFile(t *testing.T, approxSize int) (string, string) {
	t.Helper()
	contentChunk := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789\n"
	repeats := (approxSize / len(contentChunk)) + 1
	var fullContent strings.Builder
	for i := 0; i < repeats; i++ {
		fullContent.WriteString(contentChunk)
	}
	expectedContent := fullContent.String()
	filePath := createTempFile(t, expectedContent)
	return filePath, expectedContent
}

// --- Test Cases ---

func TestFileReadExecutor_Execute_Success(t *testing.T) {
	// Create a temporary file with content for test
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(tempFile, []byte("line 1\nline 2\nline 3\n"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	task := &FileReadTask{
		BaseTask: BaseTask{
			TaskId: "read-test-1",
		},
		Parameters: FileReadParameters{
			FilePath: tempFile,
		},
	}

	executor := NewFileReadExecutor()
	ctx := context.Background()

	resultChan, err := executor.Execute(ctx, task)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	// Collect results from the channel
	finalResult, _, received := collectStreamingResults_FileRead(t, resultChan, 5*time.Second)
	require.True(t, received, "Did not receive final result")

	if finalResult.Status != StatusSucceeded {
		t.Errorf("Execute failed with status %s: %s", finalResult.Status, finalResult.Error)
	}

	// Verify the content of the output
	if !strings.Contains(finalResult.Message, "finished successfully") {
		t.Errorf("Execute didn't return success message. Got: %s", finalResult.Message)
	}

	// Verify that the task's status was updated
	if task.Status != StatusSucceeded {
		t.Errorf("Task status was not updated: expected %s, got %s",
			StatusSucceeded, task.Status)
	}

	// Verify that the Output field was populated
	if task.Output.TaskID != task.TaskId {
		t.Errorf("Task Output.TaskID was not populated: expected %s, got %s",
			task.TaskId, task.Output.TaskID)
	}
}

func TestFileReadExecutor_Execute_Success_MultiChunk(t *testing.T) {
	executor := NewFileReadExecutor()
	// Create a file larger than one chunk (e.g., 15KB)
	fileSize := 15 * 1024
	tempFilePath, expectedContent := createLargeTempFile(t, fileSize)

	cmd := &FileReadTask{
		BaseTask: BaseTask{
			TaskId:      "test-read-multichunk-1",
			Description: "Test multi-chunk file read",
			Status:      StatusPending,
		},
		Parameters: FileReadParameters{
			FilePath: tempFilePath,
		},
	}

	resultsChan, err := executor.Execute(context.Background(), cmd)
	require.NoError(t, err, "Execute setup failed")

	// Use a longer timeout for potentially larger file reads
	finalResult, combinedOutput, received := collectStreamingResults_FileRead(t, resultsChan, 10*time.Second)
	require.True(t, received, "Did not receive final result")

	assert.Equal(t, cmd.TaskId, finalResult.TaskID)
	assert.Equal(t, StatusSucceeded, finalResult.Status)
	assert.Empty(t, finalResult.Error, "Expected no error message")
	assert.Contains(t, finalResult.Message, "File reading finished successfully in")

	// Verify the full content matches
	assert.Equal(t, expectedContent, combinedOutput, "Combined output does not match large file content")
	// A more detailed test could count the number of StatusRunning messages
	// but comparing the final content is usually sufficient.
}

func TestFileReadExecutor_Execute_EmptyFile(t *testing.T) {
	executor := NewFileReadExecutor()
	expectedContent := ""
	tempFilePath := createTempFile(t, expectedContent)

	cmd := &FileReadTask{
		BaseTask: BaseTask{
			TaskId:      "test-read-empty-1",
			Description: "Test empty file read",
			Status:      StatusPending,
		},
		Parameters: FileReadParameters{
			FilePath: tempFilePath,
		},
	}

	resultsChan, err := executor.Execute(context.Background(), cmd)
	require.NoError(t, err, "Execute setup failed")

	finalResult, combinedOutput, received := collectStreamingResults_FileRead(t, resultsChan, 5*time.Second)
	require.True(t, received, "Did not receive final result")

	assert.Equal(t, StatusSucceeded, finalResult.Status)
	assert.Empty(t, finalResult.Error, "Expected no error message")
	assert.Contains(t, finalResult.Message, "File reading finished successfully in")
	assert.Equal(t, expectedContent, combinedOutput, "Combined output should be empty for an empty file")
}

func TestFileReadExecutor_Execute_FileNotFound(t *testing.T) {
	executor := NewFileReadExecutor()
	nonExistentPath := filepath.Join(t.TempDir(), "non_existent_file.txt")

	cmd := &FileReadTask{
		BaseTask: BaseTask{
			TaskId:      "test-read-notfound-1",
			Description: "Test file not found",
			Status:      StatusPending,
		},
		Parameters: FileReadParameters{
			FilePath: nonExistentPath,
		},
	}

	resultsChan, err := executor.Execute(context.Background(), cmd)
	require.NoError(t, err, "Execute setup failed") // Setup itself shouldn't fail

	finalResult, combinedOutput, received := collectStreamingResults_FileRead(t, resultsChan, 5*time.Second)
	require.True(t, received, "Did not receive final result")

	assert.Equal(t, StatusFailed, finalResult.Status)
	assert.NotEmpty(t, finalResult.Error, "Expected an error message for file not found")
	assert.Contains(t, finalResult.Error, "failed to open file", "Error message should indicate failure to open")
	assert.Contains(t, finalResult.Error, "no such file or directory", "Error message should mention 'no such file'")
	assert.Contains(t, finalResult.Message, "File reading failed", "Final message should indicate failure")
	assert.Empty(t, combinedOutput, "Combined output should be empty on file not found error")
}

func TestFileReadExecutor_Execute_Cancellation(t *testing.T) {
	executor := NewFileReadExecutor()
	// Create a large file to ensure reading takes some time
	fileSize := 50 * 1024 // 50KB
	tempFilePath, expectedContent := createLargeTempFile(t, fileSize)

	cmd := &FileReadTask{
		BaseTask: BaseTask{
			TaskId:      "test-read-cancel-1",
			Description: "Test file read cancellation",
			Status:      StatusPending,
		},
		Parameters: FileReadParameters{
			FilePath: tempFilePath,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	resultsChan, err := executor.Execute(ctx, cmd)
	require.NoError(t, err, "Execute setup failed")

	// Let some initial results potentially come through, then cancel
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Collect results with a reasonable timeout
	finalResult, combinedOutput, received := collectStreamingResults_FileRead(t, resultsChan, 5*time.Second)
	require.True(t, received, "Did not receive final result after cancellation")

	assert.Equal(t, StatusFailed, finalResult.Status)
	assert.Contains(t, finalResult.Error, context.Canceled.Error(), "Expected context canceled error string")
	assert.Contains(t, finalResult.Message, "File reading cancelled", "Final message should indicate cancellation")

	// Check that we received *some* output, but not the full file
	// Cancel might happen before any output is sent, so don't require NotEmpty.
	// assert.NotEmpty(t, combinedOutput, "Expected some output before cancellation")
	assert.True(t, len(combinedOutput) < len(expectedContent), "Expected partial output (possibly empty), less than the full file content")
	if len(combinedOutput) > 0 {
		assert.True(t, strings.HasPrefix(expectedContent, combinedOutput), "Received output should be a prefix of the expected content")
	}
}

func TestFileReadExecutor_Execute_Timeout(t *testing.T) {
	executor := NewFileReadExecutor()
	fileSize := 55 * 1024
	tempFilePath, expectedContent := createLargeTempFile(t, fileSize)

	cmd := &FileReadTask{
		BaseTask: BaseTask{
			TaskId:      "test-read-timeout-1",
			Description: "Test file read timeout",
			Status:      StatusPending,
		},
		Parameters: FileReadParameters{
			FilePath: tempFilePath,
		},
	}

	testTimeout := 50 * time.Millisecond // Slightly longer timeout than 1ms to allow first chunk read
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	resultsChan, err := executor.Execute(ctx, cmd)
	require.NoError(t, err, "Execute setup failed")

	var firstResult *OutputResult
	var finalResult *OutputResult
	var combinedOutput strings.Builder

	// 1. Attempt to read the first result (could be data or immediate failure)
	select {
	case result, ok := <-resultsChan:
		require.True(t, ok, "Channel closed unexpectedly while reading first result")
		t.Logf("Received first result: Status=%s, DataLen=%d, Err='%s', Msg='%s'", result.Status, len(result.ResultData), result.Error, result.Message)
		firstResult = &result
		if result.Status != StatusRunning {
			finalResult = &result // Timeout likely happened before any read
		} else {
			combinedOutput.WriteString(result.ResultData)
		}
	case <-time.After(2 * time.Second): // Generous timeout to get the first result
		t.Fatal("Timed out waiting for the first result")
	}

	// If the first result was already the final failed status, verify and exit
	if finalResult != nil {
		require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded, "Context error should be DeadlineExceeded if first result is final failure")
		assert.Equal(t, StatusFailed, finalResult.Status)
		assert.Contains(t, finalResult.Error, context.DeadlineExceeded.Error())
		assert.Contains(t, finalResult.Message, "timed out")
		t.Logf("Timeout occurred before or during the first read attempt.")
		return
	}

	// 2. First result was StatusRunning. Now, block/wait longer than the timeout duration
	//    to ensure the context expires while we are not actively reading.
	blockDuration := testTimeout + 100*time.Millisecond
	t.Logf("First result was StatusRunning. Blocking for %v to ensure context expires...", blockDuration)
	time.Sleep(blockDuration)
	t.Logf("Finished blocking. Context should now be expired.")

	// Verify context is indeed expired now
	contextErr := ctx.Err()
	require.ErrorIs(t, contextErr, context.DeadlineExceeded, "Context error should be DeadlineExceeded after blocking")

	// 3. Attempt to read the *next* result, which should be the final FAILED status
	//    Drain any intermediate RUNNING messages that might have been buffered.
	readLoopTimeout := time.NewTimer(2 * time.Second) // Safety timeout for this read loop
	defer readLoopTimeout.Stop()
	for {
		select {
		case result, ok := <-resultsChan:
			require.True(t, ok, "Channel closed unexpectedly while draining/reading final result")
			t.Logf("Received result after block: Status=%s, DataLen=%d, Err='%s', Msg='%s'", result.Status, len(result.ResultData), result.Error, result.Message)
			if result.Status != StatusRunning {
				finalResult = &result // Found the final status
				goto Assertions       // Exit the select and loop
			}
			// It was a StatusRunning message, append data and continue loop
			combinedOutput.WriteString(result.ResultData)
		case <-readLoopTimeout.C:
			t.Fatal("Timed out waiting for the final FAILED result after blocking")
		}
	}

Assertions:
	// 4. Assertions on the final result
	require.NotNil(t, finalResult, "Did not receive the final result message")
	assert.Equal(t, StatusFailed, finalResult.Status, "Final status should be FAILED")
	assert.Contains(t, finalResult.Error, context.DeadlineExceeded.Error(), "Final error message should contain context deadline exceeded")
	assert.Contains(t, finalResult.Message, "timed out", "Final message should indicate timeout")

	// 5. Verify partial output (should contain first chunk + any buffered chunks before failure)
	actualOutput := combinedOutput.String()
	require.NotNil(t, firstResult, "Internal test error: firstResult should not be nil here")
	assert.True(t, len(actualOutput) >= len(firstResult.ResultData), "Combined output should at least contain the first chunk")
	assert.True(t, len(actualOutput) < len(expectedContent),
		"Expected partial output (length %d) to be less than full content (length %d)", len(actualOutput), len(expectedContent))
	assert.True(t, strings.HasPrefix(expectedContent, actualOutput), "Received output should be a prefix of the expected content")
}

func TestFileReadExecutor_Execute_InvalidCommandType(t *testing.T) {
	executor := NewFileReadExecutor()
	// Create a command of the wrong type
	cmd := BashExecTask{
		BaseTask: BaseTask{TaskId: "invalid-read-type-1"},
		Parameters: BashExecParameters{
			Command: "echo hello",
		},
	}

	// Pass context, although it won't be used here as error is immediate
	resultsChan, err := executor.Execute(context.Background(), cmd)

	// Expect an immediate error, not a result from the channel
	require.Error(t, err, "Expected an error for invalid command type")
	assert.Nil(t, resultsChan, "Expected nil channel on immediate error")
}

func TestFileReadExecutor_LineBasedReading(t *testing.T) {
	executor := NewFileReadExecutor()
	content := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\n"
	tempFilePath := createTempFile(t, content)

	tests := []struct {
		name        string
		startLine   int
		endLine     int
		expected    string
		expectError bool
	}{
		{
			name:      "read_from_line_2",
			startLine: 2,
			endLine:   0,
			expected:  "Line 2\nLine 3\nLine 4\nLine 5\n",
		},
		{
			name:      "read_lines_2-4",
			startLine: 2,
			endLine:   4,
			expected:  "Line 2\nLine 3\nLine 4\n",
		},
		{
			name:      "read_single_line",
			startLine: 3,
			endLine:   3,
			expected:  "Line 3\n",
		},
		{
			name:        "invalid_start_line",
			startLine:   -1,
			endLine:     0,
			expectError: true,
		},
		{
			name:        "invalid_end_line",
			startLine:   1,
			endLine:     -1,
			expectError: true,
		},
		{
			name:        "start_after_end",
			startLine:   3,
			endLine:     2,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &FileReadTask{
				BaseTask: BaseTask{
					TaskId:      "test-read-lines-" + tt.name,
					Description: "Test line-based file read",
					Status:      StatusPending,
				},
				Parameters: FileReadParameters{
					FilePath:  tempFilePath,
					StartLine: tt.startLine,
					EndLine:   tt.endLine,
				},
			}

			resultsChan, err := executor.Execute(context.Background(), cmd)
			require.NoError(t, err, "Execute setup failed")

			finalResult, combinedOutput, received := collectStreamingResults_FileRead(t, resultsChan, 5*time.Second)
			require.True(t, received, "Did not receive final result")

			if tt.expectError {
				assert.Equal(t, StatusFailed, finalResult.Status)
				assert.NotEmpty(t, finalResult.Error)
			} else {
				assert.Equal(t, StatusSucceeded, finalResult.Status)
				assert.Empty(t, finalResult.Error)
				assert.Equal(t, tt.expected, combinedOutput)
			}
		})
	}
}

func TestFileReadExecutor_ContextCancellation_FinalStatus(t *testing.T) {
	// Create a large test file to ensure reading takes some time
	fileSize := 50 * 1024 // 50KB
	tempFilePath, _ := createLargeTempFile(t, fileSize)

	ctx, cancel := context.WithCancel(context.Background())
	executor := NewFileReadExecutor()
	cmd := &FileReadTask{
		BaseTask: BaseTask{
			TaskId:      "test-cancel-final-status",
			Description: "Test file read cancellation final status",
			Status:      StatusPending,
		},
		Parameters: FileReadParameters{
			FilePath: tempFilePath,
		},
	}

	resultsChan, err := executor.Execute(ctx, cmd)
	require.NoError(t, err, "Execute setup failed")

	// Let some initial results come through, then cancel
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Collect all results until channel is closed
	var results []OutputResult
	for result := range resultsChan {
		results = append(results, result)
	}

	// Verify we received at least one result
	require.NotEmpty(t, results, "Expected at least one result")

	// Get the final result
	finalResult := results[len(results)-1]

	// Verify the final result has the correct status and message
	assert.Equal(t, StatusFailed, finalResult.Status, "Final status should be Failed")
	assert.Contains(t, finalResult.Error, context.Canceled.Error(), "Error should indicate context cancellation")
	assert.Contains(t, finalResult.Message, "File reading cancelled", "Message should indicate cancellation")
	assert.Equal(t, cmd.TaskId, finalResult.TaskID, "CommandID should match")
}

func TestFileReadExecutor_RelativePathHandling(t *testing.T) {
	executor := NewFileReadExecutor()

	// Create a temporary directory structure
	tempDir := t.TempDir()
	subDir := filepath.Join(tempDir, "subdir")
	err := os.Mkdir(subDir, 0755)
	require.NoError(t, err, "Failed to create subdirectory")

	// Create a test file in the subdirectory
	content := "Test content in subdirectory\n"
	fileName := "test.txt"
	filePath := filepath.Join(subDir, fileName)
	err = os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err, "Failed to create test file")

	tests := []struct {
		name          string
		workingDir    string
		filePath      string
		expectedError bool
	}{
		{
			name:          "absolute_path",
			workingDir:    "",
			filePath:      filePath,
			expectedError: false,
		},
		{
			name:          "relative_path_with_working_dir",
			workingDir:    subDir,
			filePath:      fileName,
			expectedError: false,
		},
		{
			name:          "relative_path_no_working_dir",
			workingDir:    "",
			filePath:      fileName,
			expectedError: true,
		},
		{
			name:          "relative_path_wrong_working_dir",
			workingDir:    tempDir,
			filePath:      fileName,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &FileReadTask{
				BaseTask: BaseTask{
					TaskId:      "test-relative-path-" + tt.name,
					Description: "Test relative path handling",
					Status:      StatusPending,
				},
				Parameters: FileReadParameters{
					BaseParameters: BaseParameters{
						WorkingDirectory: tt.workingDir,
					},
					FilePath: tt.filePath,
				},
			}

			resultsChan, err := executor.Execute(context.Background(), cmd)
			require.NoError(t, err, "Execute setup failed")

			finalResult, combinedOutput, received := collectStreamingResults_FileRead(t, resultsChan, 5*time.Second)
			require.True(t, received, "Did not receive final result")

			if tt.expectedError {
				assert.Equal(t, StatusFailed, finalResult.Status)
				assert.NotEmpty(t, finalResult.Error)
				assert.Empty(t, combinedOutput)
			} else {
				assert.Equal(t, StatusSucceeded, finalResult.Status)
				assert.Empty(t, finalResult.Error)
				assert.Equal(t, content, combinedOutput)
			}
		})
	}
}

func TestFileReadExecutor_Execute_TerminalTaskHandling(t *testing.T) {
	executor := NewFileReadExecutor()

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
			cmd := &FileReadTask{
				BaseTask: BaseTask{
					TaskId:      "terminal-fileread-test",
					Description: "Terminal fileread task test",
					Status:      tc.status,
					Output: OutputResult{
						TaskID:  "terminal-fileread-test",
						Status:  tc.status,
						Message: "Pre-existing terminal state",
					},
				},
				Parameters: FileReadParameters{
					FilePath: "nonexistent/file.txt", // Should not try to read this
				},
			}

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
