// Package task provides functionality for executing various types of tasks,
// including file patching operations. The package handles the application of
// unified diff patches to files while maintaining proper error handling,
// cancellation support, and logging.
package task

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/sourcegraph/go-diff/diff"
)

// --- Error Messages ---

const (
	// Command validation errors
	errEmptyFilePath = "file path cannot be empty for PATCH_FILE"

	// File operation errors
	errReadFileFailed  = "failed to read original file %s"
	errStatFileFailed  = "failed to stat original file %s before writing patch"
	errWriteFileFailed = "failed to write patched content to file %s"

	// Status messages
	msgEmptyPatch       = "Empty patch provided. No changes applied to file: %s"
	msgCancelledBefore  = "File patching cancelled before start for %s."
	msgCancelledWriting = "File patching cancelled before writing to %s."
	msgSuccess          = "Successfully applied patch to %s in %s."
	msgFailedParse      = "Failed to parse patch content for file %s"
	msgFailedContext    = "Patch context mismatch for file %s"
	msgFailedMultiFile  = "Patch contained multiple file diffs (unsupported) for %s"

	// DefaultFilePermissions is the default file mode for new files (rw-r--r--)
	DefaultFilePermissions = 0644
)

// --- Error Types ---

// PatchError represents an error that occurred during patch application.
// It wraps the original error with additional context about the file being patched
// and a user-friendly message.
type PatchError struct {
	Err        error
	FilePath   string
	LineNumber int
	Details    string
	Message    string
}

// Error returns a formatted error message combining the context and original error.
func (e *PatchError) Error() string {
	return fmt.Sprintf("%s: %v", e.Message, e.Err)
}

// Unwrap returns the original error for error inspection.
func (e *PatchError) Unwrap() error {
	return e.Err
}

var (
	// errParseFailed indicates the patch content could not be parsed.
	errParseFailed = errors.New("failed to parse patch")
	// errMultiFilePatch indicates the provided patch contains diffs for more than one file.
	errMultiFilePatch = errors.New("patch contains multiple file diffs, only single file patches are supported")
	// errNoFilePatch indicates the parsed patch did not contain any file diffs.
	errNoFilePatch = errors.New("failed to parse patch: no valid hunks found")
	// errHunkMismatch indicates a hunk could not be applied because the context lines didn't match the original content.
	errHunkMismatch = errors.New("hunk context does not match original content")

	// bufferPool is a sync.Pool for reusing byte buffers during patch operations
	bufferPool = sync.Pool{
		New: func() interface{} {
			// Default buffer size of 4KB
			return bytes.NewBuffer(make([]byte, 0, 4096))
		},
	}
)

// --- Patching Logic ---

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

// prepareOriginalLines splits the original content into lines, using buffer pooling
func prepareOriginalLines(originalContent []byte) [][]byte {
	if len(originalContent) == 0 {
		return [][]byte{}
	}

	// Estimate the number of lines to pre-allocate the slice
	estimatedLines := bytes.Count(originalContent, []byte{'\n'}) + 1
	lines := make([][]byte, 0, estimatedLines)

	// Get a buffer from the pool
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufferPool.Put(buf)

	start := 0
	for i := 0; i < len(originalContent); i++ {
		if originalContent[i] == '\n' {
			// Reuse buffer for the line
			buf.Reset()
			buf.Write(originalContent[start:i])
			line := make([]byte, buf.Len())
			copy(line, buf.Bytes())
			lines = append(lines, line)
			start = i + 1
		}
	}

	// Handle the last line if it doesn't end with a newline
	if start < len(originalContent) {
		buf.Reset()
		buf.Write(originalContent[start:])
		line := make([]byte, buf.Len())
		copy(line, buf.Bytes())
		lines = append(lines, line)
	}

	// Add empty line if content ends with newline
	if len(originalContent) > 0 && originalContent[len(originalContent)-1] == '\n' {
		lines = append(lines, []byte{})
	}

	return lines
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

// formatFinalOutput joins the result lines efficiently using buffer pooling
func formatFinalOutput(result [][]byte, fileDiff *diff.FileDiff, preserveTrailingNewline bool) ([]byte, error) {
	if len(result) == 0 {
		return []byte{}, nil
	}

	// Get a buffer from the pool
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufferPool.Put(buf)

	// Calculate total capacity needed
	totalSize := 0
	for _, line := range result {
		totalSize += len(line) + 1 // +1 for newline
	}
	if !preserveTrailingNewline {
		totalSize-- // Adjust if we don't need a trailing newline
	}

	// Pre-grow the buffer
	buf.Grow(totalSize)

	// Write lines efficiently
	for i, line := range result {
		if i > 0 {
			buf.WriteByte('\n')
		}
		buf.Write(line)
	}

	// Add final newline if needed
	if preserveTrailingNewline ||
		(len(fileDiff.Hunks) > 0 && bytes.HasSuffix(fileDiff.Hunks[len(fileDiff.Hunks)-1].Body, []byte("\n"))) {
		buf.WriteByte('\n')
	}

	// Create the final result
	output := make([]byte, buf.Len())
	copy(output, buf.Bytes())
	return output, nil
}

// --- Interfaces ---

// FileSystem defines the interface for file system operations.
// This allows for easier testing and dependency injection.
type FileSystem interface {
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm os.FileMode) error
	Stat(name string) (os.FileInfo, error)
	LockFile(name string) (func(), error)
}

