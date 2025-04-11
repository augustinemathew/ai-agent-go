package command

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/sourcegraph/go-diff/diff"
)

// --- Patching Logic (Adapted from provided model) ---

var (
	// errParseFailed indicates the patch content could not be parsed.
	errParseFailed = errors.New("failed to parse patch")
	// errMultiFilePatch indicates the provided patch contains diffs for more than one file.
	errMultiFilePatch = errors.New("patch contains multiple file diffs, only single file patches are supported")
	// errNoFilePatch indicates the parsed patch did not contain any file diffs.
	errNoFilePatch = errors.New("failed to parse patch: no valid hunks found")
	// errHunkMismatch indicates a hunk could not be applied because the context lines didn't match the original content.
	errHunkMismatch = errors.New("hunk context does not match original content")
)

// applyPatch applies a unified diff patch to the original content.
// It assumes the patch applies to a single file and uses github.com/sourcegraph/go-diff.
func applyPatch(originalContent []byte, patchContent []byte) ([]byte, error) {
	// Handle empty patch edge case upfront
	if len(bytes.TrimSpace(patchContent)) == 0 {
		return originalContent, nil // Applying empty patch is a no-op
	}

	// Parse the patch
	fileDiffs, err := diff.ParseMultiFileDiff(patchContent)
	if err != nil {
		return nil, fmt.Errorf("failed to parse patch: %v", err)
	}

	if len(fileDiffs) == 0 {
		return nil, errNoFilePatch
	}

	if len(fileDiffs) > 1 {
		return nil, errMultiFilePatch
	}

	fileDiff := fileDiffs[0]

	// Special handling for file creation patch (/dev/null source)
	if fileDiff.OrigName == "/dev/null" {
		return handleFileCreation(fileDiff)
	}

	// Special handling for file deletion patch (/dev/null destination)
	if fileDiff.NewName == "/dev/null" {
		return []byte{}, nil // Return empty content for file deletion
	}

	// Prepare original content lines
	originalLines := prepareOriginalLines(originalContent)

	// Apply the patch to the original content
	return applyFileDiff(fileDiff, originalLines, bytes.HasSuffix(originalContent, []byte("\n")))
}

// handleFileCreation processes a file creation diff (/dev/null source)
func handleFileCreation(fileDiff *diff.FileDiff) ([]byte, error) {
	var result [][]byte
	for _, hunk := range fileDiff.Hunks {
		hunkLines := bytes.Split(hunk.Body, []byte("\n"))
		for _, line := range hunkLines {
			if len(line) > 0 && line[0] == '+' {
				result = append(result, line[1:])
			}
		}
	}

	// Join with newlines and add final newline
	if len(result) > 0 {
		return append(bytes.Join(result, []byte("\n")), '\n'), nil
	}
	return []byte{}, nil
}

// prepareOriginalLines splits the original content into lines, handling trailing newlines
func prepareOriginalLines(originalContent []byte) [][]byte {
	originalLines := bytes.Split(originalContent, []byte("\n"))
	if len(originalContent) > 0 && !bytes.HasSuffix(originalContent, []byte("\n")) {
		originalLines = append(originalLines, []byte{}) // Add empty line if content doesn't end with newline
	}
	return originalLines
}

// applyFileDiff applies a file diff to original lines and returns the patched content
func applyFileDiff(fileDiff *diff.FileDiff, originalLines [][]byte, preserveTrailingNewline bool) ([]byte, error) {
	var result [][]byte
	currentLine := 0

	for _, hunk := range fileDiff.Hunks {
		// Add lines before the hunk
		for ; currentLine < int(hunk.OrigStartLine-1); currentLine++ {
			if currentLine < len(originalLines) {
				result = append(result, originalLines[currentLine])
			}
		}

		// Process the hunk
		hunkLines := bytes.Split(hunk.Body, []byte("\n"))
		for lineIdx, line := range hunkLines {
			// Skip empty line at end of hunk (trailing newline)
			if len(line) == 0 && lineIdx == len(hunkLines)-1 {
				continue
			}

			// Empty line in middle of hunk
			if len(line) == 0 {
				result = append(result, []byte{})
				continue
			}

			// Process line based on prefix
			switch line[0] {
			case ' ': // Context line
				if err := verifyContextLine(line, originalLines, currentLine); err != nil {
					return nil, err
				}
				result = append(result, originalLines[currentLine])
				currentLine++
			case '-': // Deletion line
				if err := verifyDeletionLine(line, originalLines, currentLine); err != nil {
					return nil, err
				}
				currentLine++
			case '+': // Addition line
				result = append(result, line[1:])
			}
		}
	}

	// Add remaining lines after last hunk
	addRemainingLines(&result, originalLines, currentLine)

	// Join lines and handle final newline
	return formatFinalOutput(result, fileDiff, preserveTrailingNewline)
}

