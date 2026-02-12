package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/parhamdb/llmsh/internal/llm"
)

// BashTool executes shell commands.
type BashTool struct {
	mu  sync.Mutex
	cwd string // persistent working directory
}

type bashArgs struct {
	Command string `json:"command"`
}

func (t *BashTool) Name() string { return "bash" }

func (t *BashTool) Description() string {
	return "Execute a bash command. The working directory persists between calls (cd is tracked). Returns combined stdout+stderr."
}

func (t *BashTool) Definition() llm.ToolDefinition {
	schema := `{
  "type": "object",
  "properties": {
    "command": {
      "type": "string",
      "description": "The bash command to execute"
    }
  },
  "required": ["command"]
}`
	return llm.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Schema:      json.RawMessage(schema),
	}
}

// Cwd returns the current working directory of the bash tool.
func (t *BashTool) Cwd() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cwd == "" {
		t.cwd, _ = os.Getwd()
	}
	return t.cwd
}

// SetCwd explicitly sets the working directory.
func (t *BashTool) SetCwd(dir string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cwd = dir
}

func (t *BashTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a bashArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("invalid arguments: %v", err)
	}

	if a.Command == "" {
		return "", fmt.Errorf("command is empty")
	}

	t.mu.Lock()
	if t.cwd == "" {
		t.cwd, _ = os.Getwd()
	}
	cwd := t.cwd
	t.mu.Unlock()

	// Wrap the command: run in current cwd, then print the final cwd so we can track cd
	// The sentinel lets us extract the final directory after the command runs
	sentinel := "___LLMSH_CWD___"
	wrapped := fmt.Sprintf("cd %q && %s\n__exit_code=$?\necho\necho '%s'\npwd\nexit $__exit_code",
		cwd, a.Command, sentinel)

	cmd := exec.CommandContext(ctx, "bash", "-c", wrapped)
	cmd.Dir = cwd

	output, err := cmd.CombinedOutput()
	result := string(output)

	// Extract the final cwd from output
	if idx := strings.LastIndex(result, sentinel+"\n"); idx != -1 {
		afterSentinel := result[idx+len(sentinel)+1:]
		newCwd := strings.TrimSpace(afterSentinel)
		if newCwd != "" {
			t.mu.Lock()
			t.cwd = newCwd
			t.mu.Unlock()
		}
		// Remove the sentinel and pwd from visible output
		result = strings.TrimRight(result[:idx], "\n")
	}

	if err != nil {
		if result != "" {
			result += "\n"
		}
		result += fmt.Sprintf("exit status: %v", err)
	}

	return result, nil
}
