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
		return originalContent, nil // Applying empty patch is a no-op
	}

	fileDiffs, err := diff.ParseMultiFileDiff(patchContent)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errParseFailed, err)
	}

	if len(fileDiffs) == 0 {
		return nil, errNoFilePatch
	}

	if len(fileDiffs) > 1 {
		return nil, errMultiFilePatch
	}

	fileDiff := fileDiffs[0]
	originalLines := splitLines(originalContent)
	// Estimate capacity: original lines + net added lines (approximated)
	addedLinesEstimate := 0
	for _, h := range fileDiff.Hunks {
		addedLinesEstimate += int(h.NewLines) - int(h.OrigLines)
	}
	patchedLines := make([]string, 0, len(originalLines)+addedLinesEstimate)

	originalLineIdx := 0 // 0-based index for originalLines

	for _, hunk := range fileDiff.Hunks {
		// Hunk line numbers are 1-based, but parser might return 0 for new files.
		hunkStartLine := int(hunk.OrigStartLine) // 1-based start line in original

		// Handle @@ -0,0 @@ case for new files: Treat start line as 1 for indexing.
		if hunkStartLine == 0 && hunk.OrigLines == 0 {
			hunkStartLine = 1
		}

		if hunkStartLine > len(originalLines)+1 {
			return nil, fmt.Errorf("%w: hunk %d starts at line %d, original only has %d lines", errLineOutOfBounds, hunk.OrigStartLine, hunkStartLine, len(originalLines))
		}

		// 1. Copy lines from original untouched before the hunk
		linesToCopy := hunkStartLine - 1 - originalLineIdx
		if linesToCopy < 0 {
			return nil, fmt.Errorf("invalid hunk state: original index %d, hunk starts at %d", originalLineIdx, hunkStartLine)
		}
		if linesToCopy > 0 {
			patchedLines = append(patchedLines, originalLines[originalLineIdx:originalLineIdx+linesToCopy]...)
			originalLineIdx += linesToCopy
		}

		// 2. Process the hunk body
		hunkBodyLines := splitLines(hunk.Body)
		expectedOriginalLineCountInHunk := 0 // Count context and delete lines

		for _, line := range hunkBodyLines {
			if len(line) == 0 {
				continue
			}

			switch line[0] {
			case ' ': // Context line
				if originalLineIdx >= len(originalLines) {
					return nil, fmt.Errorf("%w: context line refers to line %d, but original only has %d lines", errHunkMismatch, originalLineIdx+1, len(originalLines))
				}
				if line[1:] != originalLines[originalLineIdx] {
					return nil, fmt.Errorf("%w: expected '%s', got '%s' at original line %d", errHunkMismatch, line[1:], originalLines[originalLineIdx], originalLineIdx+1)
				}
				patchedLines = append(patchedLines, originalLines[originalLineIdx])
				originalLineIdx++
				expectedOriginalLineCountInHunk++
			case '-': // Deletion line
				if originalLineIdx >= len(originalLines) {
					return nil, fmt.Errorf("%w: deletion line refers to line %d, but original only has %d lines", errHunkMismatch, originalLineIdx+1, len(originalLines))
				}
				if line[1:] != originalLines[originalLineIdx] {
					return nil, fmt.Errorf("%w: expected removal of '%s', got '%s' at original line %d", errHunkMismatch, line[1:], originalLines[originalLineIdx], originalLineIdx+1)
				}
				originalLineIdx++
				expectedOriginalLineCountInHunk++
			case '+': // Addition line
				patchedLines = append(patchedLines, line[1:])
			case '\\': // "\ No newline at end of file" marker - Ignore
				continue
			default:
				return nil, fmt.Errorf("invalid line prefix in hunk body: %q", line)
			}
		}

		// 3. Sanity check hunk metadata vs processed lines (optional but good practice)
		expectedEndIdx := hunkStartLine - 1 + expectedOriginalLineCountInHunk
		if originalLineIdx != expectedEndIdx {
			return nil, fmt.Errorf("internal error: index mismatch after processing hunk. Expected %d, got %d", expectedEndIdx, originalLineIdx)
		}
	}

	// 4. Copy remaining lines from original
	if originalLineIdx < len(originalLines) {
		patchedLines = append(patchedLines, originalLines[originalLineIdx:]...)
	}

	// 5. Join lines back using newline
	return []byte(strings.Join(patchedLines, "\n")), nil
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
