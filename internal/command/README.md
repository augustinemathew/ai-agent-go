# Command Execution Package (`internal/command`)

This package defines and executes various commands asynchronously within the AI agent backend. It provides a structured way to represent commands and their results using JSON, facilitating communication between different parts of the system (e.g., a web server handling requests and a background worker executing commands).

## Core Concepts

1.  **Commands**: Represent specific actions to be performed. Each command type has a dedicated struct embedding `BaseCommand`.
2.  **Executors**: Responsible for handling the execution logic for a specific `CommandType`. Each executor implements the `CommandExecutor` interface.
3.  **Registry**: Maps `CommandType` values to their corresponding `CommandExecutor` implementations.
4.  **Results**: The `OutputResult` struct standardizes the format for reporting the outcome of a command execution, including status, messages, errors, and command-specific data. Results are sent over a channel.

## Command Reference

This section details the specific commands supported by the package, including their purpose, input JSON structure, and example output JSON upon success.

All command inputs share the following base fields:

```json
{
  "command_id": "string", // Unique identifier for this command instance
  "description": "string" // Human-readable description (optional but recommended)
}
```

All command outputs (`OutputResult`) share the following general structure. Specific details, especially for `resultData`, vary by command type.

```json
{
  "command_id": "string",      // Matches the command_id of the originating command
  "commandType": "string",     // The type of command that produced this result (e.g., "BASH_EXEC")
  "status": "string",        // Execution status: "RUNNING", "SUCCEEDED", "FAILED"
  "message": "string",       // Human-readable summary/status update
  "error": "string,omitempty", // Error details if status is "FAILED", otherwise omitted
  "resultData": "string,omitempty" // Command-specific output data, omitted if not applicable
}
```

---

### `BASH_EXEC`

Executes a shell command (`BashExecCommand`).

**Input JSON:**

```json
{
  "command_id": "unique-id-1",
  "description": "Run ls in /tmp",
  "command": "ls -l /tmp" // The shell command to execute
}
```

**Output JSON (Success Example):**

This example shows the *final combined* `OutputResult` after using `CombineOutputResults`.
The intermediate `RUNNING` messages' `resultData` (stdout/stderr) is concatenated here.

```json
{
  "command_id": "happy-bash-1",
  "commandType": "BASH_EXEC",
  "status": "SUCCEEDED",
  "message": "Command finished in 5ms. Final CWD: /Users/augustine/ai-agent-backend/ai-agent-v3.",
  "error": "",
  "resultData": "Hello from Bash!\nStarting main script execution...\nInitial directory: /Users/augustine/ai-agent-backend/ai-agent-v3\n---\n\n############################################\n# Script Exiting\n# Exit Status: 0\n# Final Working Directory: /Users/augustine/ai-agent-backend/ai-agent-v3\n############################################\n"
}
```

---

### `FILE_READ`

Reads the contents of a file, optionally from specific line numbers.

**Input JSON:**

```json
{
  "command_id": "read-1",
  "description": "Read file contents",
  "commandType": "FILE_READ",
  "file_path": "/path/to/file.txt",
  "start_line": 2,  // Optional: 1-based line number to start reading from (0 means start from beginning)
  "end_line": 4     // Optional: 1-based line number to read until (0 means read until end)
}
```

**Example Output JSON:**

```json
{
  "command_id": "happy-read-3",
  "commandType": "FILE_READ",
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
  "command_id": "unique-id-3",
  "description": "Write initial data",
  "file_path": "/path/to/output.txt", // Path to the file to write
  "content": "This is the content to write.\nSecond line."
}
```

**Output JSON (Success Example):**

`resultData` is typically empty on success. `status` goes directly to `SUCCEEDED` or `FAILED`.

```json
{
  "command_id": "happy-write-1",
  "commandType": "FILE_WRITE",
  "status": "SUCCEEDED",
  "message": "File writing finished successfully to '/var/folders/99/0xjhznh90fldmj20kkw_s_km0000gn/T/cmd_runner_demo_1744357299302616000.txt' in 0s.",
  "error": "",
  "resultData": ""
}
```

---

### `PATCH_FILE`

Applies a patch (e.g., in unified diff format) to a file (`PatchFileCommand`).

**Note:** This command can also be used to **create a new file** by providing a patch that adds content relative to an empty file (typically indicated with `--- /dev/null` in the patch header).

**Input JSON (Modify Existing File):**

