package fileutils

import (
	"errors"
	"path/filepath"
)

// ResolvePath takes a file path and working directory, and returns the absolute path.
// If the input path is already absolute, it returns it unchanged.
// If the input path is relative and workingDir is not empty, it joins them.
// If the input path is relative and workingDir is empty, it returns the relative path.
func ResolvePath(filePath, workingDir string) string {
	if filepath.IsAbs(filePath) {
		return filePath
	}

	if workingDir == "" {
		return filePath
	}

	return filepath.Join(workingDir, filePath)
}

// ResolveFilePath is a shared utility function for resolving file paths.
// It takes a file path and working directory and returns the resolved path.
func ResolveFilePath(filePath string, workingDir string) (string, error) {
	if filePath == "" {
		return "", errors.New("file path cannot be empty")
	}
	return ResolvePath(filePath, workingDir), nil
}
