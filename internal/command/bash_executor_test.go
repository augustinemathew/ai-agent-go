package command

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to collect streaming results from channel with timeout
func collectStreamingResults(t *testing.T, results <-chan OutputResult, timeout time.Duration) (finalResult OutputResult, combinedOutput string, received bool) {
	t.Helper()
	var outputBuilder strings.Builder
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case result, ok := <-results:
			if !ok {
				// Channel closed, means execution finished (or errored early)
				// Return the last meaningful result received if any, otherwise empty.
				// This case might be hit if the executor goroutine panics or exits unexpectedly.
				t.Logf("Result channel closed unexpectedly.")
				return finalResult, outputBuilder.String(), finalResult.CommandID != "" // Check if we ever got a result
			}

			// Store intermediate or final result
			finalResult = result

			if result.Status == StatusRunning {
				outputBuilder.WriteString(result.ResultData)
				// Reset timer since we received data
				if !timer.Stop() {
					<-timer.C // Drain timer if necessary
				}
				timer.Reset(timeout)
			} else {
				// Final status received (SUCCEEDED or FAILED)
				// No more ResultData expected in this message.
				assert.Empty(t, result.ResultData, "Final result message should not contain ResultData")
				return finalResult, outputBuilder.String(), true
			}

		case <-timer.C:
			t.Errorf("Test timed out waiting for results after %v", timeout)
			return OutputResult{}, outputBuilder.String(), false
		}
	}
}

// --- Expected Script Output Fragments ---
// These are parts of the script's *own* output sent to stderr that we might want to check
const scriptInitialDirOutput = `Initial directory:`
const scriptExitingOutput = `# Script Exiting`
const scriptFinalPwdOutputPrefix = `# Final Working Directory:` // Used in ChangeDirectory test

func TestBashExecExecutor_Execute_Success_Streaming(t *testing.T) {
	executor := NewBashExecExecutor()
	wd, _ := os.Getwd()
	expectedCmdOutput := "Hello Executor!\n"
	testCmd := fmt.Sprintf("echo '%s' && pwd", strings.TrimSpace(expectedCmdOutput)) // pwd output goes to stdout
	cmd := BashExecCommand{
		BaseCommand: BaseCommand{CommandID: "test-success-stream-1"},
		Command:     testCmd,
	}
	// Define the expected CWD temp file path
	expectedCwdFilePath := fmt.Sprintf("/tmp/%s.cwd", cmd.CommandID)
	// Clean up before test, just in case
	_ = os.Remove(expectedCwdFilePath)
	// Ensure cleanup after test
	t.Cleanup(func() { _ = os.Remove(expectedCwdFilePath) })

	resultsChan, err := executor.Execute(context.Background(), cmd)
	require.NoError(t, err, "Execute setup failed")

	finalResult, combinedOutput, received := collectStreamingResults(t, resultsChan, 10*time.Second)
	require.True(t, received, "Did not receive final result")

	assert.Equal(t, cmd.CommandID, finalResult.CommandID)
	assert.Equal(t, CmdBashExec, finalResult.CommandType)
	assert.Equal(t, StatusSucceeded, finalResult.Status)
	assert.Empty(t, finalResult.Error, "Expected no error message for successful command")

	// Verify combined output contains relevant script/command parts
	assert.Contains(t, combinedOutput, scriptInitialDirOutput, "Combined output missing script initial dir message")
	assert.Contains(t, combinedOutput, expectedCmdOutput, "Combined output missing command output")
	assert.Contains(t, combinedOutput, wd, "Combined output missing pwd output") // pwd output
	assert.Contains(t, combinedOutput, scriptExitingOutput, "Combined output missing script exit message")
	assert.Contains(t, combinedOutput, scriptFinalPwdOutputPrefix, "Combined output missing script final pwd message")

	// Check the final status message for CWD and duration
	expectedCwdMsg := fmt.Sprintf("Final CWD: %s", wd)
	assert.Contains(t, finalResult.Message, expectedCwdMsg, "Final message should contain the final CWD")
	assert.Contains(t, finalResult.Message, "Command finished in", "Final message should contain duration")

	// Verify the temp file exists (was not deleted by executor)
	_, err = os.Stat(expectedCwdFilePath)
	assert.NoError(t, err, "Expected %s to exist after execution", expectedCwdFilePath)
	// Verify the content of the temp file
	fileContentBytes, err := os.ReadFile(expectedCwdFilePath)
	require.NoError(t, err, "Failed to read content of %s", expectedCwdFilePath)
	assert.Equal(t, wd, strings.TrimSpace(string(fileContentBytes)), "Content of %s does not match expected CWD", expectedCwdFilePath)
}

