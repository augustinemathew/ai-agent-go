package main

import (
	"ai-agent-v3/internal/command"
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
	registry := command.NewMapRegistry()
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
	}()

	// Define the commands
	commandsToRun := []interface{}{
		command.FileWriteCommand{
			BaseCommand: command.BaseCommand{CommandID: "happy-write-1", Description: "Write demo file"},
			FilePath:    tempFilePath,
			Content:     fileContent,
		},
		command.ListDirectoryCommand{
			BaseCommand: command.BaseCommand{CommandID: "happy-list-1", Description: "List temp directory"},
			Path:        tempDir, // List the directory containing the temp file
		},
		command.FileReadCommand{
			BaseCommand: command.BaseCommand{CommandID: "happy-read-1", Description: "Read demo file"},
			FilePath:    tempFilePath,
		},
		command.BashExecCommand{
			BaseCommand: command.BaseCommand{CommandID: "happy-bash-1", Description: "Simple echo"},
			Command:     "echo \"Hello from Bash!\"",
		},
		// Add a multiline Bash script example
		command.BashExecCommand{
			BaseCommand: command.BaseCommand{CommandID: "happy-bash-2", Description: "Multiline Bash script"},
			Command:     "echo \"Starting multiline script...\"\necho \"Current directory: $(pwd)\"\nls -la\necho \"Environment variables:\"\nenv | grep PATH\necho \"Script complete!\"",
		},
		// Add a PATCH_FILE command
		command.PatchFileCommand{
			BaseCommand: command.BaseCommand{CommandID: "happy-patch-1", Description: "Patch demo file"},
			FilePath:    tempFilePath,
			// Patch to change "Hello" to "Greetings"
			Patch: fmt.Sprintf("--- a/%s\n+++ b/%s\n@@ -1,2 +1,2 @@\n-Hello from FileWrite!\n+Greetings from PatchFile!\n This is a test file.\n", tempFileName, tempFileName),
		},
		// Read the file again to see the patch result
		command.FileReadCommand{
			BaseCommand: command.BaseCommand{CommandID: "happy-read-2", Description: "Read patched demo file"},
			FilePath:    tempFilePath,
		},
		// Demonstrate line number functionality
		command.FileReadCommand{
			BaseCommand: command.BaseCommand{CommandID: "happy-read-3", Description: "Read specific lines from file"},
			FilePath:    tempFilePath,
			StartLine:   2, // Start from second line
			EndLine:     3, // Read until third line
		},
	}

	// Execute commands sequentially
	for i, cmdGeneric := range commandsToRun {
		fmt.Printf("\n--- Executing Command %d ---\n", i+1)

		var cmdType command.CommandType
		var cmdID string

		// Use type assertion to get CommandID and determine CommandType
		switch cmd := cmdGeneric.(type) {
		case command.FileWriteCommand:
			cmdType = command.CmdFileWrite
			cmdID = cmd.CommandID
			fmt.Printf("Type: %s, ID: %s, Desc: %s, Path: %s\n", cmdType, cmdID, cmd.Description, cmd.FilePath)
		case command.ListDirectoryCommand:
			cmdType = command.CmdListDirectory
			cmdID = cmd.CommandID
			fmt.Printf("Type: %s, ID: %s, Desc: %s, Path: %s\n", cmdType, cmdID, cmd.Description, cmd.Path)
		case command.FileReadCommand:
			cmdType = command.CmdFileRead
			cmdID = cmd.CommandID
			fmt.Printf("Type: %s, ID: %s, Desc: %s, Path: %s\n", cmdType, cmdID, cmd.Description, cmd.FilePath)
		case command.BashExecCommand:
			cmdType = command.CmdBashExec
			cmdID = cmd.CommandID
			fmt.Printf("Type: %s, ID: %s, Desc: %s, Command: %s\n", cmdType, cmdID, cmd.Description, cmd.Command)
		case command.PatchFileCommand:
			cmdType = command.CmdPatchFile
			cmdID = cmd.CommandID
			fmt.Printf("Type: %s, ID: %s, Desc: %s, Path: %s\\n", cmdType, cmdID, cmd.Description, cmd.FilePath)
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
		finalResult := command.CombineOutputResults(execCtx, resultsChan)

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
