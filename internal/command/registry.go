package command

import (
	"fmt"
	"sync"
)

// CommandRegistry defines the interface for retrieving the appropriate executor for a given command type.
// This acts as a factory or lookup mechanism for CommandExecutor instances.
type CommandRegistry interface {
	// GetExecutor returns the specific CommandExecutor capable of handling the provided CommandType.
	// It returns an error if no executor is registered for the given type.
	GetExecutor(cmdType CommandType) (CommandExecutor, error)
}

// MapRegistry provides a map-based implementation of the CommandRegistry interface.
// It stores CommandExecutors keyed by their corresponding CommandType.
// It is safe for concurrent use.
type MapRegistry struct {
	mu        sync.RWMutex
	executors map[CommandType]CommandExecutor
}

// NewMapRegistry creates and returns a new, initialized MapRegistry.
func NewMapRegistry() *MapRegistry {
	return &MapRegistry{
		executors: make(map[CommandType]CommandExecutor),
	}
}

// Register associates a CommandExecutor with a specific CommandType.
// If an executor is already registered for the given type, it will be overwritten.
func (r *MapRegistry) Register(cmdType CommandType, executor CommandExecutor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executors[cmdType] = executor
}

// GetExecutor retrieves the CommandExecutor registered for the given CommandType.
// It returns the executor and a nil error if found.
// If no executor is registered for the type, it returns nil and an error.
func (r *MapRegistry) GetExecutor(cmdType CommandType) (CommandExecutor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	executor, ok := r.executors[cmdType]
	if !ok {
		return nil, fmt.Errorf("no executor registered for command type: %s", cmdType)
	}
	return executor, nil
}