// verifyContextLine checks if a context line in the patch matches the original content
func verifyContextLine(line []byte, originalLines [][]byte, currentLine int) error {
	if currentLine >= len(originalLines) {
		return fmt.Errorf("context mismatch: expected '%s', got end of file at line %d",
			string(line[1:]), currentLine+1)
	}

	originalLine := bytes.TrimRight(originalLines[currentLine], "\n\r")
	patchLine := bytes.TrimRight(line[1:], "\n\r")

	if !bytes.Equal(originalLine, patchLine) {
		return fmt.Errorf("context mismatch: expected '%s', got '%s' at original line %d",
			string(patchLine), string(originalLine), currentLine+1)
	}

	return nil
}

// verifyDeletionLine checks if a deletion line in the patch matches the original content
func verifyDeletionLine(line []byte, originalLines [][]byte, currentLine int) error {
	if currentLine >= len(originalLines) {
		return fmt.Errorf("context mismatch: expected removal of '%s', got end of file at line %d",
			string(line[1:]), currentLine+1)
	}

	originalLine := bytes.TrimRight(originalLines[currentLine], "\n\r")
	patchLine := bytes.TrimRight(line[1:], "\n\r")

	if !bytes.Equal(originalLine, patchLine) {
		return fmt.Errorf("context mismatch: expected removal of '%s', got '%s' at original line %d",
			string(patchLine), string(originalLine), currentLine+1)
	}

	return nil
}

// addRemainingLines adds any lines from the original content that come after the last hunk
func addRemainingLines(result *[][]byte, originalLines [][]byte, currentLine int) {
	for ; currentLine < len(originalLines)-1 ||
		(currentLine == len(originalLines)-1 && len(originalLines[currentLine]) > 0); currentLine++ {
		*result = append(*result, originalLines[currentLine])
	}
}

// formatFinalOutput joins the result lines and adds a final newline if needed
func formatFinalOutput(result [][]byte, fileDiff *diff.FileDiff, preserveTrailingNewline bool) ([]byte, error) {
	if len(result) == 0 {
		return []byte{}, nil
	}

	output := bytes.Join(result, []byte("\n"))

	// Add final newline if original had one or if last hunk ends with newline
	if preserveTrailingNewline ||
		(len(fileDiff.Hunks) > 0 && bytes.HasSuffix(fileDiff.Hunks[len(fileDiff.Hunks)-1].Body, []byte("\n"))) {
		output = append(output, '\n')
	}

	return output, nil
}

// --- Executor Implementation ---

// PatchFileExecutor handles the execution of PatchFileCommand.
// It reads the target file, applies the patch using the internal applyPatch function,
// and writes the result back to the file.
type PatchFileExecutor struct {
	// No dependencies needed.
}

// NewPatchFileExecutor creates a new PatchFileExecutor.
func NewPatchFileExecutor() *PatchFileExecutor {
	return &PatchFileExecutor{}
}

