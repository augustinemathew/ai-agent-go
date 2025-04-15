# AI Agent Go

This repository contains the Go implementation of a task execution system for AI Agents. The system is designed to execute various types of tasks asynchronously, with powerful composition capabilities through the GroupExecutor.

## Features

- **Task-based Execution System**: Structured approach to defining and running various types of tasks
- **Executor Pattern**: Clean separation of task definitions and execution logic
- **Streaming Results**: Real-time updates for long-running tasks
- **Context-Aware Execution**: All operations respect context cancellation for proper resource cleanup
- **Concurrent Operation Safety**: File locking mechanisms prevent race conditions during concurrent operations
- **Group Task Composition**: Powerful nested task grouping with status propagation
- **Extensible Architecture**: Easy to add new task types and executors

## Recent Improvements

### GroupExecutor Enhancements

- **Status Propagation**: Improved real-time visibility into task execution progress
- **Pointer Type Consistency**: Fixed issues with task type handling to ensure all executors work with pointer types
- **Nested Task Support**: Enhanced support for complex nested task hierarchies

### PatchFileExecutor Improvements

- **Race Condition Prevention**: Implemented file locking mechanism for safe concurrent operations
- **Atomic Updates**: File modifications are now performed atomically to ensure data integrity

### General Improvements

- **Code Cleanup**: Streamlined implementations for better readability and maintainability
- **Test Coverage**: Enhanced test suite with improved assertions and edge case handling
- **Performance Optimization**: Reduced overhead in critical execution paths

## Task Types

The system supports the following task types:

- **FILE_READ**: Read contents from a file, optionally with line number range specification
- **FILE_WRITE**: Create or overwrite a file with specified content
- **PATCH_FILE**: Apply patches to existing files or create new ones using unified diff format
- **BASH_EXEC**: Execute shell commands with support for both simple and multiline scripts
- **LIST_DIRECTORY**: List contents of a directory with detailed file information
- **REQUEST_USER_INPUT**: Prompt for and collect user input
- **GROUP**: Compose and execute multiple tasks as a single unit with automatic status propagation

## Documentation

- **Usage Guide**: Detailed documentation in [internal/task/README.md](internal/task/README.md)
- **API Examples**: Complete request/response examples in [Examples.md](Examples.md)
- **Demo Applications**: Working examples in the `demos` directory

The [Examples.md](Examples.md) file contains complete JSON examples showing:
- How to structure requests for each task type
- What responses to expect from executors
- How tasks are mutated during execution
- Status propagation in hierarchical task structures

## Getting Started

### Quick Start

```go
// Initialize registry
registry := task.NewMapRegistry()

// Create a task
fileWriteTask := task.NewFileWriteTask("write-example", "Write to test file", task.FileWriteParameters{
    FilePath: "/path/to/test.txt",
    Content:  "Hello, World!",
})

// Get the appropriate executor
executor, err := registry.GetExecutor(task.TaskFileWrite)
if err != nil {
    log.Fatalf("Failed to get executor: %v", err)
}

// Execute the task
ctx := context.Background()
resultsChan, err := executor.Execute(ctx, fileWriteTask)
if err != nil {
    log.Fatalf("Failed to execute task: %v", err)
}

// Process results
finalResult := task.CombineOutputResults(ctx, resultsChan)
fmt.Printf("Task completed with status: %s\n", finalResult.Status)
```

### Group Task Example

```go
// Create a group task with two child tasks
groupTask := task.NewGroupTask(
    "group-example",
    "Group with multiple children",
    []*task.Task{
        task.NewFileWriteTask("child-1", "First child task", task.FileWriteParameters{
            FilePath: "/path/to/file1.txt",
            Content:  "Content for file 1",
        }),
        task.NewFileWriteTask("child-2", "Second child task", task.FileWriteParameters{
            FilePath: "/path/to/file2.txt",
            Content:  "Content for file 2",
        }),
    },
)

// Execute the group task
executor, _ := registry.GetExecutor(task.TaskGroup)
resultsChan, _ := executor.Execute(ctx, groupTask)

// Process results
for result := range resultsChan {
    // Each result shows progress through the group task
    fmt.Printf("Status: %s, Message: %s\n", result.Status, result.Message)
}
```

## Status Propagation

The GroupExecutor provides real-time status updates as child tasks complete:

1. **Initial Status**: When a group task starts, it sends a `RUNNING` status with a message indicating the number of children.
2. **Progress Updates**: As each child task completes, the group task sends a `RUNNING` status update with progress information.
3. **Final Status**: After all child tasks complete, the group task sends a final status of `SUCCEEDED` (if all children succeeded) or `FAILED` (if any child failed).

## License

This project is licensed under the terms of the MIT license. 