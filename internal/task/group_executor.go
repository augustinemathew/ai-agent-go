package task

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// GroupExecutor handles the execution of GroupTask.
// It manages executing a collection of child tasks, tracking their results,
// and determining the overall outcome.
type GroupExecutor struct {
	registry TaskRegistry
}

// NewGroupExecutor creates a new GroupExecutor.
func NewGroupExecutor(registry TaskRegistry) *GroupExecutor {
	return &GroupExecutor{
		registry: registry,
	}
}

// Execute implements the TaskExecutor interface for GroupTask.
// It processes each child task sequentially, tracking their results.
// The GROUP task fails if any child task fails.
func (e *GroupExecutor) Execute(ctx context.Context, cmd any) (<-chan OutputResult, error) {
	var children []*Task
	var taskId string
	var taskStatus TaskStatus
	var taskOutput OutputResult

	switch v := cmd.(type) {
	case *Task:
		if v.Type != TaskGroup {
			return nil, fmt.Errorf("invalid task type: expected TaskGroup, got %s", v.Type)
		}
		children = v.Children
		taskId = v.TaskId
		taskStatus = v.Status
		taskOutput = v.Output
	default:
		return nil, fmt.Errorf("invalid command type: expected *Task, got %T", cmd)
	}

	// If the task is already in a terminal state, return it as is
	terminalChan, err := HandleTerminalTask(taskId, taskStatus, taskOutput)
	if err != nil || terminalChan != nil {
		return terminalChan, err
	}

	if len(children) == 0 {
		return nil, fmt.Errorf("group task has no children")
	}

	results := make(chan OutputResult, 2) // Buffer for at least the running and final states

	go e.executeGroupTask(ctx, taskId, children, results)
	return results, nil
}

// executeGroupTask handles the execution of all child tasks in a separate goroutine.
func (e *GroupExecutor) executeGroupTask(ctx context.Context, taskId string, children []*Task, results chan<- OutputResult) {
	defer close(results)

	// Send initial running status
	results <- OutputResult{
		TaskID:  taskId,
		Status:  StatusRunning,
		Message: fmt.Sprintf("Starting execution of group task with %d children", len(children)),
	}

	startTime := time.Now()
	var allResults []string
	var allErrors []string
	var failedTasks int
	var processedTasks int

	// Create a child context that can be canceled if needed
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Process each child task
	for i, childTask := range children {
		// Check if the parent context is already done
		if ctx.Err() != nil {
			results <- OutputResult{
				TaskID:  taskId,
				Status:  StatusFailed,
				Message: fmt.Sprintf("Group task execution canceled after completing %d/%d child tasks", processedTasks, len(children)),
				Error:   ctx.Err().Error(),
			}
			return
		}

		// Skip tasks that are already in a terminal state
		if childTask.Status.IsTerminal() {
			// If a task is already in a terminal state, count it appropriately
			if childTask.Status == StatusFailed {
				failedTasks++
				allErrors = append(allErrors, fmt.Sprintf("Task %s already in FAILED state", childTask.TaskId))
			}
			processedTasks++
			continue
		}

		// Process the child task
		childResult := e.processChildTask(childCtx, childTask)
		processedTasks++

		// Collect the result
		if childResult.Error != "" {
			failedTasks++
			allErrors = append(allErrors, fmt.Sprintf("Task %s failed: %s", childResult.TaskID, childResult.Error))

			// Report progress for the failed task
			results <- OutputResult{
				TaskID:  taskId,
				Status:  StatusRunning,
				Message: fmt.Sprintf("Child task %d/%d failed (%s)", i+1, len(children), childResult.Status),
			}

			// Stop processing remaining tasks once one fails
			break
		}

		if childResult.ResultData != "" {
			allResults = append(allResults, childResult.ResultData)
		}

		// Report progress
		results <- OutputResult{
			TaskID:  taskId,
			Status:  StatusRunning,
			Message: fmt.Sprintf("Completed child task %d/%d (%s)", i+1, len(children), childResult.Status),
		}
	}

	// Determine final status
	finalStatus := StatusSucceeded
	var finalMessage string
	var finalError string

	if failedTasks > 0 {
		finalStatus = StatusFailed
		finalMessage = fmt.Sprintf("Group task completed with %d/%d failed tasks in %v", failedTasks, processedTasks, time.Since(startTime).Round(time.Millisecond))
		finalError = strings.Join(allErrors, "\n")
	} else {
		finalMessage = fmt.Sprintf("Group task completed successfully with %d child tasks in %v", processedTasks, time.Since(startTime).Round(time.Millisecond))
	}

	// Send final result
	finalResult := OutputResult{
		TaskID:     taskId,
		Status:     finalStatus,
		Message:    finalMessage,
		Error:      finalError,
		ResultData: strings.Join(allResults, "\n"),
	}

	results <- finalResult
}

