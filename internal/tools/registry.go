package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/parhamdb/llmsh/internal/llm"
)

// Executor defines an interface for executing tools by name.
// Both Registry and PermissionGuard implement this.
type Executor interface {
	ExecuteTool(ctx context.Context, name string, args json.RawMessage) (string, error)
	ListDefinitions() []llm.ToolDefinition
}

// Registry manages the set of available tools.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates a new, empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry.
// If a tool with the same name exists, it is overwritten.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

// Get retrieves a tool by name. Returns nil if not found.
func (r *Registry) Get(name string) Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[name]
}

// ListDefinitions returns the list of tool definitions for the LLM provider.
func (r *Registry) ListDefinitions() []llm.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]llm.ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, t.Definition())
	}
	return defs
}

// ExecuteTool finds a tool by name and executes it with the given arguments.
// It wraps error handling to return consistent error strings for the LLM.
func (r *Registry) ExecuteTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	tool := r.Get(name)
	if tool == nil {
		return "", fmt.Errorf("tool not found: %s", name)
	}

	output, err := tool.Execute(ctx, args)
	if err != nil {
		// We return the error as a string so it can be fed back to the LLM
		// as a ToolResult with IsError=true.
		return fmt.Sprintf("Error executing tool '%s': %v", name, err), nil
	}
	return output, nil
}