// Patcher defines the interface for applying patches.
// This allows for easier testing and dependency injection.
type Patcher interface {
	ApplyPatch(originalContent []byte, patchContent []byte) ([]byte, error)
}

// --- Default Implementations ---

// defaultFileSystem implements FileSystem using the standard os package.
type defaultFileSystem struct {
	fileLocks sync.Map // Map of file paths to mutexes
}

func (fs *defaultFileSystem) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func (fs *defaultFileSystem) WriteFile(name string, data []byte, perm os.FileMode) error {
	// Ensure the directory exists before writing the file
	dir := filepath.Dir(name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	return os.WriteFile(name, data, perm)
}

func (fs *defaultFileSystem) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (fs *defaultFileSystem) LockFile(name string) (func(), error) {
	// Get or create a mutex for this file
	lockKey := filepath.Clean(name)
	lockValue, _ := fs.fileLocks.LoadOrStore(lockKey, &sync.Mutex{})
	mutex := lockValue.(*sync.Mutex)

	// Lock the mutex
	mutex.Lock()

	// Return an unlock function
	return func() {
		mutex.Unlock()
	}, nil
}

// defaultPatcher implements Patcher using the internal applyPatch function.
type defaultPatcher struct{}

func (p *defaultPatcher) ApplyPatch(originalContent []byte, patchContent []byte) ([]byte, error) {
	return applyPatch(originalContent, patchContent)
}

// --- Executor Implementation ---

// PatchFileExecutor handles the execution of PatchFileCommand.
type PatchFileExecutor struct {
	fs      FileSystem
	patcher Patcher
}

// NewPatchFileExecutor creates a new PatchFileExecutor instance.
func NewPatchFileExecutor() *PatchFileExecutor {
	return &PatchFileExecutor{
		fs:      &defaultFileSystem{},
		patcher: &defaultPatcher{},
	}
}

// --- Helper Functions ---

// formatResult creates an OutputResult with the given parameters.
func formatResult(cmd *Task, status TaskStatus, message string, err error) OutputResult {
	var errMsg string
	if err != nil {
		errMsg = err.Error()
	}

	return OutputResult{
		TaskID:  cmd.TaskId,
		Status:  status,
		Message: message,
		Error:   errMsg,
	}
}

// --- Executor Methods ---

// Execute applies a patch to the file specified in the PatchFileCommand.
func (e *PatchFileExecutor) Execute(ctx context.Context, cmd any) (<-chan OutputResult, error) {
	// Create a channel for results
	results := make(chan OutputResult, 1)

	// Validate command type
	var patchCmd *Task
	switch c := cmd.(type) {
	case *Task:
		patchCmd = c
	default:
		return nil, fmt.Errorf("invalid command type: expected *PatchFileTask, got %T", cmd)
	}

	if patchCmd.Type != TaskPatchFile {
		return nil, fmt.Errorf("invalid command type: expected *PatchFileTask, got %T", cmd)
	}

	// Check if task is already in a terminal state
	terminalChan, err := HandleTerminalTask(patchCmd.TaskId, patchCmd.Status, patchCmd.Output)
	if err != nil {
		return nil, err
	}
	if terminalChan != nil {
		return terminalChan, nil
	}

	// Validate file path
	if patchCmd.Parameters.(PatchFileParameters).FilePath == "" {
		return nil, errors.New(errEmptyFilePath)
	}

	// Run the execution in a goroutine
	go func() {
		defer close(results)

		// Check context before each operation
		if err := ctx.Err(); err != nil {
			finalResult := formatResult(patchCmd, StatusFailed, "File patching cancelled.", err)
			patchCmd.Status = finalResult.Status
			patchCmd.UpdateOutput(&finalResult)
			results <- finalResult
			return
		}

		// Lock the file for exclusive access
		unlock, err := e.fs.LockFile(patchCmd.Parameters.(PatchFileParameters).FilePath)
		if err != nil {
			finalResult := formatResult(patchCmd, StatusFailed, fmt.Sprintf("Failed to lock file: %v", err), err)
			patchCmd.Status = finalResult.Status
			patchCmd.UpdateOutput(&finalResult)
			results <- finalResult
			return
		}
		defer unlock()

		// Read original file
		originalContent, err := e.readOriginalFile(patchCmd.Parameters.(PatchFileParameters).FilePath)
		if err != nil {
			finalResult := formatResult(patchCmd, StatusFailed, fmt.Sprintf("Failed to read original file: %v", err), err)
			patchCmd.Status = finalResult.Status
			patchCmd.UpdateOutput(&finalResult)
			results <- finalResult
			return
		}

		// Check context before applying patch
		if err := ctx.Err(); err != nil {
			finalResult := formatResult(patchCmd, StatusFailed, "File patching cancelled before applying patch.", err)
			patchCmd.Status = finalResult.Status
			patchCmd.UpdateOutput(&finalResult)
			results <- finalResult
			return
		}

		// Apply patch
		patchedContent, err := e.applyPatch(originalContent, []byte(patchCmd.Parameters.(PatchFileParameters).Patch))
		if err != nil {
			finalResult := formatResult(patchCmd, StatusFailed, fmt.Sprintf("Failed to apply patch: %v", err), err)
			patchCmd.Status = finalResult.Status
			patchCmd.UpdateOutput(&finalResult)
			results <- finalResult
			return
		}

		// Check context before writing file
		if err := ctx.Err(); err != nil {
			finalResult := formatResult(patchCmd, StatusFailed, "File patching cancelled before writing to file.", err)
			patchCmd.Status = finalResult.Status
			patchCmd.UpdateOutput(&finalResult)
			results <- finalResult
			return
		}

		// Write patched file
		if err := e.writePatchedFile(patchCmd.Parameters.(PatchFileParameters).FilePath, patchedContent); err != nil {
			finalResult := formatResult(patchCmd, StatusFailed, fmt.Sprintf("Failed to write patched file: %v", err), err)
			patchCmd.Status = finalResult.Status
			patchCmd.UpdateOutput(&finalResult)
			results <- finalResult
			return
		}

		// Send success result
		finalResult := formatResult(patchCmd, StatusSucceeded, fmt.Sprintf("Successfully patched file %s", patchCmd.Parameters.(PatchFileParameters).FilePath), nil)
		patchCmd.Status = finalResult.Status
		patchCmd.UpdateOutput(&finalResult)
		results <- finalResult
	}()

	return results, nil
}

// --- File Operations ---

// fileExists checks if a file exists and returns its size if it does.
// Returns (exists bool, size int64, err error)
func (e *PatchFileExecutor) fileExists(filePath string) (bool, int64, error) {
	fileInfo, err := e.fs.Stat(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, 0, nil
		}
		return false, 0, fmt.Errorf("failed to check file existence for %s: %w", filePath, err)
	}
	return true, fileInfo.Size(), nil
}