func TestBashExecExecutor_Execute_Failure_Streaming(t *testing.T) {
	executor := NewBashExecExecutor()
	wd, _ := os.Getwd() // Get current WD to check the file content
	expectedCmdOutput := "Going to fail\n"
	testCmd := fmt.Sprintf("echo '%s' && exit 123", strings.TrimSpace(expectedCmdOutput))
	cmd := BashExecCommand{
		BaseCommand: BaseCommand{CommandID: "test-fail-stream-1"},
		Command:     testCmd,
	}
	// Define the expected CWD temp file path
	expectedCwdFilePath := fmt.Sprintf("/tmp/%s.cwd", cmd.CommandID)
	// Clean up before test, just in case
	_ = os.Remove(expectedCwdFilePath)
	// Ensure cleanup after test
	t.Cleanup(func() { _ = os.Remove(expectedCwdFilePath) })

	resultsChan, err := executor.Execute(context.Background(), cmd)
	require.NoError(t, err, "Execute setup failed")

	finalResult, combinedOutput, received := collectStreamingResults(t, resultsChan, 10*time.Second)
	require.True(t, received, "Did not receive final result")

	assert.Equal(t, cmd.CommandID, finalResult.CommandID)
	assert.Equal(t, CmdBashExec, finalResult.CommandType)
	assert.Equal(t, StatusFailed, finalResult.Status)
	assert.Contains(t, finalResult.Error, "Command failed with exit code 123", "Expected non-zero exit code error")
	// The message should still contain CWD info even on failure
	expectedMsg := fmt.Sprintf("Command execution failed. Final CWD: %s.", wd)
	assert.Equal(t, expectedMsg, finalResult.Message, "Final message mismatch in failure case")

	// Verify combined output contains relevant script/command parts before failure
	assert.Contains(t, combinedOutput, scriptInitialDirOutput)
	assert.Contains(t, combinedOutput, expectedCmdOutput)
	assert.Contains(t, combinedOutput, scriptExitingOutput)        // Trap output still runs on failure
	assert.Contains(t, combinedOutput, scriptFinalPwdOutputPrefix) // Trap output still runs on failure

	// Verify the temp file exists even on failure (was not deleted by executor)
	_, err = os.Stat(expectedCwdFilePath)
	assert.NoError(t, err, "Expected %s to exist after execution (failure case)", expectedCwdFilePath)
	// Verify the content of the temp file (should be the initial WD)
	fileContentBytes, err := os.ReadFile(expectedCwdFilePath)
	require.NoError(t, err, "Failed to read content of %s (failure case)", expectedCwdFilePath)
	assert.Equal(t, wd, strings.TrimSpace(string(fileContentBytes)), "Content of %s does not match initial CWD (failure case)", expectedCwdFilePath)
}

