# Task Execution Package (`internal/task`)

This package defines and executes various tasks asynchronously within the AI agent backend. It provides a structured way to represent tasks and their results using JSON, facilitating communication between different parts of the system (e.g., a web server handling requests and a background worker executing tasks).

## Recent Updates

- **Enhanced GroupExecutor Child Task Output Forwarding**: GroupExecutor now forwards output messages from child tasks with their original task IDs preserved, providing improved visibility and traceability of individual task execution.
- **Improved GroupExecutor Status Propagation**: Enhanced status updates from child tasks to parent group tasks, providing real-time visibility into execution progress.
- **Race Condition Prevention**: Implemented file locking mechanism in the PatchFileExecutor to safely handle concurrent operations on the same files.
- **Pointer Type Consistency**: Fixed issues with task type handling to ensure all executors consistently work with pointer types rather than value types.
- **Logging Improvements**: Removed unnecessary debugging logs from all executors for cleaner code and better performance.
- **Test Consistency**: Aligned test assertions with current implementation for more reliable testing.
- **Code Cleanup**: Streamlined executor implementations for better readability and maintainability.
- **Unified Task Structure**: Consolidated different task types into a single `Task` struct with a dynamic `Parameters` field.
- **Factory Functions**: Added factory functions (`NewFileReadTask`, `NewFileWriteTask`, etc.) for more ergonomic task creation.
- **JSON Marshaling/Unmarshaling**: Implemented custom JSON serialization and deserialization for tasks with support for type-based parameter handling.

## Architectural Improvements

### Child Task Output Forwarding in GroupExecutor

The GroupExecutor now provides enhanced visibility of child task execution:

1. **Preserved Task IDs**: Child task output messages retain their original task IDs, enabling clients to track individual tasks.
2. **Dual Message Mode**: Both the original child task message and a summarized group task message are forwarded.
3. **Real-time Content Updates**: For tasks like FileRead and BashExec, the content from each child task is included in forwarded messages.
4. **Complete Execution Tracing**: Every stage of task execution is visible in the result stream.

This improvement makes it easier to:
- Track which specific child task is executing
- See intermediate results from long-running tasks
- Monitor system behavior more effectively
- Maintain proper correlation between task outputs and their source tasks

### Simplified Executor Design

Each executor now follows a more consistent pattern:
1. **Execute Entry Point**: Validates command type and creates a channel for results
2. **Execution Logic**: Runs in a separate goroutine for non-blocking operation
3. **Clean Error Handling**: Standardized error patterns for consistent reporting

### Error Handling Improvements

- **Context-Aware Execution**: All operations properly respect context cancellation signals
- **Graceful Resource Cleanup**: Ensures resources are properly released on cancellation 
- **Consistent Error Messages**: Standardized error reporting format across executor types
- **Status Transitions**: Clear state management from PENDING → RUNNING → SUCCEEDED/FAILED

### Concurrent Operation Safety

- **File Locking Mechanism**: PatchFileExecutor now uses filesystem locks to prevent race conditions during concurrent patches to the same file
- **Atomic Updates**: Write operations are performed in an atomic way to ensure data integrity
- **Enhanced Group Executor**: Properly handles task pointers throughout child task processing, ensuring consistent behavior
- **Status Propagation**: Improved mechanism for child tasks to report their status changes to parent tasks

### Performance Optimization

- **Reduced Logging Overhead**: Removed debug logging to improve performance
- **Efficient Channel Management**: Optimized channel usage to prevent leaks
- **Simplified Code Paths**: Straightforward execution flow improves maintainability

### Unified Task Structure

- **Single Task Type**: All task types now use a common `Task` struct with a dynamic `Parameters` field
- **Type-Safe Parameters**: Each task type has its own parameter struct that is type-asserted at runtime
- **Factory Functions**: New helper functions (`NewFileReadTask`, `NewFileWriteTask`, etc.) make task creation more intuitive
- **JSON Serialization**: Custom JSON marshaling/unmarshaling based on task type for easy serialization and deserialization

## Task Mutation During Execution

Executors **modify the Task objects** that are passed to them. This is an intentional design pattern that allows tasks to maintain their state and results. It's important to be aware of this behavior when working with tasks.

### Example: BashExec Task Before and After Execution

