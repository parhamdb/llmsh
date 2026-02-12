package config

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/parhamdb/llmsh/internal/parser"
	"gopkg.in/yaml.v3"
)

// Config represents the runtime configuration for llmsh.
type Config struct {
	Provider    string            `yaml:"provider"`
	Model       string            `yaml:"model"`
	Temperature float64           `yaml:"temperature"`
	MaxTurns    int               `yaml:"max_turns"`
	System      string            `yaml:"-"` // Set from script body, not config files
	Tools       []string          `yaml:"tools"`
	Permissions []string          `yaml:"permissions"`
	Env         map[string]string `yaml:"env"`

	// API keys (loaded from config or env)
	AnthropicAPIKey string `yaml:"anthropic_api_key,omitempty"`
	OpenAIAPIKey    string `yaml:"openai_api_key,omitempty"`
	OllamaHost      string `yaml:"ollama_host,omitempty"`
}

// CLIConfig holds values parsed from command line flags.
type CLIConfig struct {
	Model       *string
	Provider    *string
	Temperature *float64
	MaxTurns    *int
	Verbose     bool
	Quiet       bool
	DryRun      bool
}

// DefaultConfig returns hardcoded defaults.
func DefaultConfig() Config {
	return Config{
		Provider:    "anthropic",
		Model:       "claude-sonnet-4-5-20250929",
		Temperature: 0.0,
		MaxTurns:    10,
		Tools:       []string{"bash", "read_file", "write_file", "list_files"},
		Permissions: []string{"read", "write", "execute"},
		Env:         make(map[string]string),
		OllamaHost:  "http://localhost:11434",
	}
}

// Load merges configuration from all sources.
// Precedence: CLI flags > env vars > frontmatter > ~/.llmsh/config.yaml > /etc/llmsh/config.yaml > defaults
func Load(cli CLIConfig, fm parser.Frontmatter) (*Config, error) {
	cfg := DefaultConfig()

	// System-wide config
	if data, err := os.ReadFile("/etc/llmsh/config.yaml"); err == nil {
		var sysCfg Config
		if err := yaml.Unmarshal(data, &sysCfg); err == nil {
			merge(&cfg, sysCfg)
		}
	}

	// User config
	if home, err := os.UserHomeDir(); err == nil {
		configPath := filepath.Join(home, ".llmsh", "config.yaml")
		if data, err := os.ReadFile(configPath); err == nil {
			var userCfg Config
			if err := yaml.Unmarshal(data, &userCfg); err == nil {
				merge(&cfg, userCfg)
			}
		}
	}

	// Frontmatter
	if fm.Provider != "" {
		cfg.Provider = fm.Provider
	}
	if fm.Model != "" {
		cfg.Model = fm.Model
	}
	if fm.Temperature != nil {
		cfg.Temperature = *fm.Temperature
	}
	if fm.MaxTurns > 0 {
		cfg.MaxTurns = fm.MaxTurns
	}
	if len(fm.Tools) > 0 {
		cfg.Tools = fm.Tools
	}
	if len(fm.Permissions) > 0 {
		cfg.Permissions = fm.Permissions
	}
	if len(fm.Env) > 0 {
		if cfg.Env == nil {
			cfg.Env = make(map[string]string)
		}
		for k, v := range fm.Env {
			cfg.Env[k] = v
		}
	}

	// Environment variables
	if val := os.Getenv("LLMSH_PROVIDER"); val != "" {
		cfg.Provider = val
	}
	if val := os.Getenv("LLMSH_MODEL"); val != "" {
		cfg.Model = val
	}
	if val := os.Getenv("LLMSH_MAX_TURNS"); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			cfg.MaxTurns = n
		}
	}
	if val := os.Getenv("ANTHROPIC_API_KEY"); val != "" {
		cfg.AnthropicAPIKey = val
	}
	if val := os.Getenv("OPENAI_API_KEY"); val != "" {
		cfg.OpenAIAPIKey = val
	}
	if val := os.Getenv("OLLAMA_HOST"); val != "" {
		cfg.OllamaHost = val
	}

	// CLI flags (highest precedence)
	if cli.Provider != nil {
		cfg.Provider = *cli.Provider
	}
	if cli.Model != nil {
		cfg.Model = *cli.Model
	}
	if cli.Temperature != nil {
		cfg.Temperature = *cli.Temperature
	}
	if cli.MaxTurns != nil {
		cfg.MaxTurns = *cli.MaxTurns
	}

	return &cfg, nil
}

// merge updates dst with non-zero values from src.
func merge(dst *Config, src Config) {
	if src.Provider != "" {
		dst.Provider = src.Provider
	}
	if src.Model != "" {
		dst.Model = src.Model
	}
	if src.Temperature != 0 {
		dst.Temperature = src.Temperature
	}
	if src.MaxTurns > 0 {
		dst.MaxTurns = src.MaxTurns
	}
	if len(src.Tools) > 0 {
		dst.Tools = src.Tools
	}
	if len(src.Permissions) > 0 {
		dst.Permissions = src.Permissions
	}
	if src.AnthropicAPIKey != "" {
		dst.AnthropicAPIKey = src.AnthropicAPIKey
	}
	if src.OpenAIAPIKey != "" {
		dst.OpenAIAPIKey = src.OpenAIAPIKey
	}
	if src.OllamaHost != "" {
		dst.OllamaHost = src.OllamaHost
	}
	if len(src.Env) > 0 {
		if dst.Env == nil {
			dst.Env = make(map[string]string)
		}
		for k, v := range src.Env {
			dst.Env[k] = v
		}
	}
}
