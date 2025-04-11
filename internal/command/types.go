package command

// CommandType represents the specific kind of command/step.
// It defines the action to be performed by the executor.
type CommandType string

const (
	// CmdBashExec represents a command to execute a bash script or command line instruction.
	CmdBashExec CommandType = "BASH_EXEC"
	// CmdFileRead represents a command to read the content of a file.
	CmdFileRead CommandType = "FILE_READ"
	// CmdFileWrite represents a command to write content to a file, potentially overwriting it.
	CmdFileWrite CommandType = "FILE_WRITE"
	// CmdPatchFile represents a command to apply a patch to an existing file.
	CmdPatchFile CommandType = "PATCH_FILE"
	// CmdListDirectory represents a command to list the contents of a directory.
	CmdListDirectory CommandType = "LIST_DIRECTORY"
	// CmdRequestUserInput represents a command to prompt the user for input.
	CmdRequestUserInput CommandType = "REQUEST_USER_INPUT"
)

// ExecutionStatus indicates the outcome of an individual command execution attempt.
// It reflects the state of the command processing.
type ExecutionStatus string

const (
	// StatusRunning indicates that the command execution is currently in progress.
	StatusRunning ExecutionStatus = "RUNNING"
	// StatusSucceeded indicates that the command execution completed successfully.
	StatusSucceeded ExecutionStatus = "SUCCEEDED"
	// StatusFailed indicates that the command execution failed. Details may be in the OutputResult.
	StatusFailed ExecutionStatus = "FAILED"
)

// BaseCommand holds fields common to all command types/steps.
// It uses struct embedding in concrete command types.
type BaseCommand struct {
	// CommandID is a unique identifier for this specific command instance.
	CommandID string `json:"command_id"`
	// Description provides a human-readable explanation of the command's purpose.
	Description string `json:"description"`
}

// BashExecCommand defines the structure for executing a bash command.
type BashExecCommand struct {
	BaseCommand
	// Command is the actual bash command string to be executed.
	Command string `json:"command"`
}

// FileReadCommand defines the structure for reading a file.
type FileReadCommand struct {
	BaseCommand
	// FilePath is the path to the file that needs to be read.
	FilePath string `json:"file_path"`
}

// FileWriteCommand defines the structure for writing content to a file.
type FileWriteCommand struct {
	BaseCommand
	// FilePath is the path to the file where content will be written.
	FilePath string `json:"file_path"`
	// Content is the string data to be written into the file.
	Content string `json:"content"`
}

// PatchFileCommand defines the structure for applying a patch to a file.
type PatchFileCommand struct {
	BaseCommand
	// FilePath is the path to the file that will be patched.
	FilePath string `json:"file_path"`
	// Patch contains the patch content, typically in a standard format like unified diff.
	Patch string `json:"patch"`
}

// ListDirectoryCommand defines the structure for listing directory contents.
type ListDirectoryCommand struct {
	BaseCommand
	// Path is the directory whose contents should be listed (non-recursively).
	Path string `json:"path"`
}

// RequestUserInput defines the structure for requesting input from the user.
type RequestUserInput struct {
	BaseCommand
	// Prompt is the message displayed to the user to ask for input.
	Prompt string `json:"prompt"`
}

// OutputResult defines the structure of the result returned after executing a command.
// It provides status, messages, potential errors, and command-specific data.
type OutputResult struct {
	// CommandID links the result back to the specific command instance that was executed.
	CommandID string `json:"command_id"`
	// CommandType indicates the type of command that produced this result.
	CommandType CommandType `json:"commandType"`
	// Status reflects the final execution status (e.g., SUCCEEDED, FAILED).
	Status ExecutionStatus `json:"status"`
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
