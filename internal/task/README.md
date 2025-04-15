# Task Execution Package (`internal/task`)

This package defines and executes various tasks asynchronously within the AI agent backend. It provides a structured way to represent tasks and their results using JSON, facilitating communication between different parts of the system (e.g., a web server handling requests and a background worker executing tasks).

## Core Concepts

1.  **Tasks**: Represent specific actions to be performed. Each task type has a dedicated struct embedding `BaseTask`.
2.  **Executors**: Responsible for handling the execution logic for a specific `TaskType`. Each executor implements the `Executor` interface.
3.  **Registry**: Maps `TaskType` values to their corresponding `Executor` implementations.
4.  **Results**: The `OutputResult` struct standardizes the format for reporting the outcome of a task execution, including status, messages, errors, and task-specific data. Results are sent over a channel.
5.  **Task Output**: Tasks store their final execution result in the `Output` field. The status of a task and its output are kept in sync using the `UpdateOutput` method.

## Task Status

Tasks transition through different states during their lifecycle:

```
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
```

## Implementation Details

The package implements several important design patterns:

1. **Context-Aware Goroutines**: All executor implementations properly respect context cancellation, allowing for graceful cleanup when operations are interrupted.
2. **Context-Aware WaitGroups**: A specialized pattern is implemented to make WaitGroup operations respect context cancellation.
3. **Clean Error Handling**: Standardized error reporting throughout the system with appropriate status codes.
4. **Functional Decomposition**: Each executor is broken down into smaller, focused functions for better maintainability.

## Task Reference

This section details the specific tasks supported by the package, including their purpose, input JSON structure, and example output JSON upon success.

All task inputs share the following base fields:

```json
{
  "task_id": "string", // Unique identifier for this task instance
  "description": "string" // Human-readable description (optional but recommended)
}
```

All task outputs (`OutputResult`) share the following general structure. Specific details, especially for `resultData`, vary by task type.

```json
{
  "task_id": "string",     // Matches the task_id of the originating task
  "status": "string",      // Execution status: "" (PENDING), "RUNNING", "SUCCEEDED", "FAILED"
  "message": "string",     // Human-readable summary/status update
  "error": "string,omitempty", // Error details if status is "FAILED", otherwise omitted
  "resultData": "string,omitempty" // Task-specific output data, omitted if not applicable
}
```

## Task Output Management

Tasks maintain their own status and can store the final output result within the `Output` field of `BaseTask`. The `UpdateOutput` method ensures consistency between the task status and the output status:

```go
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
```

This method should be used whenever you need to update a task's output while maintaining status consistency.

**Example Usage in an Executor:**

```go
// Inside an executor's Execute method:
func (e *SampleExecutor) Execute(ctx context.Context, cmd any) (<-chan OutputResult, error) {
    // Cast to specific task type
    task, ok := cmd.(SampleTask)
    if !ok {
        return nil, fmt.Errorf("invalid command type: %T", cmd)
    }

    // Create results channel
    results := make(chan OutputResult, 1)
    
    go func() {
        defer close(results)
        
        // Set task to running
        task.Status = StatusRunning
        
        // Send initial status
        results <- OutputResult{
            TaskID:  task.TaskId,
            Status:  StatusRunning,
            Message: "Task started",
        }
        
        // Process task...
        
        // Create final result
        finalResult := OutputResult{
            TaskID:     task.TaskId,
            Status:     StatusSucceeded, // This will be overwritten by task's status
            Message:    "Task completed",
            ResultData: "Sample result",
        }
        
        // Update task's status based on execution outcome
        task.Status = StatusSucceeded // or StatusFailed if there was an error
        
        // Update task's output with the final result
        // This ensures output.Status == task.Status
        task.UpdateOutput(&finalResult)
        
        // Send final result
        results <- finalResult
    }()
    
    return results, nil
}
```

---

### `BASH_EXEC`

Executes a shell command (`BashExecTask`). Supports both single-line and multiline bash scripts.

**Input JSON:**

```json
{
  "task_id": "unique-id-1",
  "description": "Run multiline script",
  "parameters": {
    "command": "echo \"Starting multiline script...\"\necho \"Current directory: $(pwd)\"\nls -la\necho \"Environment variables:\"\nenv | grep PATH\necho \"Script complete!\""
  }
}
```

**Output JSON (Success Example):**

