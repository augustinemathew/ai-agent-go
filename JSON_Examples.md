# Task Executor Examples

This document provides examples of task structures and their corresponding responses for each executor type in the AI Agent Go task execution system. All examples are based on actual execution results.

## Task Structure Overview

All tasks follow this basic structure:

```json
{
  "task_id": "unique-identifier",
  "description": "Human-readable description",
  "type": "TASK_TYPE",
  "status": "",
  "parameters": {
    // Type-specific parameters
  }
}
```

## FileWriteTask

### Request

```json
{
  "task_id": "child-1",
  "description": "First child task",
  "type": "FILE_WRITE",
  "status": "",
  "parameters": {
    "working_directory": "",
    "file_path": "/path/to/child1.txt",
    "content": "Content from child 1"
  }
}
```

### Response

```json
{
  "task_id": "child-1",
  "status": "SUCCEEDED",
  "message": "File writing finished successfully to '/path/to/child1.txt' in 0s."
}
```

### Task After Execution

```json
{
  "task_id": "child-1",
  "description": "First child task",
  "status": "SUCCEEDED",
  "type": "FILE_WRITE",
  "output": {
    "task_id": "child-1",
    "status": "SUCCEEDED", 
    "message": "File writing finished successfully to '/path/to/child1.txt' in 0s."
  },
  "parameters": {
    "working_directory": "",
    "file_path": "/path/to/child1.txt",
    "content": "Content from child 1"
  }
}
```

## FileReadTask

### Request

```json
{
  "task_id": "read-example",
  "description": "Read file contents",
  "type": "FILE_READ",
  "status": "",
  "parameters": {
    "file_path": "/path/to/file.txt",
    "start_line": 0,
    "end_line": 0,
    "working_directory": ""
  }
}
```

### Response

```json
{
  "task_id": "read-example",
  "status": "SUCCEEDED",
  "message": "File reading finished successfully in 0s.",
  "resultData": "Content from the file"
}
```

### Task After Execution

```json
{
  "task_id": "read-example",
  "description": "Read file contents",
  "status": "SUCCEEDED",
  "type": "FILE_READ",
  "output": {
    "task_id": "read-example",
    "status": "SUCCEEDED",
    "message": "File reading finished successfully in 0s.",
    "resultData": "Content from the file"
  },
  "parameters": {
    "file_path": "/path/to/file.txt",
    "start_line": 0,
    "end_line": 0,
    "working_directory": ""
  }
}
```

## PatchFileTask

### Request

```json
{
  "task_id": "patch-example",
  "description": "Patch demo file",
  "type": "PATCH_FILE",
  "status": "",
  "parameters": {
    "file_path": "/path/to/file.txt",
    "patch": "--- a/file.txt\n+++ b/file.txt\n@@ -1,2 +1,2 @@\n-Original content\n+Modified content\n Second line unchanged",
    "working_directory": ""
  }
}
```

### Response

```json
{
  "task_id": "patch-example",
  "status": "SUCCEEDED",
  "message": "Successfully patched file /path/to/file.txt"
}
```

### Task After Execution

```json
{
  "task_id": "patch-example",
  "description": "Patch demo file",
  "status": "SUCCEEDED",
  "type": "PATCH_FILE",
  "output": {
    "task_id": "patch-example",
    "status": "SUCCEEDED",
    "message": "Successfully patched file /path/to/file.txt"
  },
  "parameters": {
    "file_path": "/path/to/file.txt",
    "patch": "--- a/file.txt\n+++ b/file.txt\n@@ -1,2 +1,2 @@\n-Original content\n+Modified content\n Second line unchanged",
    "working_directory": ""
  }
}
```

## BashExecTask

### Request (Simple Command)

```json
{
  "task_id": "bash-simple-example",
  "description": "Simple echo command",
  "type": "BASH_EXEC",
  "status": "",
  "parameters": {
    "command": "echo \"Hello from Bash!\"",
    "working_directory": ""
  }
}
```

### Response

```json
{
  "task_id": "bash-simple-example",
  "status": "SUCCEEDED",
  "message": "Command completed successfully in 5ms. Final CWD: /path/to/workdir.",
  "resultData": "Hello from Bash!\n"
}
```

### Task After Execution

```json
{
  "task_id": "bash-simple-example",
  "description": "Simple echo command",
  "status": "SUCCEEDED",
  "type": "BASH_EXEC",
  "output": {
    "task_id": "bash-simple-example",
    "status": "SUCCEEDED",
    "message": "Command completed successfully in 5ms. Final CWD: /path/to/workdir.",
    "resultData": "Hello from Bash!\n"
  },
  "parameters": {
    "command": "echo \"Hello from Bash!\"",
    "working_directory": ""
  }
}
```

## ListDirectoryTask

### Request

```json
{
  "task_id": "list-dir-example",
  "description": "List directory contents",
  "type": "LIST_DIRECTORY",
  "status": "",
  "parameters": {
    "path": "/path/to/directory",
    "working_directory": ""
  }
}
```

### Response

```json
{
  "task_id": "list-dir-example",
  "status": "SUCCEEDED",
  "message": "Successfully listed directory '/path/to/directory' in 1ms.",
  "resultData": "Listing for /path/to/directory:\n  [FILE] -rw-r--r-- 2025-04-14T21:48:20-07:00        256 file1.txt\n  [FILE] -rw-r--r-- 2025-04-14T21:49:30-07:00        128 file2.txt\n  [DIR ] drwxr-xr-x 2025-04-14T21:50:15-07:00          - subdirectory"
}
```

