package main

import (
	"ai-agent-v3/internal/task"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
	// No strings import needed for JSON output
)

func main() {
	fmt.Println("Starting Command Runner Happy Path Demo...")

	// 1. Create the registry. Executors are now registered automatically.
	registry := task.NewMapRegistry()
	// No manual registration needed anymore:
	// registry.Register(command.CmdBashExec, command.NewBashExecExecutor())
	// registry.Register(command.CmdFileRead, command.NewFileReadExecutor())
	// ... etc ...

	fmt.Println("Registry initialized with standard executors.")

	// --- Command Execution Sequence ---

	// Prepare temp file path and content
	tempDir := os.TempDir()
	tempFileName := fmt.Sprintf("cmd_runner_demo_%d.txt", time.Now().UnixNano())
	tempFilePath := filepath.Join(tempDir, tempFileName)
	fileContent := "Hello from FileWrite!\nThis is a test file."
	fmt.Printf("Using temporary file: %s\n", tempFilePath)

	// Ensure cleanup
	defer func() {
		fmt.Printf("Cleaning up temporary file: %s\n", tempFilePath)
		os.Remove(tempFilePath)

		// Also clean up files created by the group task
		os.Remove(filepath.Join(tempDir, "group-file1.txt"))
		os.Remove(filepath.Join(tempDir, "group-file2.txt"))
		// Cleanup relative path examples
		os.Remove(filepath.Join(tempDir, "relative-file1.txt"))
		os.Remove(filepath.Join(tempDir, "relative-file2.txt"))
		fmt.Println("Cleanup complete.")
	}()

	// Define the commands
	commandsToRun := []interface{}{
		task.NewFileWriteTask("happy-write-1", "Write demo file", task.FileWriteParameters{
			FilePath: tempFilePath,
			Content:  fileContent,
		}),
		task.NewListDirectoryTask("happy-list-1", "List temp directory", task.ListDirectoryParameters{
			Path: tempDir, // List the directory containing the temp file
		}),
		task.NewFileReadTask("happy-read-1", "Read demo file", task.FileReadParameters{
			FilePath: tempFilePath,
		}),
		task.NewBashExecTask("happy-bash-1", "Simple echo", task.BashExecParameters{
			Command: "echo \"Hello from Bash!\"",
		}),
		// Add a multiline Bash script example
		task.NewBashExecTask("happy-bash-2", "Multiline Bash script", task.BashExecParameters{
			Command: "echo \"Starting multiline script...\"\necho \"Current directory: $(pwd)\"\nls -la\necho \"Environment variables:\"\nenv | grep PATH\necho \"Script complete!\"",
		}),
		// Add a PATCH_FILE command
		task.NewPatchFileTask("happy-patch-1", "Patch demo file", task.PatchFileParameters{
			FilePath: tempFilePath,
			// Patch to change "Hello" to "Greetings"
			Patch: fmt.Sprintf("--- a/%s\n+++ b/%s\n@@ -1,2 +1,2 @@\n-Hello from FileWrite!\n+Greetings from PatchFile!\n This is a test file.\n", tempFileName, tempFileName),
		}),
		// Read the file again to see the patch result
		task.NewFileReadTask("happy-read-2", "Read patched demo file", task.FileReadParameters{
			FilePath: tempFilePath,
		}),
		// Demonstrate line number functionality
		task.NewFileReadTask("happy-read-3", "Read specific lines from file", task.FileReadParameters{
			FilePath:  tempFilePath,
			StartLine: 2, // Start from second line
			EndLine:   3, // Read until third line
		}),
		// EXAMPLE 1: Using relative paths with WorkingDirectory
		task.NewFileWriteTask("relative-write-1", "Write file using relative path", task.FileWriteParameters{
			BaseParameters: task.BaseParameters{
				WorkingDirectory: tempDir, // Set the working directory
			},
			FilePath:  "relative-file1.txt", // Relative to WorkingDirectory
			Content:   "This file was created using a relative path with WorkingDirectory parameter!",
			Overwrite: false,
		}),
		// Read the file created with relative path
		task.NewFileReadTask("relative-read-1", "Read file using relative path", task.FileReadParameters{
			BaseParameters: task.BaseParameters{
				WorkingDirectory: tempDir, // Same working directory
			},
			FilePath: "relative-file1.txt", // Relative to WorkingDirectory
		}),
		// EXAMPLE 2: BashExec with WorkingDirectory
		task.NewBashExecTask("relative-bash-1", "Run bash in specified working directory", task.BashExecParameters{
			BaseParameters: task.BaseParameters{
				WorkingDirectory: tempDir, // Set the working directory
			},
			Command: "pwd && touch relative-file2.txt && ls -la relative-file2.txt",
		}),
		// EXAMPLE 3: Patch a file using relative path
		task.NewFileWriteTask("relative-base-file", "Create file to be patched", task.FileWriteParameters{
			BaseParameters: task.BaseParameters{
				WorkingDirectory: tempDir,
			},
			FilePath: tempFileName,
			Content:  "Original content in relative file.\nThis line will stay the same.",
		}),
		task.NewPatchFileTask("relative-patch-1", "Patch file using relative path", task.PatchFileParameters{
			BaseParameters: task.BaseParameters{
				WorkingDirectory: tempDir,
			},
			FilePath: tempFileName, // Relative to WorkingDirectory
			Patch:    "--- a/file.txt\n+++ b/file.txt\n@@ -1,2 +1,2 @@\n-Original content in relative file.\n+Modified content in relative file.\n This line will stay the same.",
		}),
		// Read the patched file
		task.NewFileReadTask("relative-read-patched", "Read patched file with relative path", task.FileReadParameters{
			BaseParameters: task.BaseParameters{
				WorkingDirectory: tempDir,
			},
			FilePath: tempFileName,
		}),
		// Add a GROUP task example
		task.NewGroupTask("happy-group-1", "Group task example with nested tasks", []*task.Task{
			task.NewFileWriteTask("group-child-1", "Create a new file in the group", task.FileWriteParameters{
				FilePath: filepath.Join(tempDir, "group-file1.txt"),
				Content:  "This file was created by a child task in a group.",
			}),
			task.NewGroupTask("group-nested", "Nested group task", []*task.Task{
				task.NewFileWriteTask("nested-child-1", "Create another file in nested group", task.FileWriteParameters{
					FilePath: filepath.Join(tempDir, "group-file2.txt"),
					Content:  "This file was created by a nested group child task.",
				}),
				task.NewFileReadTask("nested-child-2", "Read the file we just created", task.FileReadParameters{
					FilePath: filepath.Join(tempDir, "group-file2.txt"),
				}),
			}),
			task.NewBashExecTask("group-child-2", "Run a bash command in the group", task.BashExecParameters{
				Command: "echo \"This is executed as part of a group task.\"",
			}),
		}),
	}

	// Execute commands sequentially
	for i, cmdGeneric := range commandsToRun {
		fmt.Printf("\n--- Executing Command %d ---\n", i+1)

		var cmdType task.TaskType
		var cmdID string

		// Use type assertion to get CommandID and determine CommandType
		cmd, ok := cmdGeneric.(*task.Task)
		if !ok {
			log.Printf("Unknown command type in sequence: %T", cmdGeneric)
			continue
		}

		cmdType = cmd.Type
		cmdID = cmd.TaskId

		// Print the task before execution
		printTaskAsJSON("BEFORE EXECUTION", cmdGeneric)

		// Display information about the task based on its type
		switch cmdType {
		case task.TaskListDirectory:
			params, ok := cmd.Parameters.(task.ListDirectoryParameters)
			if !ok {
				log.Printf("ERROR: Failed to assert parameters for %s", cmdID)
				continue
			}
			if params.WorkingDirectory != "" {
				fmt.Printf("Type: %s, ID: %s, Desc: %s, WorkingDir: %s, Path: %s\n",
					cmdType, cmdID, cmd.Description, params.WorkingDirectory, params.Path)
			} else {
				fmt.Printf("Type: %s, ID: %s, Desc: %s, Path: %s\n",
					cmdType, cmdID, cmd.Description, params.Path)
			}
		case task.TaskFileRead:
			params, ok := cmd.Parameters.(task.FileReadParameters)
			if !ok {
				log.Printf("ERROR: Failed to assert parameters for %s", cmdID)
				continue
			}
			if params.WorkingDirectory != "" {
				fmt.Printf("Type: %s, ID: %s, Desc: %s, WorkingDir: %s, Path: %s\n",
					cmdType, cmdID, cmd.Description, params.WorkingDirectory, params.FilePath)
			} else {
				fmt.Printf("Type: %s, ID: %s, Desc: %s, Path: %s\n",
					cmdType, cmdID, cmd.Description, params.FilePath)
			}
		case task.TaskBashExec:
			params, ok := cmd.Parameters.(task.BashExecParameters)
			if !ok {
				log.Printf("ERROR: Failed to assert parameters for %s", cmdID)
				continue
			}
			if params.WorkingDirectory != "" {
				fmt.Printf("Type: %s, ID: %s, Desc: %s, WorkingDir: %s, Command: %s\n",
					cmdType, cmdID, cmd.Description, params.WorkingDirectory, params.Command)
			} else {
				fmt.Printf("Type: %s, ID: %s, Desc: %s, Command: %s\n",
					cmdType, cmdID, cmd.Description, params.Command)
			}
		case task.TaskPatchFile:
			params, ok := cmd.Parameters.(task.PatchFileParameters)
			if !ok {
				log.Printf("ERROR: Failed to assert parameters for %s", cmdID)
				continue
			}
			if params.WorkingDirectory != "" {
				fmt.Printf("Type: %s, ID: %s, Desc: %s, WorkingDir: %s, Path: %s\n",
					cmdType, cmdID, cmd.Description, params.WorkingDirectory, params.FilePath)
			} else {
				fmt.Printf("Type: %s, ID: %s, Desc: %s, Path: %s\n",
					cmdType, cmdID, cmd.Description, params.FilePath)
			}
		case task.TaskFileWrite:
			params, ok := cmd.Parameters.(task.FileWriteParameters)
			if !ok {
				log.Printf("ERROR: Failed to assert parameters for %s", cmdID)
				continue
			}
			if params.WorkingDirectory != "" {
				fmt.Printf("Type: %s, ID: %s, Desc: %s, WorkingDir: %s, Path: %s\n",
					cmdType, cmdID, cmd.Description, params.WorkingDirectory, params.FilePath)
			} else {
				fmt.Printf("Type: %s, ID: %s, Desc: %s, Path: %s\n",
					cmdType, cmdID, cmd.Description, params.FilePath)
			}
		case task.TaskGroup:
			fmt.Printf("Type: %s, ID: %s, Desc: %s, Children: %d\n",
				cmdType, cmdID, cmd.Description, len(cmd.Children))
		default:
			fmt.Printf("Type: %s, ID: %s, Desc: %s\n",
				cmdType, cmdID, cmd.Description)
		}

		executor, err := registry.GetExecutor(cmdType)
		if err != nil {
			log.Printf("ERROR: Failed to get executor for %s (%s): %v", cmdType, cmdID, err)
			continue
		}

		// Execute
		// Use a background context for now, could be replaced with a request-scoped context
		execCtx := context.Background()
		resultsChan, err := executor.Execute(execCtx, cmd)
		if err != nil {
			log.Printf("ERROR: Failed to start execution for %s (%s): %v", cmdType, cmdID, err)
			continue
		}

		// Use the utility to collect the final result
		fmt.Printf("Collecting results for %s...\n", cmdID)
		finalResult := task.CombineOutputResults(execCtx, resultsChan)

		// Update the task status based on the final result
		cmd.Status = finalResult.Status

		// Process and print the single final JSON result
		fmt.Println("Final Result (JSON):")
		jsonResult, err := json.MarshalIndent(finalResult, "", "  ") // Pretty print JSON
		if err != nil {
			log.Printf("ERROR: Failed to marshal final result to JSON for %s (%s): %v", cmdType, cmdID, err)
			// Print basic info if JSON fails
			fmt.Printf("  Fallback Final Result: Status=%s, Msg='%s', Err='%s', DataLen=%d\n",
				finalResult.Status, finalResult.Message, finalResult.Error, len(finalResult.ResultData))
		} else {
			fmt.Println(string(jsonResult))
		}

		// Print the task after execution to show mutations
		fmt.Println("TASK AFTER EXECUTION:")
		printTaskAsJSON("AFTER EXECUTION", cmdGeneric)

		fmt.Printf("--- Finished Command %s (Final Status: %s) ---\n", cmdID, finalResult.Status)
	}

	fmt.Println("\nCommand runner happy path demo complete.")
}

// printTaskAsJSON prints a task object as formatted JSON
func printTaskAsJSON(label string, taskObj interface{}) {
	fmt.Printf("%s Task JSON:\n", label)
	jsonTask, err := json.MarshalIndent(taskObj, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling task to JSON: %v\n", err)
		return
	}
	fmt.Println(string(jsonTask))
}