```json
{
  "command_id": "unique-id-4",
  "description": "Apply code changes",
  "file_path": "/path/to/source.go", // Path to the file to patch
  "patch": "--- a/source.go\n+++ b/source.go\n@@ -1,1 +1,2 @@\n package main\n+import \"fmt\"" // Patch content
}
```

**Input JSON (Create New File):**

```json
{
  "command_id": "unique-id-create-5",
  "description": "Create a new file with initial content",
  "file_path": "/path/to/new_file.txt", // Path for the new file
  "patch": "--- /dev/null\n+++ b/new_file.txt\n@@ -0,0 +1,3 @@\n+First line.\n+Second line.\n+Third line.\n" // Patch starting from empty
}
```
*(Note: The patch format itself depends on the implementation, but unified diff is common).*

**Output JSON (Success Example):**

`resultData` is typically empty on success.

```json
{
  "command_id": "happy-patch-1",
  "commandType": "PATCH_FILE",
  "status": "SUCCEEDED",
  "message": "Successfully applied patch to /var/folders/99/0xjhznh90fldmj20kkw_s_km0000gn/T/cmd_runner_demo_1744357299302616000.txt in 0s.",
  "error": "",
  "resultData": ""
}
```

---

### `LIST_DIRECTORY`

Lists the contents of a directory (`ListDirectoryCommand`).

**Input JSON:**

```json
{
  "command_id": "unique-id-5",
  "description": "List project root",
  "path": "/path/to/directory" // Path to the directory to list
}
```

**Output JSON (Success Example):**

Contains a formatted string listing the directory contents in `resultData`. `status` goes directly to `SUCCEEDED` or `FAILED`.

```json
{
  "command_id": "happy-list-1",
  "commandType": "LIST_DIRECTORY",
  "status": "SUCCEEDED",
  "message": "Successfully listed directory '/var/folders/99/0xjhznh90fldmj20kkw_s_km0000gn/T/' in 2ms.",
  "error": "",
  "resultData": "Listing for /var/folders/99/0xjhznh90fldmj20kkw_s_km0000gn/T:\n  [DIR ] ... (many lines omitted for brevity) ...\n  [FILE] -rw-r--r-- 2025-04-11T00:41:39-07:00         46 cmd_runner_demo_1744357299302616000.txt\n  ... (many lines omitted for brevity) ...\n"
}
```
*(Note: The exact format of `resultData` might vary slightly based on the OS, but the structure `[TYPE] Mode Modified Size Name` is consistent. Output is truncated for brevity.)*

---

### `REQUEST_USER_INPUT`

Prompts the user for input (`RequestUserInput`). The mechanism for displaying the prompt and receiving input depends on the executor's implementation.

**Input JSON:**

```json
{
  "command_id": "unique-id-6",
  "description": "Ask for API key",
  "prompt": "Please enter your API Key:" // Message displayed to the user
}
```

**Output JSON (Conceptual Success Examples):**

The format depends on how user input is handled. `resultData` might contain the user's response, or it might be empty if the response is handled separately.

*   **(If response included in `resultData`):**
    ```json
    {
      "command_id": "unique-id-6",
      "commandType": "REQUEST_USER_INPUT",
      "status": "SUCCEEDED",
      "message": "User provided input.",
      "error": "",
      "resultData": "user-provided-api-key" // Example: User input captured here
    }
    ```
*   **(If response handled elsewhere):**
    ```json
    {
      "command_id": "unique-id-6",
      "commandType": "REQUEST_USER_INPUT",
      "status": "SUCCEEDED",
      "message": "User input prompt displayed.",
      "error": "",
      "resultData": ""
    }
    ```

---

## Usage

1.  **Initialization**: Create a `CommandRegistry`. All standard executors are registered automatically by `NewMapRegistry`.
    ```go
    package main

    import (
    	"fmt"
    	"log"
    	"os"
    	"time"

    	"your_project/internal/command" // Adjust import path
    )

    func main() {
    	// Create registry - standard executors are registered automatically.
    	registry := command.NewMapRegistry()

    	// Optional: Override or register custom executors if needed
    	// registry.Register(command.CmdBashExec, myCustomBashExecutor)

    	// ... rest of your application setup
    }

    ```

2.  **Get Executor**: When you receive a command (e.g., parsed from a JSON request), retrieve the appropriate executor from the registry using the command's type.

    ```go
    // Assume 'cmdType' is determined from the incoming request/data
    cmdType := command.CmdBashExec // Example

    executor, err := registry.GetExecutor(cmdType)
    if err != nil {
    	log.Fatalf("Error getting executor for type %s: %v", cmdType, err)
        // Handle error appropriately (e.g., return error response)
    }
    ```

