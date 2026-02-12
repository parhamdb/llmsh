package template

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/parhamdb/llmsh/internal/parser"
)

// TemplateData holds the data available to templates.
type TemplateData struct {
	Args    map[string]string // Named args from frontmatter
	ArgList []string          // Positional args
}

// Render processes the script body as a Go text/template with the given arguments.
// If the body contains no template directives, args are appended as a structured block.
func Render(body string, argDefs []parser.Arg, args []string) (string, error) {
	data := buildTemplateData(argDefs, args)

	// Check if body uses templates
	if strings.Contains(body, "{{") {
		tmpl, err := template.New("prompt").Parse(body)
		if err != nil {
			return "", fmt.Errorf("template parse error: %w", err)
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return "", fmt.Errorf("template render error: %w", err)
		}
		return buf.String(), nil
	}

	// No template directives: append args as a structured block
	if len(args) == 0 {
		return body, nil
	}

	var sb strings.Builder
	sb.WriteString(body)
	sb.WriteString("\n\n## Arguments\n")

	// If we have named args, use those
	if len(argDefs) > 0 {
		for name, val := range data.Args {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", name, val))
		}
	} else {
		// Positional args
		for i, arg := range args {
			sb.WriteString(fmt.Sprintf("- arg[%d]: %s\n", i, arg))
		}
	}

	return sb.String(), nil
}

func buildTemplateData(argDefs []parser.Arg, args []string) TemplateData {
	data := TemplateData{
		Args:    make(map[string]string),
		ArgList: args,
	}

	for i, def := range argDefs {
		if i < len(args) {
			data.Args[def.Name] = args[i]
		} else if def.Default != "" {
			data.Args[def.Name] = def.Default
		}
	}

	return data
}