**Before Execution:**
```json
{
  "TaskId": "bash-example",
  "Description": "Simple echo command",
  "Type": "",
  "Status": "",
  "Children": null,
  "Output": {
    "task_id": "",
    "status": "",
    "message": "",
    "error": "",
    "resultData": ""
  },
  "Parameters": {
    "Command": "echo \"Hello World\"",
    "WorkingDirectory": ""
  }
}
```

**After Execution:**
```json
{
  "TaskId": "bash-example",
  "Description": "Simple echo command",
  "Type": "",
  "Status": "SUCCEEDED",
  "Children": null,
  "Output": {
    "task_id": "bash-example",
    "status": "SUCCEEDED",
    "message": "Command completed successfully in 5ms. Final CWD: /path/to/workdir.",
    "error": "",
    "resultData": "Hello World\n"
  },
  "Parameters": {
    "Command": "echo \"Hello World\"",
    "WorkingDirectory": ""
  }
}
```

### Example: FileRead Task Before and After Execution

**Before Execution:**
```json
{
  "TaskId": "read-example",
  "Description": "Read a file",
  "Type": "",
  "Status": "",
  "Children": null,
  "Output": {
    "task_id": "",
    "status": "",
    "message": "",
    "error": "",
    "resultData": ""
  },
  "Parameters": {
    "FilePath": "/path/to/file.txt",
    "StartLine": 0,
    "EndLine": 0,
    "WorkingDirectory": ""
  }
}
```

**After Execution:**
```json
{
  "TaskId": "read-example",
  "Description": "Read a file",
  "Type": "",
  "Status": "SUCCEEDED",
  "Children": null,
  "Output": {
    "task_id": "read-example",
    "status": "SUCCEEDED",
    "message": "File reading finished successfully in 0s.",
    "error": "",
    "resultData": "File content goes here\nSecond line\n"
  },
  "Parameters": {
    "FilePath": "/path/to/file.txt",
    "StartLine": 0,
    "EndLine": 0,
    "WorkingDirectory": ""
  }
}
```

### Key Changes During Execution

1. **Status Field**: Changes from empty (PENDING) to RUNNING (not shown) and finally to SUCCEEDED or FAILED
2. **Output Field**: Gets populated with:
   - `task_id`: Set to match the task's TaskId
   - `status`: Set to match the task's Status
   - `message`: Human-readable message about the execution
   - `error`: If the task failed, contains error details
   - `resultData`: Task-specific output data

### Important Design Note

Since executors mutate the input task object, if you need to preserve the original task state, you should make a copy before passing it to an executor:

```go
// Create a copy of the task
taskCopy := originalTask
resultsChannel, err := executor.Execute(ctx, &taskCopy)
// originalTask is unchanged, taskCopy is mutated
```

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

**Complete Task Example:**

```json
{
  "BaseTask": {
    "TaskId": "happy-bash-2",
    "Description": "Run multiline script",
    "Type": "",
    "Status": "",
    "Children": null,
    "Output": {
      "task_id": "",
      "status": "",
      "message": "",
      "error": "",
      "resultData": ""
    }
  },
  "Parameters": {
    "Command": "echo \"Starting multiline script...\"\necho \"Current directory: $(pwd)\"\nls -la\necho \"Environment variables:\"\nenv | grep PATH\necho \"Script complete!\"",
    "WorkingDirectory": ""
  }
}
```

**After Execution:**

