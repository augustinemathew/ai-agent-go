package task

import (
	"fmt"
	"sync"
)

// TaskRegistry defines the interface for retrieving the appropriate executor for a given command type.
// This acts as a factory or lookup mechanism for TaskExecutor instances.
type TaskRegistry interface {
	// GetExecutor returns the specific TaskExecutor capable of handling the provided TaskType.
	// It returns an error if no executor is registered for the given type.
	GetExecutor(cmdType TaskType) (TaskExecutor, error)
}

// MapRegistry provides a map-based implementation of the TaskRegistry interface.
// It stores TaskExecutors keyed by their corresponding TaskType.
// It is safe for concurrent use.
type MapRegistry struct {
	mu        sync.RWMutex
	executors map[TaskType]TaskExecutor
}

// NewMapRegistry creates and returns a new MapRegistry, automatically registering
// all known standard task executors.
func NewMapRegistry() *MapRegistry {
	r := &MapRegistry{
		executors: make(map[TaskType]TaskExecutor),
	}

	// Register all known executors automatically
	r.Register(TaskBashExec, NewBashExecExecutor())
	r.Register(TaskFileRead, NewFileReadExecutor()) // Consider if buffer size needs configuration
	r.Register(TaskFileWrite, NewFileWriteExecutor())
	r.Register(TaskPatchFile, NewPatchFileExecutor())
	r.Register(TaskListDirectory, NewListDirectoryExecutor())
	r.Register(TaskRequestUserInput, NewRequestUserInputExecutor())

	// Register the GroupExecutor which needs the registry itself
	r.Register(TaskGroup, NewGroupExecutor(r))

	// Add future executors here...

	return r
}

// Register associates a CommandExecutor with a specific CommandType.
// If an executor is already registered for the given type, it will be overwritten.
// This is kept public in case users want to override or add custom executors.
func (r *MapRegistry) Register(cmdType TaskType, executor TaskExecutor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executors[cmdType] = executor
}

// GetExecutor retrieves the CommandExecutor registered for the given CommandType.
// It returns the executor and a nil error if found.
// If no executor is registered for the type, it returns nil and an error.
func (r *MapRegistry) GetExecutor(cmdType TaskType) (TaskExecutor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	executor, ok := r.executors[cmdType]
	if !ok {
		return nil, fmt.Errorf("no executor registered for command type: %s", cmdType)
	}
	return executor, nil
}
