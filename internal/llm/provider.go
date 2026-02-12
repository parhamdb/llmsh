package llm

import (
	"context"
	"encoding/json"
)

// Role represents the sender of a message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is a unified message structure for all providers.
type Message struct {
	Role    Role
	Content []ContentPart
}

// ContentPartType identifies the type of content (text, tool call, tool result).
type ContentPartType string

const (
	ContentPartText       ContentPartType = "text"
	ContentPartToolCall   ContentPartType = "tool_call"
	ContentPartToolResult ContentPartType = "tool_result"
	ContentPartImage      ContentPartType = "image" // Future proofing
)

// ContentPart represents a distinct piece of content within a message.
// Providers like Anthropic treat messages as a list of blocks.
type ContentPart struct {
	Type ContentPartType

	// Text content. Used if Type == ContentPartText.
	Text string

	// ToolCall data. Used if Type == ContentPartToolCall.
	ToolCall *ToolCall

	// ToolResult data. Used if Type == ContentPartToolResult.
	ToolResult *ToolResult
}

// ToolCall represents a request from the LLM to execute a tool.
type ToolCall struct {
	ID   string          // Unique ID for the call (crucial for OpenAI/Anthropic mapping)
	Name string          // Name of the tool to execute
	Args json.RawMessage // JSON arguments for the tool
}

// ToolResult represents the output of a tool execution.
type ToolResult struct {
	ToolCallID string // Must match the ID of the initiating ToolCall
	Name       string // Name of the tool
	Output     string // The result string (stdout/stderr or error message)
	IsError    bool   // Hints if the output is an error message
}

// StreamEventType defines the kind of event occurring during streaming.
type StreamEventType string

const (
	StreamEventTextDelta StreamEventType = "text_delta" // Token stream
	StreamEventToolCall  StreamEventType = "tool_call"  // Tool call constructed (might be partial or complete depending on implementation)
	StreamEventError     StreamEventType = "error"
	StreamEventDone      StreamEventType = "done"
)

// StreamEvent is a single event emitted during a streaming generation.
type StreamEvent struct {
	Type StreamEventType

	// TextDelta is the chunk of text generated.
	TextDelta string

	// ToolCall is populated when a tool call is detected/completed.
	// For some providers, this might be built up incrementally or emitted at the end.
	ToolCall *ToolCall

	// Error is populated if an error occurs during the stream.
	Error error
}

// Provider defines the interface that all LLM backends must implement.
type Provider interface {
	// Name returns the provider name (e.g., "anthropic", "openai", "ollama").
	Name() string

	// SetSystem updates the system prompt for subsequent requests.
	SetSystem(system string)

	// Generate sends a request to the LLM and returns the complete response.
	Generate(ctx context.Context, messages []Message, tools []ToolDefinition) (*Message, error)

	// Stream sends a request to the LLM and streams the response via a channel.
	Stream(ctx context.Context, messages []Message, tools []ToolDefinition) (<-chan StreamEvent, error)
}

// ToolDefinition describes a tool to the LLM (Name, Description, Schema).
// This is the portable format that Provider implementations must convert to their specific API format.
type ToolDefinition struct {
	Name        string
	Description string
	Schema      json.RawMessage // JSON Schema describing the arguments
}
