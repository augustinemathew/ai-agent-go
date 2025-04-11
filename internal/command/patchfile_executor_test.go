package command

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a temporary file for patch tests
func createPatchTestTempFile(t *testing.T, dir, filename, content string) string {
	t.Helper()
	fp := filepath.Join(dir, filename)
	err := os.WriteFile(fp, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create temp file %s: %v", fp, err)
	}
	return fp
}

// Helper function to read file content for patch tests
func readPatchTestFileContent(t *testing.T, filepath string) string {
	t.Helper()
	content, err := os.ReadFile(filepath)
	if err != nil {
		// Allow reading non-existent file to return empty string for assertion consistency
		if errors.Is(err, os.ErrNotExist) {
			return ""
		}
		t.Fatalf("Failed to read file %s: %v", filepath, err)
	}
	return string(content)
}

// Helper to wait for and collect results from the channel for patch tests
func collectPatchTestResults(t *testing.T, resultsChan <-chan OutputResult, timeout time.Duration) []OutputResult {
	t.Helper()
	var results []OutputResult
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case result, ok := <-resultsChan:
			if !ok {
				return results // Channel closed
			}
			results = append(results, result)
			// If it's a terminal state, we might expect the channel to close soon,
			// but we continue collecting briefly in case of unexpected extra messages.
			if result.Status == StatusSucceeded || result.Status == StatusFailed {
				// Reset timer slightly to allow channel close signal
				if !timer.Stop() {
					// Drain the channel if stop failed, common in race conditions
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(100 * time.Millisecond)
			}
		case <-timer.C:
			t.Fatalf("Timeout waiting for results from channel")
			return results // Should not be reached
		}
	}
}

