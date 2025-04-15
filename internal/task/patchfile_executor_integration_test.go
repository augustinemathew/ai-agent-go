package task

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFullPatchWorkflow tests the complete patch application workflow
// including file creation, modification, and cleanup
func TestFullPatchWorkflow(t *testing.T) {
	dir := t.TempDir()
	executor := NewPatchFileExecutor()

	// Test creating a new file
	t.Run("Create New File", func(t *testing.T) {
		filePath := filepath.Join(dir, "new_file.txt")
		cmd := &PatchFileTask{
			BaseTask: BaseTask{TaskId: "create-1"},
			Parameters: PatchFileParameters{
				FilePath: filePath,
				Patch:    "--- /dev/null\n+++ b/new_file.txt\n@@ -0,0 +1,2 @@\n+Line 1\n+Line 2\n",
			},
		}

		resultsChan, err := executor.Execute(context.Background(), cmd)
		require.NoError(t, err)

		results := collectPatchTestResults(t, resultsChan, 2*time.Second)
		require.Len(t, results, 1)
		assert.Equal(t, StatusSucceeded, results[0].Status)

		content := readPatchTestFileContent(t, filePath)
		assert.Equal(t, "Line 1\nLine 2\n", content)
	})

	// Test modifying an existing file
	t.Run("Modify Existing File", func(t *testing.T) {
		filePath := filepath.Join(dir, "existing_file.txt")
		initialContent := "Line 1\nLine 2\nLine 3\n"
		createPatchTestTempFile(t, dir, "existing_file.txt", initialContent)

		cmd := &PatchFileTask{
			BaseTask: BaseTask{TaskId: "modify-1"},
			Parameters: PatchFileParameters{
				FilePath: filePath,
				Patch:    "--- a/existing_file.txt\n+++ b/existing_file.txt\n@@ -1,3 +1,4 @@\n Line 1\n Line 2\n+New Line\n Line 3\n",
			},
		}

		resultsChan, err := executor.Execute(context.Background(), cmd)
		require.NoError(t, err)

		results := collectPatchTestResults(t, resultsChan, 2*time.Second)
		require.Len(t, results, 1)
		assert.Equal(t, StatusSucceeded, results[0].Status)

		content := readPatchTestFileContent(t, filePath)
		assert.Equal(t, "Line 1\nLine 2\nNew Line\nLine 3\n", content)
	})
}

// TestErrorHandlingScenarios tests various error handling scenarios
func TestErrorHandlingScenarios(t *testing.T) {
	dir := t.TempDir()
	executor := NewPatchFileExecutor()

	// Test invalid patch format
	t.Run("Invalid Patch Format", func(t *testing.T) {
		filePath := filepath.Join(dir, "invalid_patch.txt")
		createPatchTestTempFile(t, dir, "invalid_patch.txt", "content")

		cmd := &PatchFileTask{
			BaseTask: BaseTask{TaskId: "invalid-1"},
			Parameters: PatchFileParameters{
				FilePath: filePath,
				Patch:    "invalid patch format",
			},
		}

		resultsChan, err := executor.Execute(context.Background(), cmd)
		require.NoError(t, err)

		results := collectPatchTestResults(t, resultsChan, 2*time.Second)
		require.Len(t, results, 1)
		assert.Equal(t, StatusFailed, results[0].Status)
		assert.Contains(t, results[0].Error, "failed to parse patch")
	})

	// Test file permission errors
	t.Run("Permission Error", func(t *testing.T) {
		filePath := filepath.Join(dir, "readonly.txt")
		createPatchTestTempFile(t, dir, "readonly.txt", "content")
		require.NoError(t, os.Chmod(filePath, 0444))
		defer os.Chmod(filePath, 0644)

		cmd := &PatchFileTask{
			BaseTask: BaseTask{TaskId: "perm-1"},
			Parameters: PatchFileParameters{
				FilePath: filePath,
				Patch:    "--- a/readonly.txt\n+++ b/readonly.txt\n@@ -1,1 +1,1 @@\n-content\n+new content\n",
			},
		}

		resultsChan, err := executor.Execute(context.Background(), cmd)
		require.NoError(t, err)

		results := collectPatchTestResults(t, resultsChan, 2*time.Second)
		require.Len(t, results, 1)
		assert.Equal(t, StatusFailed, results[0].Status)
		assert.Contains(t, results[0].Error, "permission denied")
	})
}

// TestConcurrentPatchOperations tests concurrent patch operations
func TestConcurrentPatchOperations(t *testing.T) {
	dir := t.TempDir()
	executor := NewPatchFileExecutor()

	// Test concurrent patches to different files
	t.Run("Concurrent Different Files", func(t *testing.T) {
		const numFiles = 10
		var wg sync.WaitGroup
		wg.Add(numFiles)

		for i := 0; i < numFiles; i++ {
			go func(i int) {
				defer wg.Done()
				filePath := filepath.Join(dir, fmt.Sprintf("concurrent_%d.txt", i))
				createPatchTestTempFile(t, dir, fmt.Sprintf("concurrent_%d.txt", i), "content")

				cmd := &PatchFileTask{
					BaseTask: BaseTask{TaskId: fmt.Sprintf("concurrent-%d", i)},
					Parameters: PatchFileParameters{
						FilePath: filePath,
						Patch:    fmt.Sprintf("--- a/concurrent_%d.txt\n+++ b/concurrent_%d.txt\n@@ -1,1 +1,1 @@\n-content\n+new content %d\n", i, i, i),
					},
				}

				resultsChan, err := executor.Execute(context.Background(), cmd)
				require.NoError(t, err)

				results := collectPatchTestResults(t, resultsChan, 2*time.Second)
				require.Len(t, results, 1)
				assert.Equal(t, StatusSucceeded, results[0].Status)

				content := readPatchTestFileContent(t, filePath)
				assert.Equal(t, fmt.Sprintf("new content %d\n", i), content)
			}(i)
		}

		wg.Wait()
	})

	// Test concurrent patches to the same file
	t.Run("Concurrent Same File", func(t *testing.T) {
		filePath := filepath.Join(dir, "concurrent_same.txt")
		createPatchTestTempFile(t, dir, "concurrent_same.txt", "content")

		const numPatches = 5
		var wg sync.WaitGroup
		wg.Add(numPatches)

		for i := 0; i < numPatches; i++ {
			go func(i int) {
				defer wg.Done()
				cmd := &PatchFileTask{
					BaseTask: BaseTask{TaskId: fmt.Sprintf("concurrent-same-%d", i)},
					Parameters: PatchFileParameters{
						FilePath: filePath,
						Patch:    fmt.Sprintf("--- a/concurrent_same.txt\n+++ b/concurrent_same.txt\n@@ -1,1 +1,1 @@\n-content\n+new content %d\n", i),
					},
				}

				resultsChan, err := executor.Execute(context.Background(), cmd)
				require.NoError(t, err)

				results := collectPatchTestResults(t, resultsChan, 2*time.Second)
				require.Len(t, results, 1)
				// Some patches may fail due to concurrent modifications
				if results[0].Status == StatusSucceeded {
					content := readPatchTestFileContent(t, filePath)
					assert.Contains(t, content, "new content")
				}
			}(i)
		}

		wg.Wait()
	})
}