// processChildTask handles the execution of a single child task and returns its final result.
func (e *GroupExecutor) processChildTask(ctx context.Context, childTask *Task) OutputResult {
	// Use the task status as-is if pending, otherwise set to running
	taskStatus := childTask.Status
	if taskStatus.IsPending() {
		taskStatus = StatusRunning
	}

	// Get the appropriate executor for this task type
	executor, err := e.registry.GetExecutor(childTask.Type)
	if err != nil {
		finalResult := OutputResult{
			TaskID:  childTask.TaskId,
			Status:  StatusFailed,
			Message: "Failed to get executor for child task",
			Error:   err.Error(),
		}
		// Update child task status and output
		childTask.Status = finalResult.Status
		childTask.Output = finalResult
		return finalResult
	}

	// Convert the generic Task to the appropriate concrete type based on its type
	var concreteTask interface{}
	var executeErr error

	switch childTask.Type {
	case TaskFileWrite:
		if params, ok := childTask.Parameters.(FileWriteParameters); ok {
			concreteTask = NewFileWriteTask(childTask.TaskId, childTask.Description, params)
		} else {
			executeErr = fmt.Errorf("invalid parameters for FileWriteCommand: %T", childTask.Parameters)
		}
	case TaskFileRead:
		if params, ok := childTask.Parameters.(FileReadParameters); ok {
			concreteTask = NewFileReadTask(childTask.TaskId, childTask.Description, params)
		} else {
			executeErr = fmt.Errorf("invalid parameters for FileReadTask: %T", childTask.Parameters)
		}
	case TaskBashExec:
		if params, ok := childTask.Parameters.(BashExecParameters); ok {
			concreteTask = NewBashExecTask(childTask.TaskId, childTask.Description, params)
		} else {
			executeErr = fmt.Errorf("invalid parameters for BashExecTask: %T", childTask.Parameters)
		}
	case TaskPatchFile:
		if params, ok := childTask.Parameters.(PatchFileParameters); ok {
			concreteTask = NewPatchFileTask(childTask.TaskId, childTask.Description, params)
		} else {
			executeErr = fmt.Errorf("invalid parameters for PatchFileCommand: %T", childTask.Parameters)
		}
	case TaskListDirectory:
		if params, ok := childTask.Parameters.(ListDirectoryParameters); ok {
			concreteTask = NewListDirectoryTask(childTask.TaskId, childTask.Description, params)
		} else {
			executeErr = fmt.Errorf("invalid parameters for ListDirectoryCommand: %T", childTask.Parameters)
		}
	case TaskRequestUserInput:
		if params, ok := childTask.Parameters.(RequestUserInputParameters); ok {
			concreteTask = NewRequestUserInputTask(childTask.TaskId, childTask.Description, params)
		} else {
			executeErr = fmt.Errorf("invalid parameters for RequestUserInput: %T", childTask.Parameters)
		}
	case TaskGroup:
		// For groups, create a new Task with the same status
		newTask := childTask
		concreteTask = newTask
	default:
		executeErr = fmt.Errorf("unsupported task type: %s", childTask.Type)
	}

	// If there was an error preparing the concrete task, return a failure result
	if executeErr != nil {
		finalResult := OutputResult{
			TaskID:  childTask.TaskId,
			Status:  StatusFailed,
			Message: "Failed to create concrete task",
			Error:   executeErr.Error(),
		}
		// Update child task status and output
		childTask.Status = finalResult.Status
		childTask.Output = finalResult
		return finalResult
	}

	// Execute the child task with the appropriate concrete type
	childResultsChan, err := executor.Execute(ctx, concreteTask)
	if err != nil {
		finalResult := OutputResult{
			TaskID:  childTask.TaskId,
			Status:  StatusFailed,
			Message: "Failed to execute child task",
			Error:   err.Error(),
		}
		// Update child task status and output
		childTask.Status = finalResult.Status
		childTask.Output = finalResult
		return finalResult
	}

	// Collect all results from the child task
	var lastResult OutputResult
	var resultData strings.Builder

	// Read all results from the channel
	for result := range childResultsChan {
		lastResult = result
		if result.ResultData != "" {
			resultData.WriteString(result.ResultData)
		}
	}

	// Create the final child result
	finalResult := lastResult
	if resultData.Len() > 0 {
		finalResult.ResultData = resultData.String()
	}

	// Update child task status and output based on final result
	childTask.Status = finalResult.Status
	childTask.Output = finalResult

	// Update the concrete task's status if it's a Task type
	if ct, ok := concreteTask.(*Task); ok {
		ct.Status = finalResult.Status
		ct.Output = finalResult
	}

	return finalResult
}
