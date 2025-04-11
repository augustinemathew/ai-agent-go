package command

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
				return finalResult, outputBuilder.String(), finalResult.CommandID != ""
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
	executor := NewFileReadExecutor()
	expectedContent := "This is a test file.\nWith multiple lines."
	tempFilePath := createTempFile(t, expectedContent)

	cmd := FileReadCommand{
		BaseCommand: BaseCommand{CommandID: "test-read-success-1"},
		FilePath:    tempFilePath,
	}

	resultsChan, err := executor.Execute(context.Background(), cmd)
	require.NoError(t, err, "Execute setup failed")

	finalResult, combinedOutput, received := collectStreamingResults_FileRead(t, resultsChan, 5*time.Second)
	require.True(t, received, "Did not receive final result")

	assert.Equal(t, cmd.CommandID, finalResult.CommandID)
	assert.Equal(t, CmdFileRead, finalResult.CommandType)
	assert.Equal(t, StatusSucceeded, finalResult.Status)
	assert.Empty(t, finalResult.Error, "Expected no error message")
	assert.Contains(t, finalResult.Message, "File reading finished successfully in")
	assert.Equal(t, expectedContent, combinedOutput, "Combined output does not match file content")
}

func TestFileReadExecutor_Execute_Success_MultiChunk(t *testing.T) {
	executor := NewFileReadExecutor()
	// Create a file larger than one chunk (e.g., 15KB)
	fileSize := 15 * 1024
	tempFilePath, expectedContent := createLargeTempFile(t, fileSize)

	cmd := FileReadCommand{
		BaseCommand: BaseCommand{CommandID: "test-read-multichunk-1"},
		FilePath:    tempFilePath,
	}

	resultsChan, err := executor.Execute(context.Background(), cmd)
	require.NoError(t, err, "Execute setup failed")

	// Use a longer timeout for potentially larger file reads
	finalResult, combinedOutput, received := collectStreamingResults_FileRead(t, resultsChan, 10*time.Second)
	require.True(t, received, "Did not receive final result")

	assert.Equal(t, cmd.CommandID, finalResult.CommandID)
	assert.Equal(t, CmdFileRead, finalResult.CommandType)
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

	cmd := FileReadCommand{
		BaseCommand: BaseCommand{CommandID: "test-read-empty-1"},
		FilePath:    tempFilePath,
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

	cmd := FileReadCommand{
		BaseCommand: BaseCommand{CommandID: "test-read-notfound-1"},
		FilePath:    nonExistentPath,
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

	cmd := FileReadCommand{
		BaseCommand: BaseCommand{CommandID: "test-read-cancel-1"},
		FilePath:    tempFilePath,
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

	cmd := FileReadCommand{
		BaseCommand: BaseCommand{CommandID: "test-read-timeout-1"},
		FilePath:    tempFilePath,
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
	cmd := BashExecCommand{
		BaseCommand: BaseCommand{CommandID: "invalid-read-type-1"},
		Command:     "echo hello",
	}

	// Pass context, although it won't be used here as error is immediate
	resultsChan, err := executor.Execute(context.Background(), cmd)

	// Expect an immediate error, not a result from the channel
	require.Error(t, err, "Expected an error for invalid command type")
	assert.Nil(t, resultsChan, "Expected nil channel on immediate error")
	assert.Contains(t, err.Error(), "invalid command type: expected FileReadCommand, got command.BashExecCommand")
}
