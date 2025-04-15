package task

import (
	"encoding/json"
)

// TaskType represents the specific kind of command/step.
// It defines the action to be performed by the executor.
type TaskType string

const (
	// TaskBashExec represents a command to execute a bash script or command line instruction.
	TaskBashExec TaskType = "BASH_EXEC"
	// TaskFileRead represents a command to read the content of a file.
	TaskFileRead TaskType = "FILE_READ"
	// TaskFileWrite represents a command to write content to a file, potentially overwriting it.
	TaskFileWrite TaskType = "FILE_WRITE"
	// TaskPatchFile represents a command to apply a patch to an existing file.
	TaskPatchFile TaskType = "PATCH_FILE"
	// TaskListDirectory represents a command to list the contents of a directory.
	TaskListDirectory TaskType = "LIST_DIRECTORY"
	// TaskRequestUserInput represents a command to prompt the user for input.
	TaskRequestUserInput TaskType = "REQUEST_USER_INPUT"
	// TaskGroup represents a group of tasks to be executed in sequence.
	// If any task fails, the group fails.
	TaskGroup TaskType = "GROUP"
)

// TaskStatus indicates the outcome of an individual command execution attempt.
// It reflects the state of the command processing.
type TaskStatus string

const (
	// StatusPending represents a command that hasn't started execution
	// An empty status value is equivalent to StatusPending
	StatusPending TaskStatus = ""
	// StatusRunning represents a command that is currently being executed
	StatusRunning TaskStatus = "RUNNING"
	// StatusSucceeded represents a successfully executed command
	StatusSucceeded TaskStatus = "SUCCEEDED"
	// StatusFailed represents a failed command execution
	StatusFailed TaskStatus = "FAILED"
)

// IsTerminal returns true if the status represents a terminal state
// (either SUCCEEDED or FAILED).
func (s TaskStatus) IsTerminal() bool {
	return s == StatusSucceeded || s == StatusFailed
}

// IsPending returns true if the status is empty ("") or explicitly PENDING.
func (s TaskStatus) IsPending() bool {
	return s == "" || s == StatusPending
}

// BaseTask holds fields common to all command types/steps.
// It uses struct embedding in concrete command types.
type BaseTask struct {
	// TaskId is a unique identifier for this specific command instance.
	TaskId string `json:"task_id"`
	// Description provides a human-readable explanation of the command's purpose.
	Description string `json:"description"`
	// Status reflects the final execution status (RUNNING, SUCCEEDED, FAILED).
	Status TaskStatus `json:"status"`
	// TaskType indicates the type of task.
	Type TaskType `json:"type"`
	// Children is an array of sub-tasks. Only used for TaskGroup type.
	Children []*Task `json:"children,omitempty"`
	// Output holds the result of the command execution.
	// This is set by the executor when the command is finished.
	Output OutputResult `json:"output,omitempty"`
}

// Task is a union type representing any task type
type Task struct {
	BaseTask
	Parameters interface{} `json:"parameters,omitempty"`
}

// BaseParameters holds fields common to all command parameters.
type BaseParameters struct {
	// WorkingDirectory is the directory in which the command will be executed.
	// If not provided, the command will run in the default directory.
	WorkingDirectory string `json:"working_directory"`
}

// BashExecParameters holds parameters specific to the BashExecTask.
type BashExecParameters struct {
	BaseParameters
	// Command is the actual bash command string to be executed.
	// Multiple commands can be provided as a multi-line string.
	Command string `json:"command"`
}

// BashExecTask defines the structure for executing a bash command.
func NewBashExecTask(taskId string, description string, parameters BashExecParameters) *Task {
	return &Task{
		BaseTask:   BaseTask{TaskId: taskId, Type: TaskBashExec, Description: description},
		Parameters: parameters,
	}
}

