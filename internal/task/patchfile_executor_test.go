package task

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/sourcegraph/go-diff/diff"
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
			cmd := NewPatchFileTask(tc.commandID, tc.name, PatchFileParameters{
				FilePath: filePath,
				Patch:    tc.patch,
			})

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
			if result.TaskID != tc.commandID {
				t.Errorf("Expected command ID %s, got %s", tc.commandID, result.TaskID)
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
		expectedStatus TaskStatus
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
			cmd: NewPatchFileTask("fail-path-1", "Empty file path", PatchFileParameters{
				FilePath: "",
				Patch:    "patch",
			}),
			expectedStatus: "", // No result expected
			expectedError:  "file path cannot be empty",
			initialErr:     true,
		},
		{
			name: "Context Mismatch - File Exists",
			cmd: NewPatchFileTask("fail-ctx-exists-1", "Context mismatch - File exists", PatchFileParameters{
				FilePath: existingFilePath,
				Patch:    "--- a/test_fail_exists.txt\n+++ b/test_fail_exists.txt\n@@ -1,3 +1,3 @@\n line1\n-WRONG_LINE\n+correct_line\n line3\n",
			}),
			expectedStatus: StatusFailed,
			expectedError:  "context mismatch",
			initialErr:     false,
		},
		{
			name: "Context Mismatch - File Non-Existent", // Apply modify patch to non-existent file
			cmd: NewPatchFileTask("fail-ctx-nonexist-1", "Context mismatch - File non-existent", PatchFileParameters{
				FilePath: nonExistentFilePath,
				Patch:    "--- a/test_fail_nonexist.txt\n+++ b/test_fail_nonexist.txt\n@@ -1,1 +1,1 @@\n-line1\n+newline1\n", // Requires line1 to exist
			}),
			expectedStatus: StatusFailed,
			expectedError:  "context mismatch",
			initialErr:     false,
		},
		{
			name: "Parse Error - File Exists",
			cmd: NewPatchFileTask("fail-parse-exists-1", "Parse error - File exists", PatchFileParameters{
				FilePath: existingFilePath,
				Patch:    "this is not a valid patch format",
			}),
			expectedStatus: StatusFailed,
			expectedError:  "failed to parse patch",
			initialErr:     false,
		},
		{
			name: "Parse Error - File Non-Existent",
			cmd: NewPatchFileTask("fail-parse-nonexist-1", "Parse error - File non-existent", PatchFileParameters{
				FilePath: nonExistentFilePath,
				Patch:    "this is not a valid patch format",
			}),
			expectedStatus: StatusFailed,
			expectedError:  "failed to parse patch",
			initialErr:     false,
		},
		{
			name: "Multi-File Patch Error - File Exists",
			cmd: NewPatchFileTask("fail-multi-exists-1", "Multi-file patch error - File exists", PatchFileParameters{
				FilePath: existingFilePath,
				Patch:    "--- a/file1.txt\n+++ b/file1.txt\n@@ -1,1 +1,1 @@\n-a\n+b\n--- a/file2.txt\n+++ b/file2.txt\n@@ -1,1 +1,1 @@\n-c\n+d\n",
			}),
			expectedStatus: StatusFailed,
			expectedError:  "patch contains multiple file diffs",
			initialErr:     false,
		},
		{
			name: "Multi-File Patch Error - File Non-Existent",
			cmd: NewPatchFileTask("fail-multi-nonexist-1", "Multi-file patch error - File non-existent", PatchFileParameters{
				FilePath: nonExistentFilePath,
				Patch:    "--- a/file1.txt\n+++ b/file1.txt\n@@ -1,1 +1,1 @@\n-a\n+b\n--- a/file2.txt\n+++ b/file2.txt\n@@ -1,1 +1,1 @@\n-c\n+d\n",
			}),
			expectedStatus: StatusFailed,
			expectedError:  "patch contains multiple file diffs",
			initialErr:     false,
		},
		{
			name: "Write Error (Read Only File)",
			cmd: NewPatchFileTask("fail-write-1", "Write error (Read only file)", PatchFileParameters{
				FilePath: readOnlyFilePath,
				Patch:    "--- a/readonly.txt\n+++ b/readonly.txt\n@@ -1,1 +1,1 @@\n-cant write\n+can write\n",
			}),
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
	cmdExists := NewPatchFileTask("cancel-exists-1", "Context cancellation - File exists", PatchFileParameters{
		FilePath: filePathExists,
		Patch:    patchExists,
	})

	// Case 2: File Does Not Exist
	filePathNonExistent := filepath.Join(dir, "cancellation_test_nonexist.txt")
	patchNonExistent := "--- /dev/null\n+++ b/cancellation_test_nonexist.txt\n@@ -0,0 +1,1 @@\n+created line\n" // Patch to create
	cmdNonExistent := NewPatchFileTask("cancel-nonexist-1", "Context cancellation - File non-existent", PatchFileParameters{
		FilePath: filePathNonExistent,
		Patch:    patchNonExistent,
	})

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
		// New test cases to improve coverage
		{
			name:     "patch_with_many_remaining_lines",
			original: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6\nLine 7\nLine 8\nLine 9\nLine 10\n",
			patch:    "--- a/test.txt\n+++ b/test.txt\n@@ -1,3 +1,4 @@\n Line 1\n Line 2\n+New Line\n Line 3\n",
			expected: "Line 1\nLine 2\nNew Line\nLine 3\nLine 4\nLine 5\nLine 6\nLine 7\nLine 8\nLine 9\nLine 10\n",
		},
		{
			name:     "patch_with_no_trailing_newline",
			original: "Line 1\nLine 2\nLine 3", // Note: no trailing newline
			patch:    "--- a/test.txt\n+++ b/test.txt\n@@ -1,3 +1,4 @@\n Line 1\n Line 2\n+New Line\n Line 3",
			expected: "Line 1\nLine 2\nNew Line\nLine 3\n", // Our implementation adds a trailing newline
		},
		{
			name:     "patch_adds_trailing_newline",
			original: "Line 1\nLine 2\nLine 3", // Note: no trailing newline
			patch:    "--- a/test.txt\n+++ b/test.txt\n@@ -1,3 +1,4 @@\n Line 1\n Line 2\n Line 3\n+Line 4\n",
			expected: "Line 1\nLine 2\nLine 3\nLine 4\n",
		},
		{
			name:     "patch_preserves_empty_file",
			original: "",
			patch:    "--- /dev/null\n+++ b/test.txt\n",
			expected: "",
		},
		{
			name:          "context_line_eof_error",
			original:      "Line 1\n",
			patch:         "--- a/test.txt\n+++ b/test.txt\n@@ -1,2 +1,2 @@\n Line 1\n Line 2\n",
			expectError:   true,
			errorContains: "context mismatch: expected 'Line 2', got '' at original line 2",
		},
		{
			name:          "deletion_line_eof_error",
			original:      "Line 1\n",
			patch:         "--- a/test.txt\n+++ b/test.txt\n@@ -1,2 +1,1 @@\n Line 1\n-Line 2\n",
			expectError:   true,
			errorContains: "context mismatch: expected removal of 'Line 2', got '' at original line 2",
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

// Test specifically for addRemainingLines function to improve coverage
func TestAddRemainingLines(t *testing.T) {
	testCases := []struct {
		name          string
		originalLines [][]byte
		currentLine   int
		expected      [][]byte
	}{
		{
			name:          "add_multiple_remaining_lines",
			originalLines: [][]byte{[]byte("Line 1"), []byte("Line 2"), []byte("Line 3"), []byte("Line 4")},
			currentLine:   1,
			expected:      [][]byte{[]byte("Line 2"), []byte("Line 3"), []byte("Line 4")},
		},
		{
			name:          "add_single_remaining_line",
			originalLines: [][]byte{[]byte("Line 1"), []byte("Line 2")},
			currentLine:   1,
			expected:      [][]byte{[]byte("Line 2")},
		},
		{
			name:          "add_no_remaining_lines",
			originalLines: [][]byte{[]byte("Line 1"), []byte("Line 2")},
			currentLine:   2,
			expected:      nil, // The function results in an empty slice, which is nil
		},
		{
			name:          "add_empty_last_line",
			originalLines: [][]byte{[]byte("Line 1"), []byte("Line 2"), []byte("")},
			currentLine:   2,
			expected:      nil, // The function results in an empty slice, which is nil
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var result [][]byte
			addRemainingLines(&result, tc.originalLines, tc.currentLine)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// Test specifically for verifyContextLine and verifyDeletionLine functions
func TestLineVerification(t *testing.T) {
	testCases := []struct {
		name           string
		functionToTest string // "context" or "deletion"
		line           []byte
		originalLines  [][]byte
		currentLine    int
		expectError    bool
		errorContains  string
	}{
		{
			name:           "matching_context_line",
			functionToTest: "context",
			line:           []byte(" Line 2"),
			originalLines:  [][]byte{[]byte("Line 1"), []byte("Line 2"), []byte("Line 3")},
			currentLine:    1,
			expectError:    false,
		},
		{
			name:           "mismatched_context_line",
			functionToTest: "context",
			line:           []byte(" Wrong Line"),
			originalLines:  [][]byte{[]byte("Line 1"), []byte("Line 2"), []byte("Line 3")},
			currentLine:    1,
			expectError:    true,
			errorContains:  "context mismatch: expected 'Wrong Line', got 'Line 2'",
		},
		{
			name:           "context_line_at_eof",
			functionToTest: "context",
			line:           []byte(" Line 4"),
			originalLines:  [][]byte{[]byte("Line 1"), []byte("Line 2"), []byte("Line 3")},
			currentLine:    3,
			expectError:    true,
			errorContains:  "context mismatch: expected 'Line 4', got end of file",
		},
		{
			name:           "matching_deletion_line",
			functionToTest: "deletion",
			line:           []byte("-Line 2"),
			originalLines:  [][]byte{[]byte("Line 1"), []byte("Line 2"), []byte("Line 3")},
			currentLine:    1,
			expectError:    false,
		},
		{
			name:           "mismatched_deletion_line",
			functionToTest: "deletion",
			line:           []byte("-Wrong Line"),
			originalLines:  [][]byte{[]byte("Line 1"), []byte("Line 2"), []byte("Line 3")},
			currentLine:    1,
			expectError:    true,
			errorContains:  "context mismatch: expected removal of 'Wrong Line', got 'Line 2'",
		},
		{
			name:           "deletion_line_at_eof",
			functionToTest: "deletion",
			line:           []byte("-Line 4"),
			originalLines:  [][]byte{[]byte("Line 1"), []byte("Line 2"), []byte("Line 3")},
			currentLine:    3,
			expectError:    true,
			errorContains:  "context mismatch: expected removal of 'Line 4', got end of file",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			if tc.functionToTest == "context" {
				err = verifyContextLine(tc.line, tc.originalLines, tc.currentLine)
			} else {
				err = verifyDeletionLine(tc.line, tc.originalLines, tc.currentLine)
			}

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func BenchmarkPatchProcessing(b *testing.B) {
	// Generate test data of different sizes
	generateContent := func(lines, lineLength int) []byte {
		content := make([]byte, 0, lines*(lineLength+1))
		line := bytes.Repeat([]byte("a"), lineLength)
		for i := 0; i < lines; i++ {
			content = append(content, line...)
			content = append(content, '\n')
		}
		return content
	}

	generatePatch := func(lines, lineLength int, modifyEvery int) []byte {
		var patch bytes.Buffer
		patch.WriteString("--- a/test.txt\n+++ b/test.txt\n")

		currentLine := 1
		for currentLine < lines {
			// Write hunk header
			patch.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n",
				currentLine, modifyEvery+1, currentLine, modifyEvery+2))

			// Write context and changes
			for i := 0; i < modifyEvery; i++ {
				line := bytes.Repeat([]byte("a"), lineLength)
				patch.WriteByte(' ')
				patch.Write(line)
				patch.WriteByte('\n')
				currentLine++
			}

			// Add a new line
			newLine := bytes.Repeat([]byte("b"), lineLength)
			patch.WriteByte('+')
			patch.Write(newLine)
			patch.WriteByte('\n')

			if currentLine >= lines {
				break
			}
		}
		return patch.Bytes()
	}

	benchCases := []struct {
		name        string
		lines       int
		lineLength  int
		modifyEvery int
	}{
		{"Small_File", 100, 50, 10},
		{"Medium_File", 1000, 100, 50},
		{"Large_File", 10000, 200, 100},
		{"Huge_File", 100000, 500, 1000},
	}

	for _, bc := range benchCases {
		original := generateContent(bc.lines, bc.lineLength)
		patch := generatePatch(bc.lines, bc.lineLength, bc.modifyEvery)

		b.Run(fmt.Sprintf("PrepareLines_%s", bc.name), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(original)))
			for i := 0; i < b.N; i++ {
				lines := prepareOriginalLines(original)
				runtime.KeepAlive(lines)
			}
		})

		b.Run(fmt.Sprintf("FormatOutput_%s", bc.name), func(b *testing.B) {
			lines := prepareOriginalLines(original)
			fileDiff, _ := diff.ParseFileDiff(patch)

			b.ReportAllocs()
			b.SetBytes(int64(len(original)))
			for i := 0; i < b.N; i++ {
				output, _ := formatFinalOutput(lines, fileDiff, true)
				runtime.KeepAlive(output)
			}
		})

		b.Run(fmt.Sprintf("FullPatch_%s", bc.name), func(b *testing.B) {
			executor := NewPatchFileExecutor()
			b.ReportAllocs()
			b.SetBytes(int64(len(original)))
			for i := 0; i < b.N; i++ {
				output, _ := executor.patcher.ApplyPatch(original, patch)
				runtime.KeepAlive(output)
			}
		})
	}
}

func TestPatchFileExecutor_Execute_TerminalTaskHandling(t *testing.T) {
	executor := NewPatchFileExecutor()

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
			cmd := NewPatchFileTask("terminal-patchfile-test", "Terminal patchfile task test", PatchFileParameters{
				FilePath: "nonexistent/file.txt", // Should not try to patch this
				Patch:    "--- a/file.txt\n+++ b/file.txt\n@@ -1,1 +1,1 @@\n-old\n+new",
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

func TestConcurrentPatchOps(t *testing.T) {
	t.Run("Concurrent_Same_File", func(t *testing.T) {
		// Create temporary directory
		tempDir := t.TempDir()

		// Create test file with initial content
		testFilePath := filepath.Join(tempDir, "test_file.txt")
		initialContent := "content"
		err := os.WriteFile(testFilePath, []byte(initialContent), 0644)
		require.NoError(t, err, "Failed to write test file")

		// Create patch executor
		patchExecutor := NewPatchFileExecutor()

		// Number of concurrent patches to apply
		numPatches := 5
		var wg sync.WaitGroup
		results := make([]OutputResult, 0, numPatches)
		var resultsMutex sync.Mutex

		// Create patches that append additional content so they don't conflict completely
		for i := 0; i < numPatches; i++ {
			i := i // Capture loop variable
			wg.Add(1)

			go func() {
				defer wg.Done()

				// Read current file content before creating patch
				currentContent, err := os.ReadFile(testFilePath)
				if err != nil {
					t.Logf("Patch %d: Error reading file: %v", i, err)
					currentContent = []byte(initialContent) // Fallback to initial content
				}

				// Create a unified diff patch to add a new line with our index
				patch := fmt.Sprintf("--- %s\n+++ %s\n@@ -1 +1,2 @@\n %s\n+new line %d\n",
					testFilePath, testFilePath, string(currentContent), i)

				cmd := NewPatchFileTask(fmt.Sprintf("patch-cmd-%d", i), "Patch file task test", PatchFileParameters{
					FilePath: testFilePath,
					Patch:    patch,
				})

				ctx := context.Background()
				resultChan, err := patchExecutor.Execute(ctx, cmd)
				require.NoError(t, err, "Failed to execute patch %d", i)

				// Collect results
				for result := range resultChan {
					t.Logf("Patch %d result: status=%s, error=%v", i, result.Status, result.Error)
					resultsMutex.Lock()
					results = append(results, result)
					resultsMutex.Unlock()
				}
			}()
		}

		// Wait for all patches to complete
		wg.Wait()

		// Read final file content
		finalContent, err := os.ReadFile(testFilePath)
		require.NoError(t, err, "Failed to read final content")

		// Verify that at least one patch succeeded
		successCount := 0
		for _, result := range results {
			if result.Status == StatusSucceeded {
				successCount++
			}
		}

		require.GreaterOrEqual(t, successCount, 1, "At least one patch should succeed")

		// Final content should contain "new line" at least once
		require.NotEmpty(t, finalContent, "File content should not be empty")
		require.Contains(t, string(finalContent), "new line", "File should contain the patched content")

		t.Logf("Final file content: %s", string(finalContent))
		t.Logf("Success count: %d out of %d attempts", successCount, numPatches)
	})
}
