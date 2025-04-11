package command_test

import (
	"ai-agent-v3/internal/command"
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	// cmpopts is not needed for these simple comparisons
)

// TODO: Add tests for type validation or helper functions if any are added.

func TestBaseCommandDeserialization(t *testing.T) {
	jsonData := `{
		"command_id": "test-cmd-123",
		"description": "This is a test command"
	}`
	var cmd command.BaseCommand
	err := json.Unmarshal([]byte(jsonData), &cmd)
	if err != nil {
		t.Fatalf("Failed to unmarshal BaseCommand JSON: %v", err)
	}

	expected := command.BaseCommand{
		CommandID:   "test-cmd-123",
		Description: "This is a test command",
	}

	if diff := cmp.Diff(expected, cmd); diff != "" {
		t.Errorf("BaseCommand mismatch (-want +got):\n%s", diff)
	}
}

func TestOutputResultDeserialization(t *testing.T) {
	// Corrected Status to StatusSucceeded and ResultData to string
	jsonData := "{\n\t\t\"command_id\": \"res-cmd-456\",\n\t\t\"status\": \"SUCCEEDED\",\n\t\t\"message\": \"Operation successful\",\n\t\t\"error\": \"\",\n\t\t\"resultData\": \"line1\\nline2\"\n\t}"
	var result command.OutputResult
	err := json.Unmarshal([]byte(jsonData), &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal OutputResult JSON: %v", err)
	}

	expected := command.OutputResult{
		CommandID:  "res-cmd-456",
		Status:     command.StatusSucceeded, // Correct constant
		Message:    "Operation successful",
		Error:      "",
		ResultData: "line1\nline2", // Correct type (string)
	}

	if diff := cmp.Diff(expected, result); diff != "" {
		t.Errorf("OutputResult mismatch (-want +got):\n%s", diff)
	}
}

func TestBashExecCommandDeserialization(t *testing.T) {
	// Removed non-existent fields: timeout_seconds, working_directory
	jsonData := "{\n\t\t\"command_id\": \"bash-exec-001\",\n\t\t\"description\": \"Execute echo\",\n\t\t\"command\": \"echo 'hello world'\"\n\t}"
	var cmd command.BashExecCommand
	err := json.Unmarshal([]byte(jsonData), &cmd)
	if err != nil {
		t.Fatalf("Failed to unmarshal BashExecCommand JSON: %v", err)
	}

	expected := command.BashExecCommand{
		BaseCommand: command.BaseCommand{
			CommandID:   "bash-exec-001",
			Description: "Execute echo",
		},
		Command: "echo 'hello world'",
		// Removed non-existent fields TimeoutSeconds, WorkingDirectory
	}

	if diff := cmp.Diff(expected, cmd); diff != "" {
		t.Errorf("BashExecCommand mismatch (-want +got):\n%s", diff)
	}
}

func TestFileReadCommandDeserialization(t *testing.T) {
	// Removed non-existent fields: start_line, end_line
	jsonData := "{\n\t\t\"command_id\": \"read-file-002\",\n\t\t\"description\": \"Read a config file\",\n\t\t\"file_path\": \"/etc/config.yaml\"\n\t}"
	var cmd command.FileReadCommand
	err := json.Unmarshal([]byte(jsonData), &cmd)
	if err != nil {
		t.Fatalf("Failed to unmarshal FileReadCommand JSON: %v", err)
	}

	expected := command.FileReadCommand{
		BaseCommand: command.BaseCommand{
			CommandID:   "read-file-002",
			Description: "Read a config file",
		},
		FilePath: "/etc/config.yaml",
		// Removed non-existent fields StartLine, EndLine
	}

	if diff := cmp.Diff(expected, cmd); diff != "" {
		t.Errorf("FileReadCommand mismatch (-want +got):\n%s", diff)
	}
}

func TestFileWriteCommandDeserialization(t *testing.T) {
	// Removed non-existent field: append
	jsonData := "{\n\t\t\"command_id\": \"write-file-003\",\n\t\t\"description\": \"Write data to a log file\",\n\t\t\"file_path\": \"/var/log/app.log\",\n\t\t\"content\": \"Log entry: success\"\n\t}"
	var cmd command.FileWriteCommand
	err := json.Unmarshal([]byte(jsonData), &cmd)
	if err != nil {
		t.Fatalf("Failed to unmarshal FileWriteCommand JSON: %v", err)
	}

	expected := command.FileWriteCommand{
		BaseCommand: command.BaseCommand{
			CommandID:   "write-file-003",
			Description: "Write data to a log file",
		},
		FilePath: "/var/log/app.log",
		Content:  "Log entry: success",
		// Removed non-existent field Append
	}

	if diff := cmp.Diff(expected, cmd); diff != "" {
		t.Errorf("FileWriteCommand mismatch (-want +got):\n%s", diff)
	}
}

func TestListDirectoryCommandDeserialization(t *testing.T) {
	// Removed non-existent field: recursive
	jsonData := "{\n\t\t\"command_id\": \"list-dir-004\",\n\t\t\"description\": \"List contents of /home/user\",\n\t\t\"path\": \"/home/user\"\n\t}"
	var cmd command.ListDirectoryCommand
	err := json.Unmarshal([]byte(jsonData), &cmd)
	if err != nil {
		t.Fatalf("Failed to unmarshal ListDirectoryCommand JSON: %v", err)
	}

	expected := command.ListDirectoryCommand{
		BaseCommand: command.BaseCommand{
			CommandID:   "list-dir-004",
			Description: "List contents of /home/user",
		},
		Path: "/home/user",
		// Removed non-existent field Recursive
	}

	if diff := cmp.Diff(expected, cmd); diff != "" {
		t.Errorf("ListDirectoryCommand mismatch (-want +got):\n%s", diff)
	}
}

func TestPatchFileCommandDeserialization(t *testing.T) {
	// Note the escaped backslashes in the patch content JSON
	jsonData := "{\n\t\t\"command_id\": \"patch-file-005\",\n\t\t\"description\": \"Apply patch to main.go\",\n\t\t\"file_path\": \"src/main.go\",\n\t\t\"patch\": \"--- a/main.go\\n+++ b/main.go\\n@@ -1 +1 @@\\n-hello\\n+world\"\n\t}"
	var cmd command.PatchFileCommand
	err := json.Unmarshal([]byte(jsonData), &cmd)
	if err != nil {
		t.Fatalf("Failed to unmarshal PatchFileCommand JSON: %v", err)
	}

	expected := command.PatchFileCommand{
		BaseCommand: command.BaseCommand{
			CommandID:   "patch-file-005",
			Description: "Apply patch to main.go",
		},
		FilePath: "src/main.go",
		Patch:    "--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-hello\n+world", // The expected patch string should not be escaped here
	}

	if diff := cmp.Diff(expected, cmd); diff != "" {
		t.Errorf("PatchFileCommand mismatch (-want +got):\n%s", diff)
	}
}