type FileReadParameters struct {
	BaseParameters
	FilePath  string `json:"file_path"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}

func NewFileReadTask(taskId string, description string, parameters FileReadParameters) *Task {
	return &Task{
		BaseTask:   BaseTask{TaskId: taskId, Type: TaskFileRead, Description: description},
		Parameters: parameters,
	}
}

type FileWriteParameters struct {
	BaseParameters
	FilePath  string `json:"file_path"`
	Content   string `json:"content"`
	Overwrite bool   `json:"overwrite,omitempty"`
}

func NewFileWriteTask(taskId string, description string, parameters FileWriteParameters) *Task {
	return &Task{
		BaseTask:   BaseTask{TaskId: taskId, Type: TaskFileWrite, Description: description},
		Parameters: parameters,
	}
}

type PatchFileParameters struct {
	BaseParameters
	FilePath string `json:"file_path"`
	Patch    string `json:"patch"`
}

// PatchFileTask defines the structure for applying a patch to a file.
func NewPatchFileTask(taskId string, description string, parameters PatchFileParameters) *Task {
	return &Task{
		BaseTask:   BaseTask{TaskId: taskId, Type: TaskPatchFile, Description: description},
		Parameters: parameters,
	}
}

type ListDirectoryParameters struct {
	BaseParameters
	Path string `json:"path"`
}

// ListDirectoryTask defines the structure for listing directory contents.
func NewListDirectoryTask(taskId string, description string, parameters ListDirectoryParameters) *Task {
	return &Task{
		BaseTask:   BaseTask{TaskId: taskId, Type: TaskListDirectory, Description: description},
		Parameters: parameters,
	}
}

type RequestUserInputParameters struct {
	BaseParameters
	Prompt string `json:"prompt"`
}

func NewRequestUserInputTask(taskId string, description string, parameters RequestUserInputParameters) *Task {
	return &Task{
		BaseTask:   BaseTask{TaskId: taskId, Type: TaskRequestUserInput, Description: description},
		Parameters: parameters,
	}
}

// GroupTask defines the structure for a group of tasks that will be executed in sequence.
func NewGroupTask(taskId string, description string, children []*Task) *Task {
	return &Task{
		BaseTask: BaseTask{
			TaskId:      taskId,
			Type:        TaskGroup,
			Description: description,
			Children:    children,
		},
	}
}

// OutputResult defines the structure of the result returned after executing a command.
// It provides status, messages, potential errors, and command-specific data.
type OutputResult struct {
	// TaskID links the result back to the specific command instance that was executed.
	TaskID string `json:"task_id"`
	// Status reflects the final execution status (RUNNING, SUCCEEDED, FAILED).
	Status TaskStatus `json:"status"`
	// Message provides a human-readable summary or status update about the execution.
	Message string `json:"message"`
	// Error contains details about any error that occurred during execution. It's empty on success.
	Error string `json:"error,omitempty"`
	// ResultData holds command-specific output as a string.
	// For BashExec, it's stdout.
	// For FileRead, it's the file content.
	// For ListDirectory, it's a newline-separated list of entries.
	// For others like FileWrite or PatchFile, it might be empty if success is indicated by Status.
	ResultData string `json:"resultData,omitempty"`
}

// Command is a generic interface that all command structs should implicitly satisfy.
// This is used primarily for type assertions and generic handling,
// although Go's type system handles this implicitly via struct embedding and interfaces.
// We don't strictly need this interface definition but include it for clarity.
// type Command interface {
//	// IsCommand is a marker method; its presence indicates a struct is a command.
//	// Actual command identification happens via type switches or reflection if needed.
//	IsCommand()
// }

// Implement IsCommand marker for BaseCommand to satisfy the conceptual Command interface.
// Note: Go doesn't require explicit interface implementation declarations.
// func (bc BaseCommand) IsCommand() {}

// UpdateOutput updates the task's Output field with the provided OutputResult.
// It ensures that the Status field in both the BaseTask and its Output are consistent.
func (bt *BaseTask) UpdateOutput(output *OutputResult) {
	if output == nil {
		return
	}

	// Make a copy of the output
	outputCopy := *output

	// Ensure the output status matches the task status
	outputCopy.Status = bt.Status

	// Update the task output
	bt.Output = outputCopy
}

// MarshalJSON implements custom JSON marshaling for Task to handle dynamic Parameters typing
func (t *Task) MarshalJSON() ([]byte, error) {
	// Create a map to hold all fields
	data := make(map[string]interface{})

	// Add all BaseTask fields
	data["task_id"] = t.TaskId
	data["description"] = t.Description
	data["status"] = t.Status
	data["type"] = t.Type

	// Add Children if not nil
	if t.Children != nil {
		data["children"] = t.Children
	}

	// Add Output if not empty
	if t.Output != (OutputResult{}) {
		data["output"] = t.Output
	}

	// Add Parameters based on task type
	if t.Parameters != nil {
		data["parameters"] = t.Parameters
	}

	// Marshal the map to JSON
	return json.Marshal(data)
}

// UnmarshalJSON implements custom JSON unmarshaling for Task
// It correctly handles the dynamic Parameters field based on TaskType
func (t *Task) UnmarshalJSON(data []byte) error {
	// First, unmarshal into a map to get the type field
	var objMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &objMap); err != nil {
		return err
	}

	// Extract type field to determine parameter type
	var taskType TaskType
	if typeData, ok := objMap["type"]; ok {
		if err := json.Unmarshal(typeData, &taskType); err != nil {
			return err
		}
	}

	// Create task base structure
	var baseTask BaseTask
	if err := json.Unmarshal(data, &baseTask); err != nil {
		return err
	}

	// Set BaseTask fields
	t.BaseTask = baseTask

	// Handle parameters based on task type if parameters field exists
	if paramsData, ok := objMap["parameters"]; ok {
		switch taskType {
		case TaskBashExec:
			var params BashExecParameters
			if err := json.Unmarshal(paramsData, &params); err != nil {
				return err
			}
			t.Parameters = params

		case TaskFileRead:
			var params FileReadParameters
			if err := json.Unmarshal(paramsData, &params); err != nil {
				return err
			}
			t.Parameters = params

		case TaskFileWrite:
			var params FileWriteParameters
			if err := json.Unmarshal(paramsData, &params); err != nil {
				return err
			}
			t.Parameters = params

		case TaskPatchFile:
			var params PatchFileParameters
			if err := json.Unmarshal(paramsData, &params); err != nil {
				return err
			}
			t.Parameters = params

		case TaskListDirectory:
			var params ListDirectoryParameters
			if err := json.Unmarshal(paramsData, &params); err != nil {
				return err
			}
			t.Parameters = params

		case TaskRequestUserInput:
			var params RequestUserInputParameters
			if err := json.Unmarshal(paramsData, &params); err != nil {
				return err
			}
			t.Parameters = params

		case TaskGroup:
			// GroupTask doesn't have parameters - it uses Children
		}
	}

	return nil
}

// ToJSON converts a Task to a JSON string
func (t *Task) ToJSON() (string, error) {
	data, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ToPrettyJSON converts a Task to a formatted JSON string with indentation
func (t *Task) ToPrettyJSON() (string, error) {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FromJSON creates a Task from a JSON string
func FromJSON(jsonStr string) (*Task, error) {
	task := &Task{}
	err := json.Unmarshal([]byte(jsonStr), task)
	if err != nil {
		return nil, err
	}
	return task, nil
}
