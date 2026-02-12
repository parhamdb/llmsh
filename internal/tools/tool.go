package tools

import (
	"context"
	"encoding/json"

	"github.com/parhamdb/llmsh/internal/llm"
)

// Tool represents an executable capability exposed to the LLM.
type Tool interface {
	// Name returns the unique identifier for the tool (e.g., "bash").
	Name() string

	// Description returns a human-readable explanation of what the tool does.
	Description() string

	// Definition returns the JSON Schema for the tool's arguments.
	// This is used by the LLM Provider to inform the model how to call the tool.
	Definition() llm.ToolDefinition

	// Execute performs the tool's action using the provided arguments.
	// args is the raw JSON bytes provided by the LLM.
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}
