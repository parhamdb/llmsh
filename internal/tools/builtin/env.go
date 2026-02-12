package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/parhamdb/llmsh/internal/llm"
)

// EnvTool provides access to environment variables.
type EnvTool struct{}

type envArgs struct {
	Name   string `json:"name,omitempty"`
	Action string `json:"action"` // "get", "list"
}

func (t *EnvTool) Name() string { return "env" }
func (t *EnvTool) Description() string {
	return "Get or list environment variables. Use action 'get' with a name to get a specific variable, or 'list' to list all."
}

func (t *EnvTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"action": { "type": "string", "enum": ["get", "list"], "description": "Action: get a specific var or list all" },
				"name": { "type": "string", "description": "Environment variable name (for 'get' action)" }
			},
			"required": ["action"]
		}`),
	}
}

func (t *EnvTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a envArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", err
	}

	switch a.Action {
	case "get":
		if a.Name == "" {
			return "", fmt.Errorf("name is required for 'get' action")
		}
		val, ok := os.LookupEnv(a.Name)
		if !ok {
			return fmt.Sprintf("Environment variable %s is not set", a.Name), nil
		}
		return val, nil
	case "list":
		env := os.Environ()
		var sb strings.Builder
		for _, e := range env {
			sb.WriteString(e)
			sb.WriteString("\n")
		}
		return sb.String(), nil
	default:
		return "", fmt.Errorf("unknown action: %s (use 'get' or 'list')", a.Action)
	}
}
