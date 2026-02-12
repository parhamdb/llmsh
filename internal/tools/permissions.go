package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/parhamdb/llmsh/internal/llm"
)

// PermissionGuard wraps a tool registry and enforces permission checks.
// Denied calls return error text to the LLM (not fatal) so it can adapt.
type PermissionGuard struct {
	inner       *Registry
	permissions map[string]bool // e.g. {"read": true, "write": true, "execute": true}
}

// toolPermissions maps tool names to their required permission.
var toolPermissions = map[string]string{
	"bash":         "execute",
	"read_file":    "read",
	"write_file":   "write",
	"list_files":   "read",
	"http_request": "execute",
	"env":          "read",
}

// NewPermissionGuard wraps a registry with permission enforcement.
func NewPermissionGuard(registry *Registry, allowedPermissions []string) *PermissionGuard {
	perms := make(map[string]bool)
	for _, p := range allowedPermissions {
		perms[p] = true
	}
	return &PermissionGuard{
		inner:       registry,
		permissions: perms,
	}
}

func (pg *PermissionGuard) Get(name string) Tool {
	return pg.inner.Get(name)
}

func (pg *PermissionGuard) ListDefinitions() []llm.ToolDefinition {
	return pg.inner.ListDefinitions()
}

// ExecuteTool checks permissions before executing.
func (pg *PermissionGuard) ExecuteTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	requiredPerm, known := toolPermissions[name]
	if known && !pg.permissions[requiredPerm] {
		// Return permission denied as a tool result (not a fatal error)
		return fmt.Sprintf("Permission denied: tool '%s' requires '%s' permission, which is not granted by this script's permissions configuration.", name, requiredPerm), nil
	}
	return pg.inner.ExecuteTool(ctx, name, args)
}