func TestPatchFileExecutor_Execute_Success(t *testing.T) {
	testCases := []struct {
		name            string
		initialContent  string
		patch           string
		expectedContent string
		commandID       string
	}{
		{
			name:            "Simple Add",
			initialContent:  "line1\nline3\n",
			patch:           "--- a/test.txt\n+++ b/test.txt\n@@ -1,2 +1,3 @@\n line1\n+line2\n line3\n",
			expectedContent: "line1\nline2\nline3\n",
			commandID:       "patch-add-1",
		},
		{
			name:            "Simple Delete",
			initialContent:  "line1\nline2\nline3\n",
			patch:           "--- a/test.txt\n+++ b/test.txt\n@@ -1,3 +1,2 @@\n line1\n-line2\n line3\n",
			expectedContent: "line1\nline3\n",
			commandID:       "patch-del-1",
		},
		{
			name:            "Create File",
			initialContent:  "", // File does not exist initially
			patch:           "--- /dev/null\n+++ b/newfile.txt\n@@ -0,0 +1,2 @@\n+Newline 1\n+Newline 2\n",
			expectedContent: "Newline 1\nNewline 2\n",
			commandID:       "patch-create-1",
		},
		{
			name:            "Empty Patch",
			initialContent:  "line1\nline2\n",
			patch:           "",               // Empty patch string
			expectedContent: "line1\nline2\n", // Should preserve original content exactly
			commandID:       "patch-empty-1",
		},
		{
			name:            "Whitespace Patch",
			initialContent:  "line1\nline2\n",
			patch:           " \t\n ",         // Whitespace patch string
			expectedContent: "line1\nline2\n", // Should preserve original content exactly
			commandID:       "patch-ws-1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			filename := "test.txt"
			if tc.name == "Create File" {
				filename = "newfile.txt"
			}
			filePath := filepath.Join(dir, filename)

			if tc.initialContent != "" || tc.name != "Create File" {
				// Create file unless it's the 'Create File' test case
				createPatchTestTempFile(t, dir, filename, tc.initialContent)
			}

			executor := NewPatchFileExecutor()
			cmd := PatchFileCommand{
				BaseCommand: BaseCommand{CommandID: tc.commandID},
				FilePath:    filePath,
				Patch:       tc.patch,
			}

			// Test passing command by value
			resultsChan, err := executor.Execute(context.Background(), cmd)
			if err != nil {
				t.Fatalf("Execute failed unexpectedly: %v", err)
			}

			results := collectPatchTestResults(t, resultsChan, 2*time.Second)

			if len(results) != 1 {
				t.Fatalf("Expected 1 result, got %d: %+v", len(results), results)
			}

			result := results[0]
			if result.Status != StatusSucceeded {
				t.Errorf("Expected status SUCCEEDED, got %s. Msg: %s, Err: %s", result.Status, result.Message, result.Error)
			}
			if result.CommandID != tc.commandID {
				t.Errorf("Expected command ID %s, got %s", tc.commandID, result.CommandID)
			}
			if result.CommandType != CmdPatchFile {
				t.Errorf("Expected command type %s, got %s", CmdPatchFile, result.CommandType)
			}

			// Verify file content
			actualContent := readPatchTestFileContent(t, filePath)
			if diff := cmp.Diff(tc.expectedContent, actualContent); diff != "" {
				t.Errorf("File content mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestPatchFileExecutor_Execute_Failure(t *testing.T) {
	dir := t.TempDir()
	// Create a file for mismatch/read/write tests when file *exists*
	initialContent := "line1\nline2\nline3\n"
	existingFilePath := createPatchTestTempFile(t, dir, "test_fail_exists.txt", initialContent)
	// Define path for a file that *does not* exist
	nonExistentFilePath := filepath.Join(dir, "test_fail_nonexist.txt")

	// Create a read-only file
	readOnlyFilePath := createPatchTestTempFile(t, dir, "readonly.txt", "cant write")
	if err := os.Chmod(readOnlyFilePath, 0444); err != nil {
		t.Fatalf("Failed to make file read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnlyFilePath, 0644) }) // Attempt cleanup

	testCases := []struct {
		name           string
		cmd            any // Use any to test type errors
		expectedStatus ExecutionStatus
		expectedError  string // Substring to check in result.Error or initial error
		initialErr     bool   // Whether Execute itself should return an error
	}{
		{
			name:           "Invalid Command Type",
			cmd:            struct{ Foo string }{Foo: "bar"},
			expectedStatus: "", // No result expected
			expectedError:  "invalid command type",
			initialErr:     true,
		},
		{
			name: "Empty File Path",
			cmd: PatchFileCommand{
				BaseCommand: BaseCommand{CommandID: "fail-path-1"},
				FilePath:    "",
				Patch:       "patch",
			},
			expectedStatus: "", // No result expected
			expectedError:  "file path cannot be empty",
			initialErr:     true,
		},
		{
			name: "Context Mismatch - File Exists",
			cmd: &PatchFileCommand{
				BaseCommand: BaseCommand{CommandID: "fail-ctx-exists-1"},
				FilePath:    existingFilePath,
				Patch:       "--- a/test_fail_exists.txt\n+++ b/test_fail_exists.txt\n@@ -1,3 +1,3 @@\n line1\n-WRONG_LINE\n+correct_line\n line3\n",
			},
			expectedStatus: StatusFailed,
			expectedError:  "context mismatch",
			initialErr:     false,
		},
		{
			name: "Context Mismatch - File Non-Existent", // Apply modify patch to non-existent file
			cmd: &PatchFileCommand{
				BaseCommand: BaseCommand{CommandID: "fail-ctx-nonexist-1"},
				FilePath:    nonExistentFilePath,
				Patch:       "--- a/test_fail_nonexist.txt\n+++ b/test_fail_nonexist.txt\n@@ -1,1 +1,1 @@\n-line1\n+newline1\n", // Requires line1 to exist
			},
			expectedStatus: StatusFailed,
			expectedError:  "context mismatch",
			initialErr:     false,
		},
		{
			name: "Parse Error - File Exists",
			cmd: &PatchFileCommand{
				BaseCommand: BaseCommand{CommandID: "fail-parse-exists-1"},
				FilePath:    existingFilePath,
				Patch:       "this is not a valid patch format",
			},
			expectedStatus: StatusFailed,
			expectedError:  "failed to parse patch",
			initialErr:     false,
		},
		{
			name: "Parse Error - File Non-Existent",
			cmd: &PatchFileCommand{
				BaseCommand: BaseCommand{CommandID: "fail-parse-nonexist-1"},
				FilePath:    nonExistentFilePath,
				Patch:       "this is not a valid patch format",
			},
			expectedStatus: StatusFailed,
			expectedError:  "failed to parse patch",
			initialErr:     false,
		},
		{
			name: "Multi-File Patch Error - File Exists",
			cmd: &PatchFileCommand{
				BaseCommand: BaseCommand{CommandID: "fail-multi-exists-1"},
				FilePath:    existingFilePath,
				Patch:       "--- a/file1.txt\n+++ b/file1.txt\n@@ -1,1 +1,1 @@\n-a\n+b\n--- a/file2.txt\n+++ b/file2.txt\n@@ -1,1 +1,1 @@\n-c\n+d\n",
			},
			expectedStatus: StatusFailed,
			expectedError:  "patch contains multiple file diffs",
			initialErr:     false,
		},
		{
			name: "Multi-File Patch Error - File Non-Existent",
			cmd: &PatchFileCommand{
				BaseCommand: BaseCommand{CommandID: "fail-multi-nonexist-1"},
				FilePath:    nonExistentFilePath,
				Patch:       "--- a/file1.txt\n+++ b/file1.txt\n@@ -1,1 +1,1 @@\n-a\n+b\n--- a/file2.txt\n+++ b/file2.txt\n@@ -1,1 +1,1 @@\n-c\n+d\n",
			},
			expectedStatus: StatusFailed,
			expectedError:  "patch contains multiple file diffs",
			initialErr:     false,
		},
		{
			name: "Write Error (Read Only File)",
			cmd: &PatchFileCommand{
				BaseCommand: BaseCommand{CommandID: "fail-write-1"},
				FilePath:    readOnlyFilePath,
				Patch:       "--- a/readonly.txt\n+++ b/readonly.txt\n@@ -1,1 +1,1 @@\n-cant write\n+can write\n",
			},
			expectedStatus: StatusFailed,
			expectedError:  "permission denied", // Error message check might need OS-specific adjustment
			initialErr:     false,
		},
	}

	executor := NewPatchFileExecutor()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resultsChan, err := executor.Execute(context.Background(), tc.cmd)

			if tc.initialErr {
				if err == nil {
					t.Fatalf("Expected an initial error, but got nil")
				}
				if !strings.Contains(err.Error(), tc.expectedError) {
					t.Errorf("Expected initial error containing '%s', got '%v'", tc.expectedError, err)
				}
				// No results channel expected if initial error occurs
				if resultsChan != nil {
					t.Errorf("Expected nil results channel on initial error, but got one")
				}
				return // End test case for initial errors
			}

			// If no initial error was expected
			if err != nil {
				t.Fatalf("Execute failed unexpectedly: %v", err)
			}
			if resultsChan == nil {
				t.Fatalf("Expected results channel, but got nil")
			}

			results := collectPatchTestResults(t, resultsChan, 2*time.Second)

			if len(results) != 1 {
				t.Fatalf("Expected 1 result, got %d: %+v", len(results), results)
			}

			result := results[0]
			if result.Status != tc.expectedStatus {
				t.Errorf("Expected status %s, got %s. Msg: %s, Err: %s", tc.expectedStatus, result.Status, result.Message, result.Error)
			}
			if result.Error == "" {
				t.Errorf("Expected non-empty Error field for failed status, got empty")
			}
			if !strings.Contains(result.Error, tc.expectedError) {
				t.Errorf("Expected error containing '%s', got '%s'", tc.expectedError, result.Error)
			}

			// Ensure file wasn't unexpectedly modified on failure (except for write test where it fails during write)
			// Check appropriate path based on test type
			checkPath := existingFilePath
			if strings.Contains(tc.name, "Non-Existent") {
				checkPath = nonExistentFilePath
			}

			if tc.name == "Context Mismatch - File Exists" || tc.name == "Parse Error - File Exists" || tc.name == "Multi-File Patch Error - File Exists" {
				actualContent := readPatchTestFileContent(t, checkPath)
				if diff := cmp.Diff(initialContent, actualContent); diff != "" {
					t.Errorf("File content was modified unexpectedly on failure %s (-want +got):\n%s", tc.name, diff)
				}
			} else if strings.Contains(tc.name, "Non-Existent") {
				// Ensure non-existent file wasn't created
				if _, err := os.Stat(checkPath); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("File %s was created unexpectedly on failure %s", checkPath, tc.name)
				}
			}
		})
	}
}

func TestPatchFileExecutor_Execute_ContextCancellation(t *testing.T) {
	dir := t.TempDir()
	// Case 1: File Exists
	initialContentExists := "line1\nline2\n"
	filePathExists := createPatchTestTempFile(t, dir, "cancellation_test_exists.txt", initialContentExists)
	patchExists := "--- a/cancellation_test_exists.txt\n+++ b/cancellation_test_exists.txt\n@@ -1,2 +1,3 @@\n line1\n+inserted\n line2\n"
	cmdExists := &PatchFileCommand{
		BaseCommand: BaseCommand{CommandID: "cancel-exists-1"},
		FilePath:    filePathExists,
		Patch:       patchExists,
	}

	// Case 2: File Does Not Exist
	filePathNonExistent := filepath.Join(dir, "cancellation_test_nonexist.txt")
	patchNonExistent := "--- /dev/null\n+++ b/cancellation_test_nonexist.txt\n@@ -0,0 +1,1 @@\n+created line\n" // Patch to create
	cmdNonExistent := &PatchFileCommand{
		BaseCommand: BaseCommand{CommandID: "cancel-nonexist-1"},
		FilePath:    filePathNonExistent,
		Patch:       patchNonExistent,
	}

	executor := NewPatchFileExecutor()

	t.Run("CancelBeforeStart - File Exists", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		resultsChan, err := executor.Execute(ctx, cmdExists)
		if err != nil {
			t.Fatalf("Execute failed unexpectedly: %v", err)
		}

		results := collectPatchTestResults(t, resultsChan, 1*time.Second)
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d: %+v", len(results), results)
		}
		if results[0].Status != StatusFailed {
			t.Errorf("Expected status FAILED, got %s", results[0].Status)
		}
		if !errors.Is(results[0].UnwrapError(), context.Canceled) && results[0].Error != context.Canceled.Error() {
			// Check if the error is context.Canceled or wraps it
			t.Errorf("Expected error to be context.Canceled, got '%s'", results[0].Error)
		}
		// Ensure existing file not modified
		actualContent := readPatchTestFileContent(t, filePathExists)
		if diff := cmp.Diff(initialContentExists, actualContent); diff != "" {
			t.Errorf("File content mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("CancelBeforeStart - File Non-Existent", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		resultsChan, err := executor.Execute(ctx, cmdNonExistent)
		if err != nil {
			t.Fatalf("Execute failed unexpectedly: %v", err)
		}

		results := collectPatchTestResults(t, resultsChan, 1*time.Second)
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d: %+v", len(results), results)
		}
		if results[0].Status != StatusFailed {
			t.Errorf("Expected status FAILED, got %s", results[0].Status)
		}
		if !errors.Is(results[0].UnwrapError(), context.Canceled) && results[0].Error != context.Canceled.Error() {
			// Check if the error is context.Canceled or wraps it
			t.Errorf("Expected error to be context.Canceled, got '%s'", results[0].Error)
		}
		// Ensure non-existent file wasn't created
		if _, err := os.Stat(filePathNonExistent); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("File %s was created unexpectedly on cancellation", filePathNonExistent)
		}
	})

	// Note: Testing cancellation *during* patch apply or file write is difficult
	// to time reliably in unit tests. These tests cover cancellation checks
	// before read and before write primarily.
}