// readOriginalFile reads the original file content.
// If the file doesn't exist, it returns an empty byte slice.
// Returns an error if there was a problem reading the file.
func (e *PatchFileExecutor) readOriginalFile(filePath string) ([]byte, error) {
	exists, _, err := e.fileExists(filePath)
	if err != nil {
		return nil, err
	}

	if !exists {
		return []byte{}, nil
	}

	originalContent, err := e.fs.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	return originalContent, nil
}

// writePatchedFile writes the patched content back to the file.
func (e *PatchFileExecutor) writePatchedFile(filePath string, patchedContent []byte) error {
	perm, err := e.getFilePermissions(filePath)
	if err != nil {
		return fmt.Errorf(errStatFileFailed, filePath)
	}

	if err := e.fs.WriteFile(filePath, patchedContent, perm); err != nil {
		return err
	}

	return nil
}

// getFilePermissions retrieves the file permissions for the given path.
// If the file exists, it returns the current permissions.
// If the file doesn't exist, it returns DefaultFilePermissions.
// Returns an error if there was a problem accessing the file.
func (e *PatchFileExecutor) getFilePermissions(filePath string) (os.FileMode, error) {
	fileInfo, err := e.fs.Stat(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultFilePermissions, nil
		}
		return 0, fmt.Errorf("failed to get file permissions for %s: %w", filePath, err)
	}
	return fileInfo.Mode().Perm(), nil
}