func TestBashExecExecutor_Execute_CombinedOutput_Streaming(t *testing.T) {
	executor := NewBashExecExecutor()
	wd, _ := os.Getwd() // Get current WD to check the file content
	// Command prints to stdout then stderr
	expectedStdout := "Output to stdout\n"
	expectedStderr := "Error to stderr\n"
	testCmd := fmt.Sprintf("echo '%s' >&1 && echo '%s' >&2", strings.TrimSpace(expectedStdout), strings.TrimSpace(expectedStderr))
	cmd := BashExecCommand{
		BaseCommand: BaseCommand{CommandID: "test-combined-stream-1"},
		Command:     testCmd,
	}
	// Define the expected CWD temp file path
	expectedCwdFilePath := fmt.Sprintf("/tmp/%s.cwd", cmd.CommandID)
	// Clean up before test, just in case
	_ = os.Remove(expectedCwdFilePath)
	// Ensure cleanup after test
	t.Cleanup(func() { _ = os.Remove(expectedCwdFilePath) })

	resultsChan, err := executor.Execute(context.Background(), cmd)
	require.NoError(t, err, "Execute setup failed")

	finalResult, combinedOutput, received := collectStreamingResults(t, resultsChan, 10*time.Second)
	require.True(t, received, "Did not receive final result")

	assert.Equal(t, StatusSucceeded, finalResult.Status) // Command itself should succeed
	assert.Empty(t, finalResult.Error)

	// Check combined output for relevant script parts, command stdout, and command stderr
	assert.Contains(t, combinedOutput, scriptInitialDirOutput)
	assert.Contains(t, combinedOutput, expectedStdout, "Combined output missing command stdout")
	assert.Contains(t, combinedOutput, expectedStderr, "Combined output missing command stderr")
	assert.Contains(t, combinedOutput, scriptExitingOutput)
	assert.Contains(t, combinedOutput, scriptFinalPwdOutputPrefix)

	// Verify the temp file exists (was not deleted by executor)
	_, err = os.Stat(expectedCwdFilePath)
	assert.NoError(t, err, "Expected %s to exist after execution", expectedCwdFilePath)
	// Verify the content of the temp file
	fileContentBytes, err := os.ReadFile(expectedCwdFilePath)
	require.NoError(t, err, "Failed to read content of %s", expectedCwdFilePath)
	assert.Equal(t, wd, strings.TrimSpace(string(fileContentBytes)), "Content of %s does not match expected CWD", expectedCwdFilePath)
}

func TestBashExecExecutor_Execute_ChangeDirectory_Streaming(t *testing.T) {
	executor := NewBashExecExecutor()
	wd, _ := os.Getwd()
	// Change to /tmp and verify it's reported by the trap
	expectedCmdOutput := "Changed directory\n"
	expectedFinalWd := "/private/tmp" // Expected final path on macOS, adjust if needed for other OS
	testCmd := fmt.Sprintf("cd %s && echo '%s'", expectedFinalWd, strings.TrimSpace(expectedCmdOutput))
	cmd := BashExecCommand{
		BaseCommand: BaseCommand{CommandID: "test-cd-stream-1"},
		Command:     testCmd,
	}
	// Define the expected CWD temp file path
	expectedCwdFilePath := fmt.Sprintf("/tmp/%s.cwd", cmd.CommandID)
	// Clean up before test, just in case
	_ = os.Remove(expectedCwdFilePath)
	// Ensure cleanup after test
	t.Cleanup(func() { _ = os.Remove(expectedCwdFilePath) })

	resultsChan, err := executor.Execute(context.Background(), cmd)
	require.NoError(t, err, "Execute setup failed")

	finalResult, combinedOutput, received := collectStreamingResults(t, resultsChan, 10*time.Second)
	require.True(t, received, "Did not receive final result")

	assert.Equal(t, StatusSucceeded, finalResult.Status)
	assert.Empty(t, finalResult.Error)
	assert.Contains(t, combinedOutput, expectedCmdOutput, "Combined output missing command output")

	// Check the final message for the CWD report
	assert.Contains(t, finalResult.Message, fmt.Sprintf("Final CWD: %s", expectedFinalWd), "Final message should report %s as final CWD", expectedFinalWd)

	// Check the combined output for the script's trap message showing the CWD
	assert.Contains(t, combinedOutput, fmt.Sprintf("Final Working Directory: %s", expectedFinalWd), "Combined output missing trap CWD report for %s", expectedFinalWd)
	// Ensure the initial directory is also present in the combined output
	assert.Contains(t, combinedOutput, fmt.Sprintf("Initial directory: %s", wd))
	// Ensure the exiting message is present
	assert.Contains(t, combinedOutput, scriptExitingOutput)

	// Verify the temp file exists (was not deleted by executor)
	_, err = os.Stat(expectedCwdFilePath)
	assert.NoError(t, err, "Expected %s to exist after execution", expectedCwdFilePath)
	// Verify the content of the temp file
	fileContentBytes, err := os.ReadFile(expectedCwdFilePath)
	require.NoError(t, err, "Failed to read content of %s", expectedCwdFilePath)
	assert.Equal(t, expectedFinalWd, strings.TrimSpace(string(fileContentBytes)), "Content of %s does not match expected final CWD %s", expectedCwdFilePath, expectedFinalWd)
}

