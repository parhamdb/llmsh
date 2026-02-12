package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/parhamdb/llmsh/internal/llm"
)

// ReadFileTool reads a file from the filesystem.
type ReadFileTool struct{}

type readFileArgs struct {
	Path string `json:"path"`
}

func (t *ReadFileTool) Name() string { return "read_file" }
func (t *ReadFileTool) Description() string { return "Read the contents of a file." }

func (t *ReadFileTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": { "type": "string", "description": "The path to the file to read" }
			},
			"required": ["path"]
		}`),
	}
}

func (t *ReadFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a readFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", err
	}
	content, err := os.ReadFile(a.Path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// WriteFileTool writes content to a file.
type WriteFileTool struct{}

type writeFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (t *WriteFileTool) Name() string { return "write_file" }
func (t *WriteFileTool) Description() string { return "Write content to a file. Overwrites existing files." }

func (t *WriteFileTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": { "type": "string", "description": "The path to the file to write" },
				"content": { "type": "string", "description": "The content to write" }
			},
			"required": ["path", "content"]
		}`),
	}
}

func (t *WriteFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a writeFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", err
	}
	
	// Create directory if it doesn't exist
	dir := filepath.Dir(a.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %v", err)
	}

	if err := os.WriteFile(a.Path, []byte(a.Content), 0644); err != nil {
		return "", err
	}
	return fmt.Sprintf("Successfully wrote to %s", a.Path), nil
}

// ListFilesTool lists files in a directory.
type ListFilesTool struct{}

type listFilesArgs struct {
	Path string `json:"path"`
}

func (t *ListFilesTool) Name() string { return "list_files" }
func (t *ListFilesTool) Description() string { return "List files and directories in a given path." }

func (t *ListFilesTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": { "type": "string", "description": "The directory path to list (defaults to current directory if empty)" }
			}
		}`),
	}
}

func (t *ListFilesTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a listFilesArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", err
	}
	
targetPath := a.Path
	if targetPath == "" {
		targetPath = "."
	}

	entries, err := os.ReadDir(targetPath)
	if err != nil {
		return "", err
	}

	var result string
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		prefix := "F"
		if entry.IsDir() {
			prefix = "D"
		}
		result += fmt.Sprintf("[%s] %s (%d bytes)\n", prefix, entry.Name(), info.Size())
	}
	return result, nil
}