```json
{
  "BaseTask": {
    "TaskId": "happy-bash-2",
    "Description": "Run multiline script",
    "Type": "",
    "Status": "SUCCEEDED",
    "Children": null,
    "Output": {
      "task_id": "happy-bash-2",
      "status": "SUCCEEDED",
      "message": "Command completed successfully in 10ms. Final CWD: /Users/augustine/ai-agent-4/ai-agent-go.",
      "error": "",
      "resultData": "Starting multiline script...\nCurrent directory: /Users/augustine/ai-agent-4/ai-agent-go\ntotal 24\ndrwxr-xr-x   9 augustine  staff   288 Apr 14 22:43 .\ndrwxr-xr-x   3 augustine  staff    96 Apr 14 17:50 ..\ndrwxr-xr-x@  3 augustine  staff    96 Apr 14 18:25 .cursor\ndrwxr-xr-x  12 augustine  staff   384 Apr 14 23:09 .git\n-rw-r--r--   1 augustine  staff   414 Apr 14 17:50 .gitignore\ndrwxr-xr-x   3 augustine  staff    96 Apr 14 17:50 cmd\n-rw-r--r--@  1 augustine  staff   299 Apr 14 18:00 go.mod\n-rw-r--r--   1 augustine  staff  1658 Apr 14 17:50 go.sum\ndrwxr-xr-x   3 augustine  staff    96 Apr 14 17:55 internal\nEnvironment variables:\nPATH=/opt/homebrew/Cellar/go/1.24.1/libexec/bin:/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/System/Cryptexes/App/usr/bin:/usr/bin:/bin:/usr/sbin:/sbin:/var/run/com.apple.security.cryptexd/codex.system/bootstrap/usr/local/bin:/var/run/com.apple.security.cryptexd/codex.system/bootstrap/usr/bin:/var/run/com.apple.security.cryptexd/codex.system/bootstrap/usr/appleinternal/bin:/opt/homebrew/Caskroom/miniconda/base/bin:/opt/homebrew/Caskroom/miniconda/base/condabin\nINFOPATH=/opt/homebrew/share/info:/opt/homebrew/share/info:\nScript complete!\nStarting main script execution...\nInitial directory: /Users/augustine/ai-agent-4/ai-agent-go\n---\n\n############################################\n# Script Exiting\n# Exit Status: 0\n# Final Working Directory: /Users/augustine/ai-agent-4/ai-agent-go\n############################################\n"
    }
  },
  "Parameters": {
    "Command": "echo \"Starting multiline script...\"\necho \"Current directory: $(pwd)\"\nls -la\necho \"Environment variables:\"\nenv | grep PATH\necho \"Script complete!\"",
    "WorkingDirectory": ""
  }
}
```

**Result JSON Only:**

```json
{
  "task_id": "happy-bash-2",
  "status": "SUCCEEDED",
  "message": "Command completed successfully in 10ms. Final CWD: /Users/augustine/ai-agent-4/ai-agent-go.",
  "resultData": "Starting multiline script...\nCurrent directory: /Users/augustine/ai-agent-4/ai-agent-go\ntotal 24\ndrwxr-xr-x   9 augustine  staff   288 Apr 14 22:43 .\ndrwxr-xr-x   3 augustine  staff    96 Apr 14 17:50 ..\ndrwxr-xr-x@  3 augustine  staff    96 Apr 14 18:25 .cursor\ndrwxr-xr-x  12 augustine  staff   384 Apr 14 23:09 .git\n-rw-r--r--   1 augustine  staff   414 Apr 14 17:50 .gitignore\ndrwxr-xr-x   3 augustine  staff    96 Apr 14 17:50 cmd\n-rw-r--r--@  1 augustine  staff   299 Apr 14 18:00 go.mod\n-rw-r--r--   1 augustine  staff  1658 Apr 14 17:50 go.sum\ndrwxr-xr-x   3 augustine  staff    96 Apr 14 17:55 internal\nEnvironment variables:\nPATH=/opt/homebrew/Cellar/go/1.24.1/libexec/bin:/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/System/Cryptexes/App/usr/bin:/usr/bin:/bin:/usr/sbin:/sbin:/var/run/com.apple.security.cryptexd/codex.system/bootstrap/usr/local/bin:/var/run/com.apple.security.cryptexd/codex.system/bootstrap/usr/bin:/var/run/com.apple.security.cryptexd/codex.system/bootstrap/usr/appleinternal/bin:/opt/homebrew/Caskroom/miniconda/base/bin:/opt/homebrew/Caskroom/miniconda/base/condabin\nINFOPATH=/opt/homebrew/share/info:/opt/homebrew/share/info:\nScript complete!\nStarting main script execution...\nInitial directory: /Users/augustine/ai-agent-4/ai-agent-go\n---\n\n############################################\n# Script Exiting\n# Exit Status: 0\n# Final Working Directory: /Users/augustine/ai-agent-4/ai-agent-go\n############################################\n"
}
```

---

### `FILE_READ`

Reads the contents of a file, optionally from specific line numbers.

**Complete Task Example:**

```json
{
  "BaseTask": {
    "TaskId": "read-1",
    "Description": "Read file contents",
    "Type": "",
    "Status": "",
    "Children": null,
    "Output": {
      "task_id": "",
      "status": "",
      "message": "",
      "error": "",
      "resultData": ""
    }
  },
  "Parameters": {
    "FilePath": "/path/to/file.txt",
    "StartLine": 2,
    "EndLine": 4,
    "WorkingDirectory": ""
  }
}
```

**After Execution:**