// Add UnwrapError method to OutputResult for easier error checking with errors.Is/As
// This assumes OutputResult.Error stores the error string.
// A more robust approach would store the actual error object if possible.
func (or OutputResult) UnwrapError() error {
	if or.Error == "" {
		return nil
	}
	// This is a simplification. If Error stores complex wrapped errors as strings,
	// this won't fully unwrap. Consider storing the actual `error` type in OutputResult.
	if or.Error == context.Canceled.Error() {
		return context.Canceled
	}
	if or.Error == context.DeadlineExceeded.Error() {
		return context.DeadlineExceeded
	}
	// Add other known error string mappings if necessary
	return errors.New(or.Error) // Return a basic error wrapping the string
}

func TestApplyPatch_EdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		original      string
		patch         string
		expected      string
		expectError   bool
		errorContains string
	}{
		{
			name:     "empty_file_add_content",
			original: "",
			patch:    "--- /dev/null\n+++ b/test.txt\n@@ -0,0 +1,2 @@\n+Line 1\n+Line 2\n",
			expected: "Line 1\nLine 2\n",
		},
		{
			name:     "empty_file_add_empty_content",
			original: "",
			patch:    "--- /dev/null\n+++ b/test.txt\n@@ -0,0 +1 @@\n+\n",
			expected: "\n",
		},
		{
			name:     "delete_entire_file",
			original: "Line 1\nLine 2\n",
			patch:    "--- a/test.txt\n+++ /dev/null\n@@ -1,2 +0,0 @@\n-Line 1\n-Line 2\n",
			expected: "",
		},
		{
			name:     "delete_middle_lines",
			original: "Line 1\nLine 2\nLine 3\nLine 4\n",
			patch:    "--- a/test.txt\n+++ b/test.txt\n@@ -1,4 +1,2 @@\n Line 1\n-Line 2\n-Line 3\n Line 4\n",
			expected: "Line 1\nLine 4\n",
		},
		{
			name:          "invalid_patch_format",
			original:      "test",
			patch:         "invalid patch format",
			expectError:   true,
			errorContains: "failed to parse patch",
		},
		{
			name:          "mismatched_context",
			original:      "Line 1\nLine 2\n",
			patch:         "--- a/test.txt\n+++ b/test.txt\n@@ -1,2 +1,2 @@\n Line 1\n-Wrong Line\n+New Line\n",
			expectError:   true,
			errorContains: "context mismatch",
		},
		{
			name:     "multiple_hunks",
			original: "Line 1\nLine 2\nLine 3\nLine 4\n",
			patch:    "--- a/test.txt\n+++ b/test.txt\n@@ -1,2 +1,2 @@\n Line 1\n-Line 2\n+New Line 2\n@@ -3,2 +3,2 @@\n Line 3\n-Line 4\n+New Line 4\n",
			expected: "Line 1\nNew Line 2\nLine 3\nNew Line 4\n",
		},
		{
			name:     "add_at_end",
			original: "Line 1\nLine 2\n",
			patch:    "--- a/test.txt\n+++ b/test.txt\n@@ -1,2 +1,3 @@\n Line 1\n Line 2\n+Line 3\n",
			expected: "Line 1\nLine 2\nLine 3\n",
		},
		{
			name:     "add_at_beginning",
			original: "Line 2\nLine 3\n",
			patch:    "--- a/test.txt\n+++ b/test.txt\n@@ -0,0 +1 @@\n+Line 1\n@@ -1,2 +2,2 @@\n Line 2\n Line 3\n",
			expected: "Line 1\nLine 2\nLine 3\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := applyPatch([]byte(tt.original), []byte(tt.patch))
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, string(result))
		})
	}
}
