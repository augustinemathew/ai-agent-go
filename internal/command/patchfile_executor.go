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
	errNoFilePatch = errors.New("patch does not contain any file diffs")
	// errHunkMismatch indicates a hunk could not be applied because the context lines didn't match the original content.
	errHunkMismatch = errors.New("hunk context does not match original content")
	// errLineOutOfBounds indicates a hunk refers to line numbers outside the bounds of the original content.
	errLineOutOfBounds = errors.New("hunk refers to line numbers out of bounds")
)

// applyPatch applies a unified diff patch to the original content.
// It assumes the patch applies to a single file and uses github.com/sourcegraph/go-diff.
func applyPatch(originalContent []byte, patchContent []byte) ([]byte, error) {
	// Handle empty patch edge case upfront
	if len(bytes.TrimSpace(patchContent)) == 0 {
		log.Printf("DEBUG: Empty patch detected, returning original content")
		return originalContent, nil // Applying empty patch is a no-op
	}

	log.Printf("DEBUG: Parsing patch content:\n%s", string(patchContent))
	fileDiffs, err := diff.ParseMultiFileDiff(patchContent)
	if err != nil {
		log.Printf("DEBUG: Failed to parse patch: %v", err)
		return nil, fmt.Errorf("failed to parse patch: %v", err)
	}

	if len(fileDiffs) == 0 {
		log.Printf("DEBUG: No valid hunks found in patch")
		return nil, fmt.Errorf("failed to parse patch: no valid hunks found")
	}

	if len(fileDiffs) > 1 {
		log.Printf("DEBUG: Multiple file diffs found: %d", len(fileDiffs))
		return nil, fmt.Errorf("patch contains multiple file diffs, only single file patches are supported")
	}

	fileDiff := fileDiffs[0]
	log.Printf("DEBUG: Processing file diff - OrigName: %s, NewName: %s", fileDiff.OrigName, fileDiff.NewName)

	// Special handling for /dev/null source in creation patches
	if fileDiff.OrigName == "/dev/null" {
		log.Printf("DEBUG: Handling file creation patch")
		var result [][]byte
		for _, hunk := range fileDiff.Hunks {
			log.Printf("DEBUG: Processing creation hunk: %d lines", len(hunk.Body))
			hunkLines := bytes.Split(hunk.Body, []byte("\n"))
			for _, line := range hunkLines {
				if len(line) > 0 && line[0] == '+' {
					log.Printf("DEBUG: Adding new line: '%s'", string(line[1:]))
					result = append(result, line[1:])
				}
			}
		}
		// Join with newlines and add final newline
		if len(result) > 0 {
			log.Printf("DEBUG: Creating new file with %d lines", len(result))
			return append(bytes.Join(result, []byte("\n")), '\n'), nil
		}
		log.Printf("DEBUG: Creating empty file")
		return []byte{}, nil
	}

	// Handle deletion of entire file
	if fileDiff.NewName == "/dev/null" {
		log.Printf("DEBUG: Handling file deletion")
		return []byte{}, nil
	}

	log.Printf("DEBUG: Original content (%d bytes):\n%s", len(originalContent), string(originalContent))
	originalLines := bytes.Split(originalContent, []byte("\n"))
	if len(originalContent) > 0 && !bytes.HasSuffix(originalContent, []byte("\n")) {
		log.Printf("DEBUG: Original content doesn't end with newline, appending empty line")
		originalLines = append(originalLines, []byte{})
	}

	// Track if we should preserve trailing newline
	preserveTrailingNewline := len(originalContent) > 0 && bytes.HasSuffix(originalContent, []byte("\n"))
	log.Printf("DEBUG: Preserve trailing newline: %v", preserveTrailingNewline)

	// Apply each hunk
	var result [][]byte
	currentLine := 0

	for hunkIdx, hunk := range fileDiff.Hunks {
		log.Printf("DEBUG: Processing hunk %d: @@ -%d,%d +%d,%d @@",
			hunkIdx, hunk.OrigStartLine, hunk.OrigLines, hunk.NewStartLine, hunk.NewLines)

		// Add lines before the hunk
		for ; currentLine < int(hunk.OrigStartLine-1); currentLine++ {
			if currentLine < len(originalLines) {
				log.Printf("DEBUG: Copying pre-hunk line %d: '%s'", currentLine+1, string(originalLines[currentLine]))
				result = append(result, originalLines[currentLine])
			}
		}

		// Process the hunk
		origIdx := 0
		addIdx := 0
		hunkLines := bytes.Split(hunk.Body, []byte("\n"))
		for lineIdx, line := range hunkLines {
			if len(line) == 0 && lineIdx == len(hunkLines)-1 {
				// Skip empty line at end of hunk
				continue
			}

			log.Printf("DEBUG: Processing hunk line %d: '%s'", lineIdx+1, string(line))
			if len(line) == 0 {
				// Empty line in middle of hunk
				result = append(result, []byte{})
				continue
			}

			switch line[0] {
			case ' ':
				// Context line - verify it matches
				if currentLine >= len(originalLines) {
					log.Printf("DEBUG: Context line error: EOF at line %d", currentLine+1)
					return nil, fmt.Errorf("context mismatch: expected '%s', got end of file at line %d", string(line[1:]), currentLine+1)
				}
				originalLine := bytes.TrimRight(originalLines[currentLine], "\n\r")
				patchLine := bytes.TrimRight(line[1:], "\n\r")
				log.Printf("DEBUG: Comparing context - Original: '%s', Patch: '%s'", string(originalLine), string(patchLine))
				if !bytes.Equal(originalLine, patchLine) {
					log.Printf("DEBUG: Context mismatch at line %d", currentLine+1)
					return nil, fmt.Errorf("context mismatch: expected '%s', got '%s' at original line %d", string(patchLine), string(originalLine), currentLine+1)
				}
				result = append(result, originalLines[currentLine])
				currentLine++
				origIdx++
				addIdx++
			case '-':
				// Deletion - verify it matches
				if currentLine >= len(originalLines) {
					log.Printf("DEBUG: Deletion line error: EOF at line %d", currentLine+1)
					return nil, fmt.Errorf("context mismatch: expected removal of '%s', got end of file at line %d", string(line[1:]), currentLine+1)
				}
				originalLine := bytes.TrimRight(originalLines[currentLine], "\n\r")
				patchLine := bytes.TrimRight(line[1:], "\n\r")
				log.Printf("DEBUG: Comparing deletion - Original: '%s', Patch: '%s'", string(originalLine), string(patchLine))
				if !bytes.Equal(originalLine, patchLine) {
					log.Printf("DEBUG: Deletion mismatch at line %d", currentLine+1)
					return nil, fmt.Errorf("context mismatch: expected removal of '%s', got '%s' at original line %d", string(patchLine), string(originalLine), currentLine+1)
				}
				currentLine++
				origIdx++
			case '+':
				// Addition
				log.Printf("DEBUG: Adding new line: '%s'", string(line[1:]))
				result = append(result, line[1:])
				addIdx++
			}
		}
	}

	// Add remaining lines after last hunk
	for ; currentLine < len(originalLines)-1 || (currentLine == len(originalLines)-1 && len(originalLines[currentLine]) > 0); currentLine++ {
		log.Printf("DEBUG: Copying post-hunk line %d: '%s'", currentLine+1, string(originalLines[currentLine]))
		result = append(result, originalLines[currentLine])
	}

	// Join lines with newlines
	if len(result) == 0 {
		return []byte{}, nil
	}

	output := bytes.Join(result, []byte("\n"))
	if preserveTrailingNewline || (len(fileDiff.Hunks) > 0 && bytes.HasSuffix(fileDiff.Hunks[len(fileDiff.Hunks)-1].Body, []byte("\n"))) {
		output = append(output, '\n')
	}

	log.Printf("DEBUG: Final output (%d bytes):\n%s", len(output), string(output))
	return output, nil
}

// splitLines splits byte content into lines, handling both \n and \r\n endings.
// It removes the line endings from the resulting strings.
func splitLines(content []byte) []string {
	content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	trimmedContent := bytes.TrimSuffix(content, []byte("\n"))

	if len(trimmedContent) == 0 && len(content) > 0 {
		return []string{""}
	}
	if len(content) == 0 {
		return []string{}
	}

	lines := bytes.Split(trimmedContent, []byte("\n"))
	result := make([]string, len(lines))
	for i, line := range lines {
		result[i] = string(line)
	}
	return result
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