// --- Patch Operations ---

// applyPatch applies the patch to the original content.
func (e *PatchFileExecutor) applyPatch(originalContent []byte, patchContent []byte) ([]byte, error) {
	patchedContent, err := e.patcher.ApplyPatch(originalContent, patchContent)
	if err != nil {
		return nil, e.mapPatchError(err, string(originalContent))
	}
	return patchedContent, nil
}

// mapPatchError maps specific patcher errors to more user-friendly messages.
// It extracts line numbers and details from errors when available and wraps them
// with additional context for better debugging.
func (e *PatchFileExecutor) mapPatchError(err error, filePath string) error {
	var (
		lineNumber int
		details    string
		patchErr   *PatchError
	)

	// Extract line number and details if available
	if errors.As(err, &patchErr) {
		lineNumber = patchErr.LineNumber
		details = patchErr.Details
		err = patchErr.Err // Use the underlying error for type checking
	}

	// Extract line number from diff.ParseError if available
	var parseErr *diff.ParseError
	if errors.As(err, &parseErr) {
		lineNumber = parseErr.Line
		details = fmt.Sprintf("parse error at line %d", parseErr.Line)
	}

	// Build error details string
	detailsStr := ""
	if details != "" {
		detailsStr = fmt.Sprintf(" (%s)", details)
	}
	if lineNumber > 0 {
		detailsStr = fmt.Sprintf("%s Line number: %d.", detailsStr, lineNumber)
	}

	switch {
	case errors.Is(err, errParseFailed):
		return &PatchError{
			Err:        fmt.Errorf("%w: %v", errParseFailed, err),
			FilePath:   filePath,
			LineNumber: lineNumber,
			Details:    details,
			Message:    fmt.Sprintf(msgFailedParse, filePath) + detailsStr,
		}
	case errors.Is(err, errHunkMismatch):
		return &PatchError{
			Err:        fmt.Errorf("%w: %v", errHunkMismatch, err),
			FilePath:   filePath,
			LineNumber: lineNumber,
			Details:    details,
			Message:    fmt.Sprintf(msgFailedContext, filePath) + detailsStr,
		}
	case errors.Is(err, errMultiFilePatch):
		return &PatchError{
			Err:        fmt.Errorf("%w: %v", errMultiFilePatch, err),
			FilePath:   filePath,
			LineNumber: lineNumber,
			Details:    details,
			Message:    fmt.Sprintf(msgFailedMultiFile, filePath) + detailsStr,
		}
	case errors.Is(err, errNoFilePatch):
		return &PatchError{
			Err:        fmt.Errorf("%w: %v", errNoFilePatch, err),
			FilePath:   filePath,
			LineNumber: lineNumber,
			Details:    details,
			Message:    fmt.Sprintf("No valid patch hunks found for file %s.%s", filePath, detailsStr),
		}
	}

	// For unknown errors, wrap with additional context
	return &PatchError{
		Err:        fmt.Errorf("failed to apply patch: %w", err),
		FilePath:   filePath,
		LineNumber: lineNumber,
		Details:    details,
		Message:    fmt.Sprintf("Failed to apply patch to file %s.%s", filePath, detailsStr),
	}
}
