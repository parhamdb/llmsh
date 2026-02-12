package builder

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/parhamdb/llmsh/internal/llm"
)

// Editor implements the --edit mode for chat-based .llmsh file editing.
type Editor struct {
	provider llm.Provider
}

// NewEditor creates a new script editor.
func NewEditor(provider llm.Provider) *Editor {
	return &Editor{provider: provider}
}

const editorSystemPrompt = `You are an assistant that helps users edit .llmsh script files.
A .llmsh file has the format:
1. Optional shebang: #!/usr/bin/env llmsh
2. YAML frontmatter between --- delimiters
3. A markdown body that serves as the system prompt

The user will show you an existing .llmsh file and describe changes they want.
Output ONLY the complete modified file content, nothing else. Start with the shebang line.
Preserve the existing structure and only change what was requested.`

// Run starts the interactive editor.
func (e *Editor) Run(ctx context.Context, filePath string) error {
	// Read existing file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", filePath, err)
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("Loaded %s. What would you like to change?\n", filePath)
	fmt.Print("> ")
	changes, _ := reader.ReadString('\n')
	changes = strings.TrimSpace(changes)

	if changes == "" {
		return fmt.Errorf("no changes specified")
	}

	prompt := fmt.Sprintf("Here is the current .llmsh file:\n\n```\n%s\n```\n\nPlease make these changes:\n%s", string(content), changes)

	messages := []llm.Message{
		{
			Role: llm.RoleUser,
			Content: []llm.ContentPart{
				{Type: llm.ContentPartText, Text: prompt},
			},
		},
	}

	fmt.Println("\nApplying changes...")

	resp, err := e.provider.Generate(ctx, messages, nil)
	if err != nil {
		return fmt.Errorf("generation failed: %w", err)
	}

	newContent := extractText(resp)
	if newContent == "" {
		return fmt.Errorf("empty response from LLM")
	}

	// Show the result
	fmt.Printf("\nUpdated %s:\n", filePath)
	fmt.Println("─────────────────────────────")
	fmt.Println(newContent)
	fmt.Println("─────────────────────────────")

	// Confirm save
	fmt.Printf("\nSave changes to %s? [Y/n] ", filePath)
	confirm, _ := reader.ReadString('\n')
	confirm = strings.TrimSpace(strings.ToLower(confirm))

	if confirm != "" && confirm != "y" && confirm != "yes" {
		fmt.Println("Cancelled.")
		return nil
	}

	if err := os.WriteFile(filePath, []byte(newContent), 0755); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("Saved changes to %s\n", filePath)
	return nil
}