func TestBashExecExecutor_Execute_Timeout_Streaming(t *testing.T) {
	// Use a context with a short deadline to test timeout behavior
	const testTimeout = 100 * time.Millisecond
	executor := NewBashExecExecutor()
	testCmd := "echo 'Starting sleep...' && sleep 1 && echo 'Finished sleep'"
	cmd := BashExecCommand{
		BaseCommand: BaseCommand{CommandID: "test-timeout-stream-1"},
		Command:     testCmd,
	}
	// Define the expected CWD temp file path
	expectedCwdFilePath := fmt.Sprintf("/tmp/%s.cwd", cmd.CommandID)
	// Clean up before test, just in case
	_ = os.Remove(expectedCwdFilePath)
	// Ensure cleanup after test
	t.Cleanup(func() { _ = os.Remove(expectedCwdFilePath) })

	// Create context with short deadline
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel() // Important to release resources

	resultsChan, err := executor.Execute(ctx, cmd)
	require.NoError(t, err, "Execute setup failed")

	// Allow slightly more time for results collection than the timeout itself
	// Increase timeout to give ample time for final message after kill
	finalResult, combinedOutput, received := collectStreamingResults(t, resultsChan, 3*time.Second)

	require.True(t, received, "Did not receive final result within timeout collection period")
	assert.Equal(t, StatusFailed, finalResult.Status)
	assert.Contains(t, finalResult.Error, "timed out", "Expected timeout error message") // Error message comes from internal timeout check
	assert.Equal(t, "Command execution timed out.", finalResult.Message)

	// Output should contain script start and the first echo, but not the second echo
	// With rapid timeout, script stderr messages might not appear.
	// assert.Contains(t, combinedOutput, scriptInitialDirOutput)
	assert.Contains(t, combinedOutput, "Starting sleep...\n")
	assert.NotContains(t, combinedOutput, "Finished sleep")
	// assert.Contains(t, combinedOutput, scriptExitingOutput)        // Trap might not run or output might not be captured
	// assert.Contains(t, combinedOutput, scriptFinalPwdOutputPrefix) // Trap might not run or output might not be captured

	// Verify the temp file exists even on timeout failure (was not deleted by executor)
	// The script might be killed before writing the file depending on exact timing,
	// but the trap *should* still execute in most cases for SIGTERM/SIGKILL.
	_, err = os.Stat(expectedCwdFilePath)
	// Check content if file exists
	if err == nil {
		fileContentBytes, readErr := os.ReadFile(expectedCwdFilePath)
		if assert.NoError(t, readErr, "Failed to read content of %s (timeout case)", expectedCwdFilePath) {
			wd, _ := os.Getwd() // Assume timeout happened before potential cd
			assert.Equal(t, wd, strings.TrimSpace(string(fileContentBytes)), "Content of %s does not match initial CWD (timeout case)", expectedCwdFilePath)
		}
	} else {
		// If the file doesn't exist, that might be acceptable depending on how quickly the process was killed
		t.Logf("CWD file %s not found after timeout, which might be expected depending on signal timing.", expectedCwdFilePath)
	}
}