3.  **Execute Command**: Call the `Execute` method on the retrieved executor, passing a context and the specific command struct (e.g., `BashExecCommand`). The `Execute` method requires the command as an `any` type, so you'll typically pass a pointer to your specific command struct.

    ```go
    import "context"

    // Assume 'cmd' is the specific command struct instance (e.g., BashExecCommand)
    // populated with data (e.g., from a JSON request).
    bashCmd := command.BashExecCommand{
        BaseCommand: command.BaseCommand{CommandID: "cmd-123", Description: "List root dir"},
        Command:     "ls -l /",
    }

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // Example timeout
    defer cancel()

    // Pass a pointer to the command struct
    resultsChan, err := executor.Execute(ctx, &bashCmd)
    if err != nil {
    	log.Printf("Error initiating command execution for %s: %v", bashCmd.CommandID, err)
        // Handle initiation error (e.g., invalid command structure)
        return
    }
    ```

4.  **Process Results**: Read results from the returned channel asynchronously. The channel will close when execution is complete (successfully or with failure).

    ```go
    log.Printf("Waiting for results for command %s...", bashCmd.CommandID)
    for result := range resultsChan {
    	// Process each result (e.g., log it, send updates via WebSocket, etc.)
    	log.Printf("Received result: %+v", result)

    	// Check for terminal states
    	if result.Status == command.StatusSucceeded || result.Status == command.StatusFailed {
    		log.Printf("Command %s finished with status: %s", result.CommandID, result.Status)
    		if result.Error != "" {
    			log.Printf("Error detail: %s", result.Error)
    		}
    		break // Or wait if intermediate results are possible after failure (unlikely for most commands)
    	}
    }
    log.Printf("Result channel closed for command %s.", bashCmd.CommandID)
    ```

## Result Collection Utility (`CombineOutputResults`)

For convenience, especially with streaming commands (`BASH_EXEC`, `FILE_READ`), the package provides a utility function `CombineOutputResults` to simplify consuming the results channel.

This function reads all `OutputResult` messages from the channel until it either closes or the provided `context.Context` is cancelled. It then returns a *single* `OutputResult` summarizing the execution:

*   If the context is cancelled before the channel closes, it returns an `OutputResult` with `StatusFailed`, an error message indicating cancellation (`context.Canceled` or `context.DeadlineExceeded`), and any `ResultData` concatenated *before* cancellation occurred. Other fields are taken from the last message received before cancellation.
*   If the channel closes normally, the `ResultData` field contains a concatenation of all non-empty `ResultData` strings received from *all* messages (including intermediate `RUNNING` ones). All other fields (`CommandID`, `CommandType`, `Status`, `Message`, `Error`) are copied directly from the *last* `OutputResult` message received before the channel closed.

**Example Usage:**

```go
    // Assume executor.Execute(ctx, cmd) was called and returned resultsChan & err
    if err != nil {
        // Handle initiation error
        log.Printf("Error initiating command: %v", err)
        return
    }

    log.Printf("Waiting for command %s to complete...", cmd.CommandID) // cmd is the command struct

    // Create a context, perhaps with a timeout
    // collectionCtx, collectionCancel := context.WithTimeout(context.Background(), 10*time.Second)
    // defer collectionCancel()
    collectionCtx := context.Background() // Or use a context passed down

    // Use the utility function to wait for completion or cancellation
    finalResult := command.CombineOutputResults(collectionCtx, resultsChan)

    log.Printf("Command %s finished with status: %s", finalResult.CommandID, finalResult.Status)

    if finalResult.Status == command.StatusSucceeded {
        log.Printf("Success Message: %s", finalResult.Message)
        if finalResult.ResultData != "" {
            log.Printf("Concatenated Output Data:\n%s", finalResult.ResultData)
        }
    } else { // StatusFailed
        log.Printf("Failure Message: %s", finalResult.Message)
        log.Printf("Error Details: %s", finalResult.Error)
        if finalResult.ResultData != "" {
            log.Printf("Concatenated Output Data (before failure):\n%s", finalResult.ResultData)
        }
    }
```

## Execution Flow

1.  **Command Received**: The system receives a command request, typically as JSON, defining the `commandType` and its specific parameters.
2.  **Parsing**: The JSON is unmarshalled into the corresponding Go command struct (e.g., `BashExecCommand`, `FileReadCommand`).
3.  **Executor Lookup**: The `CommandRegistry` is used to find the `