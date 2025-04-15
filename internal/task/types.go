package task

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
type BashExecTask struct {
	BaseTask
	Parameters BashExecParameters `json:"parameters"`
}

type FileReadParameters struct {
	BaseParameters
	FilePath  string `json:"file_path"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}

// FileReadTask defines the structure for reading a file.
type FileReadTask struct {
	BaseTask
	Parameters FileReadParameters `json:"parameters"`
}

type FileWriteParameters struct {
	BaseParameters
	FilePath  string `json:"file_path"`
	Content   string `json:"content"`
	Overwrite bool   `json:"overwrite,omitempty"`
}

// FileWriteTask defines the structure for writing content to a file.
type FileWriteTask struct {
	BaseTask
	Parameters FileWriteParameters `json:"parameters"`
}

type PatchFileParameters struct {
	BaseParameters
	FilePath string `json:"file_path"`
	Patch    string `json:"patch"`
}

// PatchFileTask defines the structure for applying a patch to a file.
type PatchFileTask struct {
	BaseTask
	Parameters PatchFileParameters `json:"parameters"`
}

type ListDirectoryParameters struct {
	BaseParameters
	Path string `json:"path"`
}

// ListDirectoryTask defines the structure for listing directory contents.
type ListDirectoryTask struct {
	BaseTask
	Parameters ListDirectoryParameters `json:"parameters"`
}

type RequestUserInputParameters struct {
	BaseParameters
	Prompt string `json:"prompt"`
}

// RequestUserInputTask defines the structure for requesting input from the user.
type RequestUserInputTask struct {
	BaseTask
	Parameters RequestUserInputParameters `json:"parameters"`
}

// GroupTask defines the structure for a group of tasks that will be executed in sequence.
type GroupTask struct {
	BaseTask
	// No additional parameters needed for GroupTask since it uses the Children field in BaseTask
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