```json
{
  "task_id": "happy-bash-2",
  "status": "SUCCEEDED",
  "message": "Command finished in 10ms. Final CWD: /Users/augustine/ai-agent-4/ai-agent-go.",
  "resultData": "Starting multiline script...\nCurrent directory: /Users/augustine/ai-agent-4/ai-agent-go\ntotal 24\ndrwxr-xr-x   9 augustine  staff   288 Apr 14 22:43 .\ndrwxr-xr-x   3 augustine  staff    96 Apr 14 17:50 ..\ndrwxr-xr-x@  3 augustine  staff    96 Apr 14 18:25 .cursor\ndrwxr-xr-x  12 augustine  staff   384 Apr 14 23:09 .git\n-rw-r--r--   1 augustine  staff   414 Apr 14 17:50 .gitignore\ndrwxr-xr-x   3 augustine  staff    96 Apr 14 17:50 cmd\n-rw-r--r--@  1 augustine  staff   299 Apr 14 18:00 go.mod\n-rw-r--r--   1 augustine  staff  1658 Apr 14 17:50 go.sum\ndrwxr-xr-x   3 augustine  staff    96 Apr 14 17:55 internal\nEnvironment variables:\nPATH=/opt/homebrew/Cellar/go/1.24.1/libexec/bin:/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/System/Cryptexes/App/usr/bin:/usr/bin:/bin:/usr/sbin:/sbin:/var/run/com.apple.security.cryptexd/codex.system/bootstrap/usr/local/bin:/var/run/com.apple.security.cryptexd/codex.system/bootstrap/usr/bin:/var/run/com.apple.security.cryptexd/codex.system/bootstrap/usr/appleinternal/bin:/opt/homebrew/Caskroom/miniconda/base/bin:/opt/homebrew/Caskroom/miniconda/base/condabin\nINFOPATH=/opt/homebrew/share/info:/opt/homebrew/share/info:\nScript complete!\nStarting main script execution...\nInitial directory: /Users/augustine/ai-agent-4/ai-agent-go\n---\n\n############################################\n# Script Exiting\n# Exit Status: 0\n# Final Working Directory: /Users/augustine/ai-agent-4/ai-agent-go\n############################################\n"
}
```

---

### `FILE_READ`

Reads the contents of a file, optionally from specific line numbers.

**Input JSON:**

```json
{
  "task_id": "read-1",
  "description": "Read file contents",
  "parameters": {
    "file_path": "/path/to/file.txt",
    "start_line": 2,  // Optional: 1-based line number to start reading from (0 means start from beginning)
    "end_line": 4     // Optional: 1-based line number to read until (0 means read until end)
  }
}
```

**Example Output JSON:**

```json
{
  "task_id": "happy-read-3",
  "status": "SUCCEEDED",
  "message": "File reading finished successfully in 0s.",
  "resultData": "This is a test file.\n"
}
```

**Notes:**
- If `start_line` is 0 or omitted, reading starts from the beginning of the file
- If `end_line` is 0 or omitted, reading continues until the end of the file
- Line numbers are 1-based (first line is line 1)
- Invalid line numbers (negative) will result in a failure
- If `start_line` is after `end_line`, the command will fail

---

### `FILE_WRITE`

Writes content to a file, overwriting if it exists (`FileWriteCommand`).

**Input JSON:**

```json
{
  "task_id": "unique-id-3",
  "description": "Write initial data",
  "parameters": {
    "file_path": "/path/to/output.txt", // Path to the file to write
    "content": "This is the content to write.\nSecond line."
  }
}
```

**Output JSON (Success Example):**

```json
{
  "task_id": "happy-write-1",
  "status": "SUCCEEDED",
  "message": "File writing finished successfully to '/var/folders/99/0xjhznh90fldmj20kkw_s_km0000gn/T/cmd_runner_demo_1744697480525532000.txt' in 0s."
}
```

---

### `PATCH_FILE`

Applies a patch (e.g., in unified diff format) to a file (`PatchFileCommand`).

**Note:** This command can also be used to **create a new file** by providing a patch that adds content relative to an empty file (typically indicated with `--- /dev/null` in the patch header).

**Input JSON (Modify Existing File):**

```json
{
  "task_id": "unique-id-4",
  "description": "Apply code changes",
  "parameters": {
    "file_path": "/path/to/source.go", // Path to the file to patch
    "patch": "--- a/source.go\n+++ b/source.go\n@@ -1,1 +1,2 @@\n package main\n+import \"fmt\"" // Patch content
  }
}
```