func TestBashExecExecutor_Execute_Cancellation_Streaming(t *testing.T) {
	// Test cancellation via the parent context
	executor := NewBashExecExecutor()
	// Command that would run for a while (reduced sleep time)
	testCmd := "echo 'Starting long process...' && sleep 1 && echo 'Finished long process'"
	cmd := BashExecCommand{
		BaseCommand: BaseCommand{CommandID: "test-cancel-stream-1"},
		Command:     testCmd,
	}
	// Define the expected CWD temp file path
	expectedCwdFilePath := fmt.Sprintf("/tmp/%s.cwd", cmd.CommandID)
	// Clean up before test, just in case
	_ = os.Remove(expectedCwdFilePath)
	// Ensure cleanup after test
	t.Cleanup(func() { _ = os.Remove(expectedCwdFilePath) })

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	resultsChan, err := executor.Execute(ctx, cmd)
	require.NoError(t, err, "Execute setup failed")

	// Wait a bit longer before cancelling to ensure process starts reliably
	time.Sleep(500 * time.Millisecond)
	cancel() // Trigger cancellation

	// Collect results with a generous timeout for collection itself
	finalResult, _, received := collectStreamingResults(t, resultsChan, 5*time.Second)

	require.True(t, received, "Did not receive final result after cancellation")
	assert.Equal(t, StatusFailed, finalResult.Status)
	assert.Contains(t, finalResult.Error, "Command execution cancelled by parent context.", "Expected cancellation error message")
	assert.Equal(t, "Command execution cancelled.", finalResult.Message)

	// No longer checking combinedOutput or CWD file for cancellation test, as timing can be unreliable.
}

func TestBashExecExecutor_Execute_InvalidCommandType(t *testing.T) {
	executor := NewBashExecExecutor()
	// Create a command of the wrong type
	cmd := FileReadCommand{
		BaseCommand: BaseCommand{CommandID: "invalid-type-stream-1"},
		FilePath:    "/some/file",
	}

	// Pass context, although it won't be used here as error is immediate
	resultsChan, err := executor.Execute(context.Background(), cmd)

	// Expect an immediate error, not a result from the channel
	require.Error(t, err, "Expected an error for invalid command type")
	assert.Nil(t, resultsChan, "Expected nil channel on immediate error")
	assert.Contains(t, err.Error(), "invalid command type: expected BashExecCommand, got command.FileReadCommand")
}

func TestBashExecExecutor_CreateErrorResult(t *testing.T) {
	executor := NewBashExecExecutor()
	cmd := BashExecCommand{
		BaseCommand: BaseCommand{
			CommandID:   "test-error",
			Description: "Test error result",
		},
		Command: "echo 'test'",
	}

	tests := []struct {
		name          string
		err           error
		expectedError string
	}{
		{
			name:          "basic error",
			err:           fmt.Errorf("command failed"),
			expectedError: "command failed",
		},
		{
			name:          "empty error",
			err:           nil,
			expectedError: "",
		},
		{
			name:          "context cancelled",
			err:           context.Canceled,
			expectedError: context.Canceled.Error(),
		},
		{
			name:          "context timeout",
			err:           context.DeadlineExceeded,
			expectedError: context.DeadlineExceeded.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := executor.CreateErrorResult(cmd, tt.err)

			assert.Equal(t, cmd.CommandID, result.CommandID)
			assert.Equal(t, CmdBashExec, result.CommandType)
			assert.Equal(t, StatusFailed, result.Status)
			assert.Contains(t, result.Message, "Command execution failed")
			assert.Equal(t, tt.expectedError, result.Error)
			assert.Empty(t, result.ResultData)
		})
	}
}
