package task_test

import (
	"ai-agent-v3/internal/task"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupExecutor_Execute_Success(t *testing.T) {
	// Create a registry
	registry := task.NewMapRegistry()

	// Create a group task with two file write children
	tempDir := t.TempDir()

	// First child task
	paramFileWrite1 := task.FileWriteParameters{
		FilePath: tempDir + "/child1.txt",
		Content:  "Content from child 1",
	}
	t.Logf("Parameters type: %T", paramFileWrite1)

	child1 := &task.Task{
		BaseTask: task.BaseTask{
			TaskId:      "child-1",
			Description: "First child task",
			Type:        task.TaskFileWrite,
		},
		Parameters: paramFileWrite1,
	}
	t.Logf("Child1 parameters type: %T", child1.Parameters)

	// Second child task
	child2 := &task.Task{
		BaseTask: task.BaseTask{
			TaskId:      "child-2",
			Description: "Second child task",
			Type:        task.TaskFileWrite,
		},
		Parameters: task.FileWriteParameters{
			FilePath: tempDir + "/child2.txt",
			Content:  "Content from child 2",
		},
	}

	// Create the group task
	groupTask := task.NewGroupTask("group-1", "Group with two children", []*task.Task{child1, child2})

	// Execute the group task
	executor, err := registry.GetExecutor(task.TaskGroup)
	if err != nil {
		t.Fatalf("Failed to get GroupExecutor: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resultsChan, err := executor.Execute(ctx, groupTask)
	if err != nil {
		t.Fatalf("Failed to execute group task: %v", err)
	}

	// Collect results
	var lastResult task.OutputResult
	var runningResults int

	for result := range resultsChan {
		if result.Status == task.StatusRunning {
			runningResults++
		}
		lastResult = result
	}

	// Verify we got running status updates
	if runningResults == 0 {
		t.Error("Expected at least one RUNNING status update")
	}

	// Verify final result
	if lastResult.Status != task.StatusSucceeded {
		t.Errorf("Expected final status SUCCEEDED, got %s with error: %s", lastResult.Status, lastResult.Error)
	}

	// Verify files were created
	verifyFileContent(t, tempDir+"/child1.txt", "Content from child 1")
	verifyFileContent(t, tempDir+"/child2.txt", "Content from child 2")
}

func TestGroupExecutor_Execute_PartialFailure(t *testing.T) {
	// Create a registry
	registry := task.NewMapRegistry()

	// Create a group task with one successful and one failing child
	tempDir := t.TempDir()

	// First child task (will succeed)
	child1 := task.Task{
		BaseTask: task.BaseTask{
			TaskId:      "child-good",
			Description: "Successful child task",
			Type:        task.TaskFileWrite,
		},
		Parameters: task.FileWriteParameters{
			FilePath: tempDir + "/good.txt",
			Content:  "This file will be created successfully",
		},
	}

	// Second child task (will fail - non-existent directory)
	child2 := task.Task{
		BaseTask: task.BaseTask{
			TaskId:      "child-bad",
			Description: "Failing child task",
			Type:        task.TaskFileWrite,
		},
		Parameters: task.FileWriteParameters{
			FilePath: tempDir + "/non-existent-dir/bad.txt", // Directory doesn't exist
			Content:  "This should fail",
		},
	}

	// Create the group task
	groupTask := task.NewGroupTask("group-partial-fail", "Group with mixed success/failure", []*task.Task{&child1, &child2})

	// Execute the group task
	executor, err := registry.GetExecutor(task.TaskGroup)
	if err != nil {
		t.Fatalf("Failed to get GroupExecutor: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resultsChan, err := executor.Execute(ctx, groupTask)
	if err != nil {
		t.Fatalf("Failed to execute group task: %v", err)
	}

	// Collect results
	var lastResult task.OutputResult
	var runningResults int

	for result := range resultsChan {
		if result.Status == task.StatusRunning {
			runningResults++
		}
		lastResult = result
	}

	// The group task should fail because one child failed
	if lastResult.Status != task.StatusFailed {
		t.Errorf("Expected final status FAILED, got %s", lastResult.Status)
	}

	// The error should mention the failing task
	if lastResult.Error == "" || len(lastResult.Error) < 10 {
		t.Errorf("Expected detailed error message, got: %s", lastResult.Error)
	}

	// The successful child should have created its file
	verifyFileContent(t, tempDir+"/good.txt", "This file will be created successfully")
}

func TestGroupExecutor_Execute_NestedGroups(t *testing.T) {
	// Create a registry
	registry := task.NewMapRegistry()

	// Create a nested group structure
	tempDir := t.TempDir()

	// Innermost child task
	innerChild := task.Task{
		BaseTask: task.BaseTask{
			TaskId:      "inner-child",
			Description: "Inner child task",
			Type:        task.TaskFileWrite,
		},
		Parameters: task.FileWriteParameters{
			FilePath: tempDir + "/inner.txt",
			Content:  "Content from inner child",
		},
	}

	// Middle group task containing the inner child
	middleGroup := task.Task{
		BaseTask: task.BaseTask{
			TaskId:      "middle-group",
			Description: "Middle group task",
			Type:        task.TaskGroup,
			Children:    []*task.Task{&innerChild},
		},
	}

	// Sibling task at the middle level
	middleSibling := task.Task{
		BaseTask: task.BaseTask{
			TaskId:      "middle-sibling",
			Description: "Middle sibling task",
			Type:        task.TaskFileWrite,
		},
		Parameters: task.FileWriteParameters{
			FilePath: tempDir + "/middle.txt",
			Content:  "Content from middle sibling",
		},
	}

	// Outer group containing middle group and middle sibling
	outerGroup := task.NewGroupTask("outer-group", "Outer group task", []*task.Task{&middleGroup, &middleSibling})

	// Execute the outer group task
	executor, err := registry.GetExecutor(task.TaskGroup)
	if err != nil {
		t.Fatalf("Failed to get GroupExecutor: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resultsChan, err := executor.Execute(ctx, outerGroup)
	if err != nil {
		t.Fatalf("Failed to execute group task: %v", err)
	}

	// Collect results
	var lastResult task.OutputResult
	for result := range resultsChan {
		lastResult = result
	}

	// Verify execution succeeded
	if lastResult.Status != task.StatusSucceeded {
		t.Errorf("Expected final status SUCCEEDED, got %s with error: %s", lastResult.Status, lastResult.Error)
	}

	// Verify all files were created
	verifyFileContent(t, tempDir+"/inner.txt", "Content from inner child")
	verifyFileContent(t, tempDir+"/middle.txt", "Content from middle sibling")
}

func TestGroupExecutor_Execute_TerminalTaskHandling(t *testing.T) {
	executor := task.NewGroupExecutor(task.NewMapRegistry())

	testCases := []struct {
		name           string
		status         task.TaskStatus
		expectedStatus task.TaskStatus
	}{
		{
			name:           "Already succeeded task",
			status:         task.StatusSucceeded,
			expectedStatus: task.StatusSucceeded,
		},
		{
			name:           "Already failed task",
			status:         task.StatusFailed,
			expectedStatus: task.StatusFailed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a child task
			childTask := task.Task{
				BaseTask: task.BaseTask{
					TaskId:      "child-1",
					Description: "Child task that should not execute",
					Type:        task.TaskBashExec,
				},
				Parameters: task.BashExecParameters{
					Command: "echo 'This should not execute'",
				},
			}

			// Create a task that's already in a terminal state
			groupTask := task.NewGroupTask("terminal-group-test", "Terminal group task test", []*task.Task{&childTask})
			groupTask.Status = tc.status
			groupTask.Output = task.OutputResult{
				TaskID:  "terminal-group-test",
				Status:  tc.status,
				Message: "Pre-existing terminal state",
			}

			resultsChan, err := executor.Execute(context.Background(), groupTask)
			require.NoError(t, err, "Execute should not return an error for terminal tasks")
			require.NotNil(t, resultsChan, "Result channel should not be nil")

			// Get the result from the channel
			var finalResult task.OutputResult
			select {
			case result, ok := <-resultsChan:
				require.True(t, ok, "Channel closed without receiving a result")
				finalResult = result
			case <-time.After(1 * time.Second):
				t.Fatal("Timed out waiting for result from terminal task")
			}

			// Check the result
			assert.Equal(t, groupTask.TaskId, finalResult.TaskID, "TaskID should match")
			assert.Equal(t, tc.expectedStatus, finalResult.Status, "Status should remain unchanged")
			assert.Equal(t, "Pre-existing terminal state", finalResult.Message, "Message should be preserved")

			// Ensure the channel is closed
			_, ok := <-resultsChan
			assert.False(t, ok, "Channel should be closed after sending the result")
		})
	}
}

// Helper function to verify file content
func verifyFileContent(t *testing.T, filePath, expectedContent string) {
	t.Helper()

	// Create a file read task to check the content
	readTask := task.NewFileReadTask("read-verify", "Read verify", task.FileReadParameters{
		FilePath: filePath,
	})

	// Get file read executor
	registry := task.NewMapRegistry()
	executor, err := registry.GetExecutor(task.TaskFileRead)
	if err != nil {
		t.Fatalf("Failed to get FileReadExecutor: %v", err)
	}

	// Execute the read task
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resultsChan, err := executor.Execute(ctx, readTask)
	if err != nil {
		t.Fatalf("Failed to execute read task for %s: %v", filePath, err)
	}

	// Collect the read content
	var content string
	var status task.TaskStatus

	for result := range resultsChan {
		status = result.Status
		content += result.ResultData
	}

	// Check that read succeeded
	if status != task.StatusSucceeded {
		t.Fatalf("Failed to read file %s", filePath)
	}

	// Trim trailing newlines for comparison
	content = strings.TrimSuffix(content, "\n")

	// Check content
	if content != expectedContent {
		t.Errorf("File %s has incorrect content. Expected: %q, Got: %q", filePath, expectedContent, content)
	}
}

// TestGroupExecutor_ChildTaskStatusUpdates verifies that child task statuses are correctly updated
func TestGroupExecutor_ChildTaskStatusUpdates(t *testing.T) {
	// Create a registry
	registry := task.NewMapRegistry()

	// Create a group task with two file write children
	tempDir := t.TempDir()

	// First child task
	child1 := &task.Task{
		BaseTask: task.BaseTask{
			TaskId:      "child-1",
			Description: "First child task",
			Type:        task.TaskFileWrite,
		},
		Parameters: task.FileWriteParameters{
			FilePath: filepath.Join(tempDir, "child1.txt"),
			Content:  "Content from child 1",
		},
	}

	// Second child task
	child2 := &task.Task{
		BaseTask: task.BaseTask{
			TaskId:      "child-2",
			Description: "Second child task",
			Type:        task.TaskFileWrite,
		},
		Parameters: task.FileWriteParameters{
			FilePath: filepath.Join(tempDir, "child2.txt"),
			Content:  "Content from child 2",
		},
	}

	// Create the group task
	groupTask := task.NewGroupTask("group-1", "Group with two children", []*task.Task{child1, child2})

	// Execute the group task
	executor, err := registry.GetExecutor(task.TaskGroup)
	if err != nil {
		t.Fatalf("Failed to get GroupExecutor: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resultsChan, err := executor.Execute(ctx, groupTask)
	if err != nil {
		t.Fatalf("Failed to execute group task: %v", err)
	}

	// Wait for all results
	var lastResult task.OutputResult
	for result := range resultsChan {
		lastResult = result
	}

	// Verify group task succeeded
	assert.Equal(t, task.StatusSucceeded, lastResult.Status, "Group task should have succeeded")

	// Verify child1 status was updated
	assert.Equal(t, task.StatusSucceeded, child1.Status, "Child1 status should be updated to SUCCEEDED")
	assert.NotEmpty(t, child1.Output, "Child1 output should not be empty")
	assert.Equal(t, child1.TaskId, child1.Output.TaskID, "Child1 output TaskID should match child1 TaskId")

	// Verify child2 status was updated
	assert.Equal(t, task.StatusSucceeded, child2.Status, "Child2 status should be updated to SUCCEEDED")
	assert.NotEmpty(t, child2.Output, "Child2 output should not be empty")
	assert.Equal(t, child2.TaskId, child2.Output.TaskID, "Child2 output TaskID should match child2 TaskId")

	// Verify the files were created with correct content
	verifyFileContent(t, filepath.Join(tempDir, "child1.txt"), "Content from child 1")
	verifyFileContent(t, filepath.Join(tempDir, "child2.txt"), "Content from child 2")
}