**Input JSON (Create New File):**

```json
{
  "task_id": "unique-id-create-5",
  "description": "Create a new file with initial content",
  "parameters": {
    "file_path": "/path/to/new_file.txt", // Path for the new file
    "patch": "--- /dev/null\n+++ b/new_file.txt\n@@ -0,0 +1,3 @@\n+First line.\n+Second line.\n+Third line.\n" // Patch starting from empty
  }
}
```
*(Note: The patch format itself depends on the implementation, but unified diff is common).*

**Output JSON (Success Example):**

```json
{
  "task_id": "happy-patch-1",
  "status": "SUCCEEDED",
  "message": "Successfully patched file /var/folders/99/0xjhznh90fldmj20kkw_s_km0000gn/T/cmd_runner_demo_1744697480525532000.txt"
}
```

---

### `LIST_DIRECTORY`

Lists the contents of a directory (`ListDirectoryCommand`).

**Input JSON:**

```json
{
  "task_id": "unique-id-5",
  "description": "List project root",
  "parameters": {
    "path": "/path/to/directory" // Path to the directory to list
  }
}
```

**Output JSON (Success Example):**

```json
{
  "task_id": "happy-list-1",
  "status": "SUCCEEDED",
  "message": "Successfully listed directory '/var/folders/99/0xjhznh90fldmj20kkw_s_km0000gn/T/' in 1ms.",
  "resultData": "Listing for /var/folders/99/0xjhznh90fldmj20kkw_s_km0000gn/T:\n  [DIR ] drwx------ 2025-04-14T21:48:20-07:00         64 ${DaemonNameOrIdentifierHere}\n  [DIR ] drwx------ 2025-04-13T11:53:28-07:00      128 .AddressBookLocks\n... (many more directory entries)"
}
```
*(Note: The exact format of `resultData` might vary slightly based on the OS, but the structure `[TYPE] Mode Modified Size Name` is consistent. Output is truncated for brevity.)*

---

### `REQUEST_USER_INPUT`

Prompts the user for input (`RequestUserInput`). The mechanism for displaying the prompt and receiving input depends on the executor's implementation.

**Input JSON:**

```json
{
  "task_id": "unique-id-6",
  "description": "Ask for API key",
  "parameters": {
    "prompt": "Please enter your API Key:" // Message displayed to the user
  }
}
```

**Output JSON (Conceptual Success Examples):**

The format depends on how user input is handled. `resultData` might contain the user's response, or it might be empty if the response is handled separately.

*   **(If response included in `resultData`):**
    ```json
    {
      "task_id": "unique-id-6",
      "status": "SUCCEEDED",
      "message": "User provided input.",
      "resultData": "user-provided-api-key" // Example: User input captured here
    }
    ```
*   **(If response handled elsewhere):**
    ```json
    {
      "task_id": "unique-id-6",
      "status": "SUCCEEDED",
      "message": "User input prompt displayed.",
      "resultData": ""
    }
    ```

---

### `GROUP`

Executes a group of tasks in sequence (`GroupTask`). If any task fails, the entire group fails and remaining tasks are not executed (fail-fast behavior).

**Input JSON:**

```json
{
  "task_id": "group-1",
  "description": "Execute a sequence of tasks",
  "type": "GROUP",
  "children": [
    {
      "task_id": "child-1",
      "description": "First child task",
      "type": "FILE_WRITE",
      "parameters": {
        "file_path": "/path/to/first-file.txt",
        "content": "Content for the first file"
      }
    },
    {
      "task_id": "child-2",
      "description": "Second child task",
      "type": "FILE_READ",
      "parameters": {
        "file_path": "/path/to/first-file.txt"
      }
    }
  ]
}
```

**Output JSON (Success Example):**

```json
{
  "task_id": "group-1",
  "status": "SUCCEEDED",
  "message": "Group task completed successfully with 2 child tasks in 15ms",
  "resultData": "Content for the first file"
}
```

**Output JSON (Failure Example):**

```json
{
  "task_id": "group-1",
  "status": "FAILED",
  "message": "Group task completed with 1/2 failed tasks in 10ms",
  "error": "Task child-2 failed: failed to open file '/path/to/nonexistent-file.txt': open /path/to/nonexistent-file.txt: no such file or directory"
}
```

**Implementation Details:**

The GroupExecutor provides a powerful task composition mechanism with the following key features:

1. **Task Type Flexibility**:
   - Accepts both specific `GroupTask` objects and generic `Task` objects
   - Automatically handles type conversion through Go's type switching

2. **Child Task Conversion**:
   - Automatically converts generic `Task` objects to concrete task types required by specific executors
   - Maintains all task properties during conversion
   - Provides detailed error messages if type conversion fails

3. **Hierarchical Composition**:
   - Supports nested groups (groups can contain other groups)
   - Allows building complex workflows from simple primitives
   - Maintains a clean hierarchy regardless of nesting depth

4. **Sequential Execution with Fail-Fast Behavior**:
   - Runs tasks sequentially in the order they appear in the children array
   - Stops execution immediately when a task fails (fail-fast)
   - Tasks start in an empty status (equivalent to PENDING) and transition to RUNNING, then SUCCEEDED or FAILED
   - Respects existing task states - already failed tasks are counted as failures
   - Skips tasks that are already in a terminal state (SUCCEEDED or FAILED)
   - Provides real-time status updates during execution
   - Reports progress after each child task completes

5. **Result Handling**:
   - Concatenates result data from all successfully executed child tasks
   - Collects detailed error information from any failing tasks
   - Provides execution statistics including processed and failed task counts
   - Execution statistics only include tasks that were processed, not skipped

**Usage Examples:**

* **Pipeline Processing**:
  ```json
  {
    "task_id": "data-pipeline",
    "type": "GROUP",
    "children": [
      {"task_id": "download", "type": "BASH_EXEC", "parameters": {"command": "curl -o data.json https://example.com/api/data"}},
      {"task_id": "process", "type": "BASH_EXEC", "parameters": {"command": "jq '.items[]' data.json > processed.json"}},
      {"task_id": "upload", "type": "BASH_EXEC", "parameters": {"command": "curl -X POST -d @processed.json https://example.com/api/upload"}}
    ]
  }
  ```

* **Nested Configuration**:
  ```json
  {
    "task_id": "project-setup",
    "type": "GROUP",
    "children": [
      {
        "task_id": "directory-setup",
        "type": "GROUP",
        "children": [
          {"task_id": "make-src", "type": "BASH_EXEC", "parameters": {"command": "mkdir -p src/main/java"}},
          {"task_id": "make-test", "type": "BASH_EXEC", "parameters": {"command": "mkdir -p src/test/java"}}
        ]
      },
      {
        "task_id": "file-creation",
        "type": "GROUP",
        "children": [
          {"task_id": "create-pom", "type": "FILE_WRITE", "parameters": {"file_path": "pom.xml", "content": "<project>...</project>"}},
          {"task_id": "create-readme", "type": "FILE_WRITE", "parameters": {"file_path": "README.md", "content": "# Project\n\nDescription here."}}
        ]
      }
    ]
  }
  ```

**Notes:**
- GROUP tasks support unlimited nesting depth (groups within groups)
- The `resultData` of a successful GROUP task contains the concatenated `resultData` of all child tasks
- If any child task fails, the GROUP task immediately reports a failure status
- Tasks continue to execute even after failures to ensure all operations have a chance to complete
- Each child task must include a valid `type` field that corresponds to a registered executor
- The GROUP executor intelligently handles both Task and concrete task type objects
- Progress updates are sent for each child task completion, making it easy to track long-running operations

---

## Usage

1.  **Initialization**: Create a `Registry`. All standard executors are registered automatically by `NewMapRegistry`.
    ```go
    package main

    import (
    	"fmt"
    	"log"
    	"os"
    	"time"

    	"your_project/internal/task" // Adjust import path
    )

    func main() {
    	// Create registry - standard executors are registered automatically.
    	registry := task.NewMapRegistry()

    	// Optional: Override or register custom executors if needed
    	// registry.Register(task.TaskBashExec, myCustomBashExecutor)

    	// ... rest of your application setup
    }

    ```

2.  **Get Executor**: When you receive a task (e.g., parsed from a JSON request), retrieve the appropriate executor from the registry using the task's type.

    ```go
    // Assume 'taskType' is determined from the incoming request/data
    taskType := task.TaskBashExec // Example

    executor, err := registry.GetExecutor(taskType)
    if err != nil {
    	log.Fatalf("Error getting executor for type %s: %v", taskType, err)
        // Handle error appropriately (e.g., return error response)
    }
    ```

