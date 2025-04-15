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
	relativeFileName := tempFileName // For relative path examples
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
		task.FileWriteTask{
			BaseTask: task.BaseTask{TaskId: "happy-write-1", Description: "Write demo file"},
			Parameters: task.FileWriteParameters{
				FilePath: tempFilePath,
				Content:  fileContent,
			},
		},
		task.ListDirectoryTask{
			BaseTask: task.BaseTask{TaskId: "happy-list-1", Description: "List temp directory"},
			Parameters: task.ListDirectoryParameters{
				Path: tempDir, // List the directory containing the temp file
			},
		},
		task.FileReadTask{
			BaseTask: task.BaseTask{TaskId: "happy-read-1", Description: "Read demo file"},
			Parameters: task.FileReadParameters{
				FilePath: tempFilePath,
			},
		},
		task.BashExecTask{
			BaseTask: task.BaseTask{TaskId: "happy-bash-1", Description: "Simple echo"},
			Parameters: task.BashExecParameters{
				Command: "echo \"Hello from Bash!\"",
			},
		},
		// Add a multiline Bash script example
		task.BashExecTask{
			BaseTask: task.BaseTask{TaskId: "happy-bash-2", Description: "Multiline Bash script"},
			Parameters: task.BashExecParameters{
				Command: "echo \"Starting multiline script...\"\necho \"Current directory: $(pwd)\"\nls -la\necho \"Environment variables:\"\nenv | grep PATH\necho \"Script complete!\"",
			},
		},
		// Add a PATCH_FILE command
		task.PatchFileTask{
			BaseTask: task.BaseTask{TaskId: "happy-patch-1", Description: "Patch demo file"},
			Parameters: task.PatchFileParameters{
				FilePath: tempFilePath,
				// Patch to change "Hello" to "Greetings"
				Patch: fmt.Sprintf("--- a/%s\n+++ b/%s\n@@ -1,2 +1,2 @@\n-Hello from FileWrite!\n+Greetings from PatchFile!\n This is a test file.\n", tempFileName, tempFileName),
			},
		},
		// Read the file again to see the patch result
		task.FileReadTask{
			BaseTask: task.BaseTask{TaskId: "happy-read-2", Description: "Read patched demo file"},
			Parameters: task.FileReadParameters{
				FilePath: tempFilePath,
			},
		},
		// Demonstrate line number functionality
		task.FileReadTask{
			BaseTask: task.BaseTask{TaskId: "happy-read-3", Description: "Read specific lines from file"},
			Parameters: task.FileReadParameters{
				FilePath:  tempFilePath,
				StartLine: 2, // Start from second line
				EndLine:   3, // Read until third line
			},
		},
		// EXAMPLE 1: Using relative paths with WorkingDirectory
		task.FileWriteTask{
			BaseTask: task.BaseTask{TaskId: "relative-write-1", Description: "Write file using relative path"},
			Parameters: task.FileWriteParameters{
				BaseParameters: task.BaseParameters{
					WorkingDirectory: tempDir, // Set the working directory
				},
				FilePath: "relative-file1.txt", // Relative to WorkingDirectory
				Content:  "This file was created using a relative path with WorkingDirectory parameter!",
			},
		},
		// Read the file created with relative path
		task.FileReadTask{
			BaseTask: task.BaseTask{TaskId: "relative-read-1", Description: "Read file using relative path"},
			Parameters: task.FileReadParameters{
				BaseParameters: task.BaseParameters{
					WorkingDirectory: tempDir, // Same working directory
				},
				FilePath: "relative-file1.txt", // Relative to WorkingDirectory
			},
		},
		// EXAMPLE 2: BashExec with WorkingDirectory
		task.BashExecTask{
			BaseTask: task.BaseTask{TaskId: "relative-bash-1", Description: "Run bash in specified working directory"},
			Parameters: task.BashExecParameters{
				BaseParameters: task.BaseParameters{
					WorkingDirectory: tempDir, // Set the working directory
				},
				Command: "pwd && touch relative-file2.txt && ls -la relative-file2.txt",
			},
		},
		// EXAMPLE 3: Patch a file using relative path
		task.FileWriteTask{
			BaseTask: task.BaseTask{TaskId: "relative-base-file", Description: "Create file to be patched"},
			Parameters: task.FileWriteParameters{
				BaseParameters: task.BaseParameters{
					WorkingDirectory: tempDir,
				},
				FilePath: relativeFileName,
				Content:  "Original content in relative file.\nThis line will stay the same.",
			},
		},
		task.PatchFileTask{
			BaseTask: task.BaseTask{TaskId: "relative-patch-1", Description: "Patch file using relative path"},
			Parameters: task.PatchFileParameters{
				BaseParameters: task.BaseParameters{
					WorkingDirectory: tempDir,
				},
				FilePath: relativeFileName, // Relative to WorkingDirectory
				Patch:    "--- a/file.txt\n+++ b/file.txt\n@@ -1,2 +1,2 @@\n-Original content in relative file.\n+Modified content in relative file.\n This line will stay the same.",
			},
		},
		// Read the patched file
		task.FileReadTask{
			BaseTask: task.BaseTask{TaskId: "relative-read-patched", Description: "Read patched file with relative path"},
			Parameters: task.FileReadParameters{
				BaseParameters: task.BaseParameters{
					WorkingDirectory: tempDir,
				},
				FilePath: relativeFileName,
			},
		},
		// Add a GROUP task example
		task.Task{
			BaseTask: task.BaseTask{
				TaskId:      "happy-group-1",
				Description: "Group task example with nested tasks",
				Type:        task.TaskGroup,
				Children: []task.Task{
					{
						BaseTask: task.BaseTask{
							TaskId:      "group-child-1",
							Description: "Create a new file in the group",
							Type:        task.TaskFileWrite,
						},
						Parameters: task.FileWriteParameters{
							FilePath: filepath.Join(tempDir, "group-file1.txt"),
							Content:  "This file was created by a child task in a group.",
						},
					},
					{
						BaseTask: task.BaseTask{
							TaskId:      "group-nested",
							Description: "Nested group task",
							Type:        task.TaskGroup,
							Children: []task.Task{
								{
									BaseTask: task.BaseTask{
										TaskId:      "nested-child-1",
										Description: "Create another file in nested group",
										Type:        task.TaskFileWrite,
									},
									Parameters: task.FileWriteParameters{
										FilePath: filepath.Join(tempDir, "group-file2.txt"),
										Content:  "This file was created by a nested group child task.",
									},
								},
								{
									BaseTask: task.BaseTask{
										TaskId:      "nested-child-2",
										Description: "Read the file we just created",
										Type:        task.TaskFileRead,
									},
									Parameters: task.FileReadParameters{
										FilePath: filepath.Join(tempDir, "group-file2.txt"),
									},
								},
							},
						},
					},
					{
						BaseTask: task.BaseTask{
							TaskId:      "group-child-2",
							Description: "Run a bash command in the group",
							Type:        task.TaskBashExec,
						},
						Parameters: task.BashExecParameters{
							Command: "echo \"This is executed as part of a group task.\"",
						},
					},
				},
			},
		},
	}

	// Execute commands sequentially
	for i, cmdGeneric := range commandsToRun {
		fmt.Printf("\n--- Executing Command %d ---\n", i+1)

		var cmdType task.TaskType
		var cmdID string

		// Use type assertion to get CommandID and determine CommandType
		switch cmd := cmdGeneric.(type) {
		case task.FileWriteTask:
			cmdType = task.TaskFileWrite
			cmdID = cmd.TaskId
			workingDir := cmd.Parameters.WorkingDirectory
			if workingDir != "" {
				fmt.Printf("Type: %s, ID: %s, Desc: %s, WorkingDir: %s, Path: %s\n",
					cmdType, cmdID, cmd.Description, workingDir, cmd.Parameters.FilePath)
			} else {
				fmt.Printf("Type: %s, ID: %s, Desc: %s, Path: %s\n",
					cmdType, cmdID, cmd.Description, cmd.Parameters.FilePath)
			}
		case task.ListDirectoryTask:
			cmdType = task.TaskListDirectory
			cmdID = cmd.TaskId
			workingDir := cmd.Parameters.WorkingDirectory
			if workingDir != "" {
				fmt.Printf("Type: %s, ID: %s, Desc: %s, WorkingDir: %s, Path: %s\n",
					cmdType, cmdID, cmd.Description, workingDir, cmd.Parameters.Path)
			} else {
				fmt.Printf("Type: %s, ID: %s, Desc: %s, Path: %s\n",
					cmdType, cmdID, cmd.Description, cmd.Parameters.Path)
			}
		case task.FileReadTask:
			cmdType = task.TaskFileRead
			cmdID = cmd.TaskId
			workingDir := cmd.Parameters.WorkingDirectory
			if workingDir != "" {
				fmt.Printf("Type: %s, ID: %s, Desc: %s, WorkingDir: %s, Path: %s\n",
					cmdType, cmdID, cmd.Description, workingDir, cmd.Parameters.FilePath)
			} else {
				fmt.Printf("Type: %s, ID: %s, Desc: %s, Path: %s\n",
					cmdType, cmdID, cmd.Description, cmd.Parameters.FilePath)
			}
		case task.BashExecTask:
			cmdType = task.TaskBashExec
			cmdID = cmd.TaskId
			workingDir := cmd.Parameters.WorkingDirectory
			if workingDir != "" {
				fmt.Printf("Type: %s, ID: %s, Desc: %s, WorkingDir: %s, Command: %s\n",
					cmdType, cmdID, cmd.Description, workingDir, cmd.Parameters.Command)
			} else {
				fmt.Printf("Type: %s, ID: %s, Desc: %s, Command: %s\n",
					cmdType, cmdID, cmd.Description, cmd.Parameters.Command)
			}
		case task.PatchFileTask:
			cmdType = task.TaskPatchFile
			cmdID = cmd.TaskId
			workingDir := cmd.Parameters.WorkingDirectory
			if workingDir != "" {
				fmt.Printf("Type: %s, ID: %s, Desc: %s, WorkingDir: %s, Path: %s\n",
					cmdType, cmdID, cmd.Description, workingDir, cmd.Parameters.FilePath)
			} else {
				fmt.Printf("Type: %s, ID: %s, Desc: %s, Path: %s\n",
					cmdType, cmdID, cmd.Description, cmd.Parameters.FilePath)
			}
		case task.Task:
			cmdType = cmd.Type
			cmdID = cmd.TaskId
			if cmd.Type == task.TaskGroup {
				fmt.Printf("Type: %s, ID: %s, Desc: %s, Children: %d\n", cmdType, cmdID, cmd.Description, len(cmd.Children))
			} else {
				fmt.Printf("Type: %s, ID: %s, Desc: %s\n", cmdType, cmdID, cmd.Description)
			}
		default:
			log.Printf("Unknown command type in sequence: %T", cmdGeneric)
			continue
		}

		executor, err := registry.GetExecutor(cmdType)
		if err != nil {
			log.Printf("ERROR: Failed to get executor for %s (%s): %v", cmdType, cmdID, err)
			continue
		}

		// Execute
		// Use a background context for now, could be replaced with a request-scoped context
		execCtx := context.Background()
		resultsChan, err := executor.Execute(execCtx, cmdGeneric)
		if err != nil {
			log.Printf("ERROR: Failed to start execution for %s (%s): %v", cmdType, cmdID, err)
			continue
		}

		// Use the utility to collect the final result
		fmt.Printf("Collecting results for %s...\n", cmdID)
		finalResult := task.CombineOutputResults(execCtx, resultsChan)

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

		fmt.Printf("--- Finished Command %s (Final Status: %s) ---\n", cmdID, finalResult.Status)
	}

	fmt.Println("\nCommand runner happy path demo complete.")
}
