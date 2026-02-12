package builder

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/parhamdb/llmsh/internal/llm"
)

// Builder implements the --new mode for guided .llmsh file creation.
type Builder struct {
	provider llm.Provider
}

// NewBuilder creates a new script builder.
func NewBuilder(provider llm.Provider) *Builder {
	return &Builder{provider: provider}
}

const builderSystemPrompt = `You are an assistant that helps users create .llmsh script files.
A .llmsh file has the format:
1. Optional shebang: #!/usr/bin/env llmsh
2. YAML frontmatter between --- delimiters containing: name, description, model, provider, tools, args, permissions, max_turns, temperature
3. A markdown body that serves as the system prompt for the AI agent

When the user describes what they want, generate a complete .llmsh file.
Output ONLY the file content, nothing else. Start with the shebang line.

Available tools: bash, read_file, write_file, list_files, http_request, env
Available permissions: read, write, execute`

// Run starts the interactive builder conversation.
func (b *Builder) Run(ctx context.Context, outputFile string) error {
	reader := bufio.NewReader(os.Stdin)

	if outputFile == "" {
		fmt.Print("Output filename (e.g., sort.llmsh): ")
		name, _ := reader.ReadString('\n')
		outputFile = strings.TrimSpace(name)
		if outputFile == "" {
			return fmt.Errorf("filename is required")
		}
	}
	if !strings.HasSuffix(outputFile, ".llmsh") {
		outputFile += ".llmsh"
	}

	fmt.Println("What should this script do?")
	fmt.Print("> ")
	description, _ := reader.ReadString('\n')
	description = strings.TrimSpace(description)

	if description == "" {
		return fmt.Errorf("description is required")
	}

	fmt.Println("What tools will it need? (bash, read_file, write_file, list_files, http_request, env)")
	fmt.Print("> ")
	toolsInput, _ := reader.ReadString('\n')
	toolsInput = strings.TrimSpace(toolsInput)

	// Build the generation prompt
	prompt := fmt.Sprintf("Create a .llmsh script file with these requirements:\n\nDescription: %s\n", description)
	if toolsInput != "" {
		prompt += fmt.Sprintf("Tools needed: %s\n", toolsInput)
	}
	prompt += fmt.Sprintf("\nThe script name should be derived from the filename: %s", strings.TrimSuffix(outputFile, ".llmsh"))

	messages := []llm.Message{
		{
			Role: llm.RoleUser,
			Content: []llm.ContentPart{
				{Type: llm.ContentPartText, Text: prompt},
			},
		},
	}

	// Generate the script
	fmt.Println("\nGenerating script...")

	resp, err := b.provider.Generate(ctx, messages, nil)
	if err != nil {
		return fmt.Errorf("generation failed: %w", err)
	}

	content := extractText(resp)
	if content == "" {
		return fmt.Errorf("empty response from LLM")
	}

	// Show the generated file
	fmt.Printf("\nGenerated %s:\n", outputFile)
	fmt.Println("─────────────────────────────")
	fmt.Println(content)
	fmt.Println("─────────────────────────────")

	// Confirm save
	fmt.Printf("\nSave to %s? [Y/n] ", outputFile)
	confirm, _ := reader.ReadString('\n')
	confirm = strings.TrimSpace(strings.ToLower(confirm))

	if confirm != "" && confirm != "y" && confirm != "yes" {
		fmt.Println("Cancelled.")
		return nil
	}

	if err := os.WriteFile(outputFile, []byte(content), 0755); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("Saved to %s\n", outputFile)
	return nil
}

func extractText(msg *llm.Message) string {
	var parts []string
	for _, part := range msg.Content {
		if part.Type == llm.ContentPartText && part.Text != "" {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "")
}