3.  **Execute Task**: Call the `Execute` method on the retrieved executor, passing a context and the specific task struct (e.g., `BashExecTask`). The `Execute` method requires the task as an `any` type, so you'll typically pass a pointer to your specific task struct.

    ```go
    import "context"

    // Assume 'cmd' is the specific task struct instance (e.g., BashExecTask)
    // populated with data (e.g., from a JSON request).
    bashTask := task.BashExecTask{
        BaseTask: task.BaseTask{TaskId: "task-123", Description: "List root dir"},
        Parameters: task.BashExecParameters{
            Command: "ls -l /",
        },
    }

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // Example timeout
    defer cancel()

    // Pass a pointer to the task struct
    resultsChan, err := executor.Execute(ctx, &bashTask)
    if err != nil {
    	log.Printf("Error initiating task execution for %s: %v", bashTask.TaskId, err)
        // Handle initiation error (e.g., invalid task structure)
        return
    }
    ```

4.  **Process Results**: Read results from the returned channel asynchronously. The channel will close when execution is complete (successfully or with failure).

    ```go
    log.Printf("Waiting for results for task %s...", bashTask.TaskId)
    for result := range resultsChan {
    	// Process each result (e.g., log it, send updates via WebSocket, etc.)
    	log.Printf("Received result: %+v", result)

    	// Check for terminal states using the IsTerminal() helper
    	if result.Status.IsTerminal() {
    		log.Printf("Task %s finished with status: %s", result.TaskID, result.Status)
    		if result.Error != "" {
    			log.Printf("Error detail: %s", result.Error)
    		}
    		break // Or wait if intermediate results are possible after failure (unlikely for most tasks)
    	}
    }
    log.Printf("Result channel closed for task %s.", bashTask.TaskId)
    ```

## Result Collection Utility

For convenience, especially with streaming tasks (`BASH_EXEC`, `FILE_READ`), the package provides utility functions to simplify consuming the results channel.

These functions read all `OutputResult` messages from the channel until it either closes or the provided `context.Context` is cancelled. It then returns a *single* `OutputResult` summarizing the execution:

*   If the context is cancelled before the channel closes, it returns an `OutputResult` with `StatusFailed`, an error message indicating cancellation (`context.Canceled` or `context.DeadlineExceeded`), and any `ResultData` concatenated *before* cancellation occurred. Other fields are taken from the last message received before cancellation.
*   If the channel closes normally, the `ResultData` field contains a concatenation of all non-empty `ResultData` strings received from *all* messages (including intermediate `RUNNING` ones). All other fields (`TaskID`, `Status`, `Message`, `Error`) are copied directly from the *last* `OutputResult` message received before the channel closed.

**Example Usage:**

```go
    // Assume executor.Execute(ctx, task) was called and returned resultsChan & err
    if err != nil {
        // Handle initiation error
        log.Printf("Error initiating task: %v", err)
        return
    }

    log.Printf("Waiting for task %s to complete...", task.TaskId) // task is the task struct

    // Create a context, perhaps with a timeout
    // collectionCtx, collectionCancel := context.WithTimeout(context.Background(), 10*time.Second)
    // defer collectionCancel()
    collectionCtx := context.Background() // Or use a context passed down

    // Use the utility function to wait for completion or cancellation
    finalResult := readFinalResult(collectionCtx, resultsChan)

    log.Printf("Task %s finished with status: %s", finalResult.TaskID, finalResult.Status)

    if finalResult.Status == task.StatusSucceeded {
        log.Printf("Success Message: %s", finalResult.Message)
        if finalResult.ResultData != "" {
            log.Printf("Output Data:\n%s", finalResult.ResultData)
        }
    } else { // StatusFailed
        log.Printf("Failure Message: %s", finalResult.Message)
        log.Printf("Error Details: %s", finalResult.Error)
        if finalResult.ResultData != "" {
            log.Printf("Output Data (before failure):\n%s", finalResult.ResultData)
        }
    }
```

## Execution Flow

1.  **Task Received**: The system receives a task request, typically as JSON, defining the task type and its specific parameters.
2.  **Parsing**: The JSON is unmarshalled into the corresponding Go task struct (e.g., `BashExecTask`, `FileReadTask`).
3.  **Executor Lookup**: The `Registry` is used to find the `Executor` implementation for the given task type.
4.  **Task Execution**: The task is executed by the appropriate executor, which sends progress and results through a channel.
5.  **Result Processing**: Results are collected from the channel, processed, and returned to the caller.