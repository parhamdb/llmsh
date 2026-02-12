package parser

import (
	"bufio"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// Arg defines a script argument.
type Arg struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
	Default     string `yaml:"default,omitempty"`
}

// Frontmatter defines the configuration structure found in .llmsh files.
type Frontmatter struct {
	Name        string            `yaml:"name,omitempty"`
	Description string            `yaml:"description,omitempty"`
	Model       string            `yaml:"model,omitempty"`
	Provider    string            `yaml:"provider,omitempty"`
	Tools       []string          `yaml:"tools,omitempty"`
	Args        []Arg             `yaml:"args,omitempty"`
	Permissions []string          `yaml:"permissions,omitempty"`
	MaxTurns    int               `yaml:"max_turns,omitempty"`
	Temperature *float64          `yaml:"temperature,omitempty"`
	Env         map[string]string `yaml:"env,omitempty"`
}

// Script represents a parsed .llmsh file.
type Script struct {
	Shebang     string
	Frontmatter Frontmatter
	Body        string
}

// Parse reads an .llmsh file and splits it into shebang, YAML frontmatter, and body.
func Parse(r io.Reader) (*Script, error) {
	scanner := bufio.NewScanner(r)
	script := &Script{}

	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(lines) == 0 {
		return script, nil
	}

	// 1. Check for shebang
	startIdx := 0
	if strings.HasPrefix(lines[0], "#!") {
		script.Shebang = lines[0]
		startIdx = 1
	}

	// 2. Check for YAML frontmatter (--- delimited)
	if startIdx < len(lines) && strings.TrimSpace(lines[startIdx]) == "---" {
		fmLines := []string{}
		fmEndIdx := -1

		for i := startIdx + 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				fmEndIdx = i
				break
			}
			fmLines = append(fmLines, lines[i])
		}

		if fmEndIdx != -1 {
			fmData := strings.Join(fmLines, "\n")
			if err := yaml.Unmarshal([]byte(fmData), &script.Frontmatter); err != nil {
				return nil, err
			}
			startIdx = fmEndIdx + 1
		}
	}

	// 3. The rest is the body (prompt template)
	if startIdx < len(lines) {
		body := strings.Join(lines[startIdx:], "\n")
		script.Body = strings.TrimSpace(body)
	}

	return script, nil
}