### Task After Execution

```json
{
  "task_id": "list-dir-example",
  "description": "List directory contents",
  "status": "SUCCEEDED",
  "type": "LIST_DIRECTORY",
  "output": {
    "task_id": "list-dir-example",
    "status": "SUCCEEDED",
    "message": "Successfully listed directory '/path/to/directory' in 1ms.",
    "resultData": "Listing for /path/to/directory:\n  [FILE] -rw-r--r-- 2025-04-14T21:48:20-07:00        256 file1.txt\n  [FILE] -rw-r--r-- 2025-04-14T21:49:30-07:00        128 file2.txt\n  [DIR ] drwxr-xr-x 2025-04-14T21:50:15-07:00          - subdirectory"
  },
  "parameters": {
    "path": "/path/to/directory",
    "working_directory": ""
  }
}
```

## GroupTask

### Request

```json
{
  "task_id": "group-1",
  "description": "Group with two children",
  "type": "GROUP",
  "status": "",
  "children": [
    {
      "task_id": "child-1",
      "description": "First child task",
      "type": "FILE_WRITE",
      "status": "",
      "parameters": {
        "file_path": "/path/to/child1.txt",
        "content": "Content from child 1",
        "working_directory": ""
      }
    },
    {
      "task_id": "child-2",
      "description": "Second child task",
      "type": "FILE_WRITE",
      "status": "",
      "parameters": {
        "file_path": "/path/to/child2.txt",
        "content": "Content from child 2",
        "working_directory": ""
      }
    }
  ]
}
```

### Intermediate Responses (Progress Updates)

```json
{
  "task_id": "group-1",
  "status": "RUNNING",
  "message": "Starting execution of group task with 2 children"
}
```

```json
{
  "task_id": "group-1",
  "status": "RUNNING",
  "message": "Completed child task 1/2 (SUCCEEDED)"
}
```

```json
{
  "task_id": "group-1",
  "status": "RUNNING",
  "message": "Completed child task 2/2 (SUCCEEDED)"
}
```

### Final Response

```json
{
  "task_id": "group-1",
  "status": "SUCCEEDED",
  "message": "Group task completed successfully with 2 child tasks in 0s"
}
```

### Task After Execution

```json
{
  "task_id": "group-1",
  "description": "Group with two children",
  "type": "GROUP",
  "status": "SUCCEEDED",
  "children": [
    {
      "task_id": "child-1",
      "description": "First child task",
      "status": "SUCCEEDED",
      "type": "FILE_WRITE",
      "output": {
        "task_id": "child-1",
        "status": "SUCCEEDED",
        "message": "File writing finished successfully to '/path/to/child1.txt' in 0s."
      },
      "parameters": {
        "working_directory": "",
        "file_path": "/path/to/child1.txt",
        "content": "Content from child 1"
      }
    },
    {
      "task_id": "child-2",
      "description": "Second child task",
      "status": "SUCCEEDED",
      "type": "FILE_WRITE",
      "output": {
        "task_id": "child-2",
        "status": "SUCCEEDED",
        "message": "File writing finished successfully to '/path/to/child2.txt' in 0s."
      },
      "parameters": {
        "working_directory": "",
        "file_path": "/path/to/child2.txt",
        "content": "Content from child 2"
      }
    }
  ],
  "output": {
    "task_id": "group-1",
    "status": "SUCCEEDED",
    "message": "Group task completed successfully with 2 child tasks in 0s"
  }
}
```

## Common Error Responses

### Invalid Task Type

```json
{
  "task_id": "example-task",
  "status": "FAILED",
  "message": "Failed to get executor for task",
  "error": "invalid task type: UNKNOWN_TYPE"
}
```

### File Not Found

```json
{
  "task_id": "read-missing",
  "status": "FAILED",
  "message": "Failed to read file",
  "error": "open /path/to/nonexistent.txt: no such file or directory"
}
```

### Permission Denied

```json
{
  "task_id": "write-protected",
  "status": "FAILED",
  "message": "Failed to write file",
  "error": "open /protected/file.txt: permission denied"
}
```

### Invalid Patch Format

```json
{
  "task_id": "invalid-patch",
  "status": "FAILED",
  "message": "Failed to parse patch",
  "error": "cannot parse patch: invalid unified diff format"
}
```

### Command Execution Failure

```json
{
  "task_id": "failed-command",
  "status": "FAILED",
  "message": "Command failed with exit code 1",
  "error": "exit status 1",
  "resultData": "bash: command-not-found: command not found\n"
}
```

### Context Cancellation

```json
{
  "task_id": "cancelled-task",
  "status": "FAILED",
  "message": "Task execution cancelled",
  "error": "context canceled"
}
```

### Context Timeout

```json
{
  "task_id": "timeout-task",
  "status": "FAILED",
  "message": "Task execution timed out",
  "error": "context deadline exceeded"
}
```

## Status Propagation in Group Tasks

Group tasks provide status updates as child tasks complete, allowing for real-time monitoring of task progress:

1. **Initial Status**: When a group task starts, it sends a RUNNING status with a message indicating the number of children.
2. **Progress Updates**: As each child task completes, the group task sends a RUNNING status update with the child's status.
3. **Final Status**: After all child tasks complete, the group task sends a final status of SUCCEEDED (if all children succeeded) or FAILED (if any child failed).

This status propagation makes it easy to monitor complex nested task hierarchies in real-time. 