// Execute applies a patch to the file specified in the PatchFileCommand.
// It expects the cmd argument to be of type *PatchFileCommand or PatchFileCommand.
// Returns a channel for results and an error if the command type is wrong or execution setup fails.
func (e *PatchFileExecutor) Execute(ctx context.Context, cmd any) (<-chan OutputResult, error) {
	patchCmd, ok := cmd.(*PatchFileCommand)
	if !ok {
		valueCmd, okValue := cmd.(PatchFileCommand)
		if !okValue {
			return nil, fmt.Errorf("invalid command type: expected *PatchFileCommand or PatchFileCommand, got %T", cmd)
		}
		patchCmd = &valueCmd
	}

	if patchCmd.FilePath == "" {
		return nil, errors.New("file path cannot be empty for PATCH_FILE")
	}

	// Handle empty patch as a no-op success case early.
	if strings.TrimSpace(patchCmd.Patch) == "" {
		results := make(chan OutputResult, 1)
		results <- OutputResult{
			CommandID:   patchCmd.CommandID,
			CommandType: CmdPatchFile,
			Status:      StatusSucceeded,
			Message:     "Empty patch provided. No changes applied to file: " + patchCmd.FilePath,
		}
		close(results)
		return results, nil
	}

	results := make(chan OutputResult, 1) // Buffer 1 for the final result

	go func() {
		startTime := time.Now()
		defer close(results)
		log.Printf("[PatchFile %s] Starting execution for file: %s", patchCmd.CommandID, patchCmd.FilePath)

		// 1. Check for immediate cancellation
		select {
		case <-ctx.Done():
			log.Printf("[PatchFile %s] Cancelled before start.", patchCmd.CommandID)
			results <- OutputResult{
				CommandID:   patchCmd.CommandID,
				CommandType: CmdPatchFile,
				Status:      StatusFailed,
				Message:     fmt.Sprintf("File patching cancelled before start for %s.", patchCmd.FilePath),
				Error:       ctx.Err().Error(),
			}
			return
		default:
		}

		// 2. Read original file content
		log.Printf("[PatchFile %s] Reading original file...", patchCmd.CommandID)
		originalContent, err := os.ReadFile(patchCmd.FilePath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("[PatchFile %s] Error reading original file: %v", patchCmd.CommandID, err)
			results <- OutputResult{
				CommandID:   patchCmd.CommandID,
				CommandType: CmdPatchFile,
				Status:      StatusFailed,
				Message:     fmt.Sprintf("Failed to read original file %s", patchCmd.FilePath),
				Error:       err.Error(),
			}
			return
		}
		log.Printf("[PatchFile %s] Original file read (exists: %t, size: %d bytes).", patchCmd.CommandID, !errors.Is(err, os.ErrNotExist), len(originalContent))

		// 3. Apply the patch using the internal function
		log.Printf("[PatchFile %s] Applying patch...", patchCmd.CommandID)
		patchedContent, err := applyPatch(originalContent, []byte(patchCmd.Patch))
		if err != nil {
			log.Printf("[PatchFile %s] Error applying patch: %v", patchCmd.CommandID, err)
			// Map specific patcher errors for clarity
			errMsg := fmt.Sprintf("Failed to apply patch to file %s", patchCmd.FilePath)
			patchErrStr := err.Error()
			if errors.Is(err, errParseFailed) {
				errMsg = fmt.Sprintf("Failed to parse patch content for file %s", patchCmd.FilePath)
			} else if errors.Is(err, errHunkMismatch) {
				errMsg = fmt.Sprintf("Patch context mismatch for file %s", patchCmd.FilePath)
			} else if errors.Is(err, errMultiFilePatch) {
				errMsg = fmt.Sprintf("Patch contained multiple file diffs (unsupported) for %s", patchCmd.FilePath)
			} // Add other specific errors as needed

			results <- OutputResult{
				CommandID:   patchCmd.CommandID,
				CommandType: CmdPatchFile,
				Status:      StatusFailed,
				Message:     errMsg,
				Error:       patchErrStr, // Include the detailed error from patcher
			}
			return
		}
		log.Printf("[PatchFile %s] Patch applied successfully (new size: %d bytes).", patchCmd.CommandID, len(patchedContent))

		// 4. Check for cancellation before writing
		select {
		case <-ctx.Done():
			log.Printf("[PatchFile %s] Cancelled before writing.", patchCmd.CommandID)
			results <- OutputResult{
				CommandID:   patchCmd.CommandID,
				CommandType: CmdPatchFile,
				Status:      StatusFailed,
				Message:     fmt.Sprintf("File patching cancelled before writing to %s.", patchCmd.FilePath),
				Error:       ctx.Err().Error(),
			}
			return
		default:
		}

		// 5. Write the patched content back to the file
		log.Printf("[PatchFile %s] Determining permissions and writing patched file...", patchCmd.CommandID)
		fileInfo, statErr := os.Stat(patchCmd.FilePath)
		perm := os.FileMode(0644)
		if statErr == nil {
			perm = fileInfo.Mode().Perm()
		} else if !errors.Is(statErr, os.ErrNotExist) {
			results <- OutputResult{
				CommandID:   patchCmd.CommandID,
				CommandType: CmdPatchFile,
				Status:      StatusFailed,
				Message:     fmt.Sprintf("Failed to stat original file %s before writing patch", patchCmd.FilePath),
				Error:       statErr.Error(),
			}
			return
		}

		err = os.WriteFile(patchCmd.FilePath, patchedContent, perm)
		if err != nil {
			log.Printf("[PatchFile %s] Error writing patched file: %v", patchCmd.CommandID, err)
			results <- OutputResult{
				CommandID:   patchCmd.CommandID,
				CommandType: CmdPatchFile,
				Status:      StatusFailed,
				Message:     fmt.Sprintf("Failed to write patched content to file %s", patchCmd.FilePath),
				Error:       err.Error(),
			}
			return
		}
		log.Printf("[PatchFile %s] Patched file written successfully.", patchCmd.CommandID)

		// 6. Success
		duration := time.Since(startTime)
		log.Printf("[PatchFile %s] Execution succeeded in %s.", patchCmd.CommandID, duration.Round(time.Millisecond))
		results <- OutputResult{
			CommandID:   patchCmd.CommandID,
			CommandType: CmdPatchFile,
			Status:      StatusSucceeded,
			Message:     fmt.Sprintf("Successfully applied patch to %s in %s.", patchCmd.FilePath, duration.Round(time.Millisecond)),
		}
	}()

	return results, nil
}