```json
{
  "BaseTask": {
    "TaskId": "read-1",
    "Description": "Read file contents",
    "Type": "",
    "Status": "SUCCEEDED",
    "Children": null,
    "Output": {
      "task_id": "read-1",
      "status": "SUCCEEDED",
      "message": "File reading finished successfully in 0s.",
      "resultData": "This is line 2\nThis is line 3\nThis is line 4\n"
    }
  },
  "Parameters": {
    "FilePath": "/path/to/file.txt",
    "StartLine": 2,
    "EndLine": 4,
    "WorkingDirectory": ""
  }
}
```

**Result JSON Only:**

```json
{
  "task_id": "happy-read-3",
  "status": "SUCCEEDED",
  "message": "File reading finished successfully in 0s.",
  "resultData": "This is a test file.\n"
}
```

---

### `FILE_WRITE`

Writes content to a file, overwriting if it exists (`FileWriteTask`).

**Complete Task Example:**

```json
{
  "BaseTask": {
    "TaskId": "happy-write-1",
    "Description": "Write initial data",
    "Type": "",
    "Status": "",
    "Children": null,
    "Output": {
      "task_id": "",
      "status": "",
      "message": "",
      "error": "",
      "resultData": ""
    }
  },
  "Parameters": {
    "FilePath": "/path/to/output.txt",
    "Content": "This is the content to write.\nSecond line.",
    "WorkingDirectory": ""
  }
}
```

**After Execution:**

```json
{
  "BaseTask": {
    "TaskId": "happy-write-1",
    "Description": "Write initial data",
    "Type": "",
    "Status": "SUCCEEDED",
    "Children": null,
    "Output": {
      "task_id": "happy-write-1",
      "status": "SUCCEEDED",
      "message": "File writing finished successfully to '/path/to/output.txt' in 0s.",
      "error": "",
      "resultData": ""
    }
  },
  "Parameters": {
    "FilePath": "/path/to/output.txt",
    "Content": "This is the content to write.\nSecond line.",
    "WorkingDirectory": ""
  }
}
```

**Result JSON Only:**

```json
{
  "task_id": "happy-write-1",
  "status": "SUCCEEDED",
  "message": "File writing finished successfully to '/var/folders/99/0xjhznh90fldmj20kkw_s_km0000gn/T/cmd_runner_demo_1744703180101541000.txt' in 0s."
}
```

**Notes:**
- The executor will create any necessary parent directories automatically
- If a file already exists at the specified path, it will be overwritten
- Empty content is allowed and will create an empty file

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

5. **Status Propagation**:
   - Sends regular status updates as child tasks progress
   - Reports status changes from child tasks to parent group tasks
   - Provides detailed progress messages with task counts and completion percentages
   - Maintains status consistency between tasks and their output results
   - Enables real-time monitoring of complex task hierarchies

6. **Result Handling**:
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

## Task Creation

Tasks can be created using the provided factory functions:

```go
// Create a file read task
readTask := task.NewFileReadTask("read-1", "Read a file", task.FileReadParameters{
    FilePath: "/path/to/file.txt",
})

// Create a file write task
writeTask := task.NewFileWriteTask("write-1", "Write to file", task.FileWriteParameters{
    FilePath: "/path/to/output.txt",
    Content:  "Hello, World!",
})

// Create a group task with nested tasks
groupTask := task.NewGroupTask("group-1", "Group of tasks", []*task.Task{
    readTask,
    writeTask,
})
```

## JSON Serialization and Deserialization

The `Task` struct provides methods for easy serialization and deserialization:

```go
// Convert task to JSON
jsonStr, err := task.ToJSON()

// Convert task to pretty-printed JSON
prettyJson, err := task.ToPrettyJSON()

// Parse JSON into a Task
parsedTask, err := task.FromJSON(jsonStr)
```

The JSON marshaling and unmarshaling automatically handles the dynamic `Parameters` field based on the task's type.

## Execution Flow

1.  **Task Received**: The system receives a task request, typically as JSON, defining the task type and its specific parameters.
2.  **Parsing**: The JSON is unmarshalled into the corresponding Go task struct (e.g., `BashExecTask`, `FileReadTask`).
3.  **Executor Lookup**: The `Registry` is used to find the `Executor` implementation for the given task type.
4.  **Task Execution**: The task is executed by the appropriate executor, which sends progress and results through a channel.
5.  **Result Processing**: Results are collected from the channel, processed, and returned to the caller.