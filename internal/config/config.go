package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/parhamdb/llmsh/internal/parser"
	"gopkg.in/yaml.v3"
)

// Config represents the runtime configuration for llmsh.
type Config struct {
	Provider    string            `yaml:"provider"`
	Model       string            `yaml:"-"`     // resolved at load time
	RawModel    ModelSpec         `yaml:"model"`  // string or list from YAML
	Temperature float64           `yaml:"temperature"`
	MaxTurns    int               `yaml:"max_turns"`
	System      string            `yaml:"-"`
	Tools       []string          `yaml:"tools"`
	Permissions []string          `yaml:"permissions"`
	Env         map[string]string `yaml:"env"`
	Providers   ProvidersConfig   `yaml:"providers"`
}

// ProvidersConfig holds nested credentials for each provider.
type ProvidersConfig struct {
	Anthropic  ProviderCreds `yaml:"anthropic"`
	OpenAI     ProviderCreds `yaml:"openai"`
	Ollama     OllamaCreds   `yaml:"ollama"`
	OpenRouter ProviderCreds `yaml:"openrouter"`
}

// ProviderCreds holds API key and optional base URL for a provider.
type ProviderCreds struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url,omitempty"`
}

// OllamaCreds holds Ollama-specific configuration.
type OllamaCreds struct {
	Host string `yaml:"host"`
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
	ShowConfig  bool
	ConfigFile  string
}

// DefaultConfig returns hardcoded defaults.
func DefaultConfig() Config {
	return Config{
		Provider:    "anthropic",
		Model:       "claude-sonnet-4-5-20250929",
		RawModel:    ModelSpec{Entries: []string{"claude-sonnet-4-5-20250929"}},
		Temperature: 0.0,
		MaxTurns:    10,
		Tools:       []string{"bash", "read_file", "write_file", "list_files"},
		Permissions: []string{"read", "write", "execute"},
		Env:         make(map[string]string),
		Providers: ProvidersConfig{
			Ollama: OllamaCreds{Host: "http://localhost:11434"},
		},
	}
}

// Load merges configuration from all sources.
// Precedence (lowest → highest):
//
//	defaults → /etc/llmsh → user config → credentials.yaml → project config →
//	.env files → frontmatter → env vars → CLI flags
func Load(cli CLIConfig, fm parser.Frontmatter) (*Config, error) {
	cfg := DefaultConfig()

	if cli.ConfigFile != "" {
		// Custom config file replaces system + user config
		loadYAMLFile(cli.ConfigFile, &cfg)
	} else {
		// System config
		loadYAMLFile("/etc/llmsh/config.yaml", &cfg)

		// User config (XDG-aware)
		if path := UserConfigPath(); path != "" {
			loadYAMLFile(path, &cfg)
		}
	}

	// Credentials file (nested providers)
	loadCredentials(&cfg)

	// Project config (CWD only)
	loadYAMLFile(".llmsh/config.yaml", &cfg)

	// .env files
	cwd, _ := os.Getwd()
	if cwd != "" {
		applyDotenv(&cfg, LoadDotenvFiles(cwd))
	}

	// Frontmatter
	applyFrontmatter(&cfg, fm)

	// Environment variables
	applyEnvVars(&cfg)

	// CLI flags (highest precedence)
	applyCLIFlags(&cfg, cli)

	// Resolve model spec → cfg.Provider + cfg.Model
	if err := resolveModel(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// loadYAMLFile reads a YAML file and merges non-zero values into dst.
func loadYAMLFile(path string, dst *Config) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var src Config
	if err := yaml.Unmarshal(data, &src); err != nil {
		return
	}
	merge(dst, src)
}

// loadCredentials loads credentials.yaml and merges into cfg.Providers.
func loadCredentials(cfg *Config) {
	path := CredentialsPath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	// credentials.yaml has a top-level "providers:" key wrapping the same struct
	var creds struct {
		Providers ProvidersConfig `yaml:"providers"`
	}
	if err := yaml.Unmarshal(data, &creds); err != nil {
		return
	}
	mergeProviders(&cfg.Providers, creds.Providers)
}

// applyDotenv injects .env vars into os.Setenv and recognized vars into config.
func applyDotenv(cfg *Config, vars map[string]string) {
	for k, v := range vars {
		// Only set in environment if not already set (real env wins)
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}

	// Apply recognized config vars from .env (only if not overridden by real env)
	applyDotenvVar(vars, "LLMSH_PROVIDER", func(v string) { cfg.Provider = v })
	applyDotenvVar(vars, "LLMSH_MODEL", func(v string) {
		cfg.RawModel = ModelSpec{Entries: []string{v}}
	})
	applyDotenvVar(vars, "LLMSH_TEMPERATURE", func(v string) {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Temperature = f
		}
	})
	applyDotenvVar(vars, "LLMSH_MAX_TURNS", func(v string) {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MaxTurns = n
		}
	})

	// Provider credential vars from .env
	applyDotenvVar(vars, "ANTHROPIC_API_KEY", func(v string) { cfg.Providers.Anthropic.APIKey = v })
	applyDotenvVar(vars, "OPENAI_API_KEY", func(v string) { cfg.Providers.OpenAI.APIKey = v })
	applyDotenvVar(vars, "OLLAMA_HOST", func(v string) { cfg.Providers.Ollama.Host = v })
	applyDotenvVar(vars, "OPENROUTER_API_KEY", func(v string) { cfg.Providers.OpenRouter.APIKey = v })
}

// applyDotenvVar calls fn with the .env value only if not already set in real env.
func applyDotenvVar(vars map[string]string, key string, fn func(string)) {
	if v, ok := vars[key]; ok && os.Getenv(key) == vars[key] {
		// Value came from .env (we set it), real env didn't override
		fn(v)
	}
}

// applyFrontmatter applies script frontmatter overrides.
func applyFrontmatter(cfg *Config, fm parser.Frontmatter) {
	if fm.Provider != "" {
		cfg.Provider = fm.Provider
	}
	if fm.Model != nil {
		spec := ModelSpecFromInterface(fm.Model)
		if !spec.IsEmpty() {
			cfg.RawModel = spec
		}
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
}

// applyEnvVars applies environment variable overrides.
func applyEnvVars(cfg *Config) {
	if val := os.Getenv("LLMSH_PROVIDER"); val != "" {
		cfg.Provider = val
	}
	if val := os.Getenv("LLMSH_MODEL"); val != "" {
		cfg.RawModel = ModelSpec{Entries: []string{val}}
	}
	if val := os.Getenv("LLMSH_TEMPERATURE"); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			cfg.Temperature = f
		}
	}
	if val := os.Getenv("LLMSH_MAX_TURNS"); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			cfg.MaxTurns = n
		}
	}

	// Provider credentials from env
	if val := os.Getenv("ANTHROPIC_API_KEY"); val != "" {
		cfg.Providers.Anthropic.APIKey = val
	}
	if val := os.Getenv("OPENAI_API_KEY"); val != "" {
		cfg.Providers.OpenAI.APIKey = val
	}
	if val := os.Getenv("OLLAMA_HOST"); val != "" {
		cfg.Providers.Ollama.Host = val
	}
	if val := os.Getenv("OPENROUTER_API_KEY"); val != "" {
		cfg.Providers.OpenRouter.APIKey = val
	}
}

// applyCLIFlags applies CLI flag overrides (highest precedence).
func applyCLIFlags(cfg *Config, cli CLIConfig) {
	if cli.Provider != nil {
		cfg.Provider = *cli.Provider
	}
	if cli.Model != nil {
		cfg.RawModel = ModelSpec{Entries: []string{*cli.Model}}
	}
	if cli.Temperature != nil {
		cfg.Temperature = *cli.Temperature
	}
	if cli.MaxTurns != nil {
		cfg.MaxTurns = *cli.MaxTurns
	}
}

// ShowConfig returns a pretty-printed config string with redacted keys.
func ShowConfig(cfg *Config) string {
	var b strings.Builder

	fmt.Fprintf(&b, "provider:    %s\n", cfg.Provider)
	fmt.Fprintf(&b, "model:       %s\n", cfg.Model)
	fmt.Fprintf(&b, "temperature: %.1f\n", cfg.Temperature)
	fmt.Fprintf(&b, "max_turns:   %d\n", cfg.MaxTurns)
	fmt.Fprintf(&b, "tools:       %v\n", cfg.Tools)
	fmt.Fprintf(&b, "permissions: %v\n", cfg.Permissions)

	b.WriteString("\nproviders:\n")
	if cfg.Providers.Anthropic.APIKey != "" {
		fmt.Fprintf(&b, "  anthropic:\n    api_key: %s\n", redact(cfg.Providers.Anthropic.APIKey))
	}
	if cfg.Providers.OpenAI.APIKey != "" {
		fmt.Fprintf(&b, "  openai:\n    api_key: %s\n", redact(cfg.Providers.OpenAI.APIKey))
	}
	if cfg.Providers.Ollama.Host != "" {
		fmt.Fprintf(&b, "  ollama:\n    host: %s\n", cfg.Providers.Ollama.Host)
	}
	if cfg.Providers.OpenRouter.APIKey != "" {
		fmt.Fprintf(&b, "  openrouter:\n    api_key: %s\n", redact(cfg.Providers.OpenRouter.APIKey))
		if cfg.Providers.OpenRouter.BaseURL != "" {
			fmt.Fprintf(&b, "    base_url: %s\n", cfg.Providers.OpenRouter.BaseURL)
		}
	}

	if len(cfg.Env) > 0 {
		b.WriteString("\nenv:\n")
		for k, v := range cfg.Env {
			fmt.Fprintf(&b, "  %s: %s\n", k, v)
		}
	}

	return b.String()
}

// redact returns a redacted version of a secret key, showing prefix and last 4 chars.
func redact(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "..." + key[len(key)-4:]
}

// merge updates dst with non-zero values from src.
func merge(dst *Config, src Config) {
	if src.Provider != "" {
		dst.Provider = src.Provider
	}
	if !src.RawModel.IsEmpty() {
		dst.RawModel = src.RawModel
	}
	if src.Temperature != 0 {
		dst.Temperature = src.Temperature
	}
	if src.MaxTurns > 0 {
		dst.MaxTurns = src.MaxTurns
	}
	if len(src.Tools) > 0 {
		dst.Tools = src.Tools // REPLACE semantics
	}
	if len(src.Permissions) > 0 {
		dst.Permissions = src.Permissions // REPLACE semantics
	}
	if len(src.Env) > 0 {
		if dst.Env == nil {
			dst.Env = make(map[string]string)
		}
		for k, v := range src.Env {
			dst.Env[k] = v // Merge-by-key
		}
	}
	mergeProviders(&dst.Providers, src.Providers)
}

// mergeProviders merges non-empty provider credential fields from src into dst.
func mergeProviders(dst *ProvidersConfig, src ProvidersConfig) {
	if src.Anthropic.APIKey != "" {
		dst.Anthropic.APIKey = src.Anthropic.APIKey
	}
	if src.Anthropic.BaseURL != "" {
		dst.Anthropic.BaseURL = src.Anthropic.BaseURL
	}
	if src.OpenAI.APIKey != "" {
		dst.OpenAI.APIKey = src.OpenAI.APIKey
	}
	if src.OpenAI.BaseURL != "" {
		dst.OpenAI.BaseURL = src.OpenAI.BaseURL
	}
	if src.Ollama.Host != "" {
		dst.Ollama.Host = src.Ollama.Host
	}
	if src.OpenRouter.APIKey != "" {
		dst.OpenRouter.APIKey = src.OpenRouter.APIKey
	}
	if src.OpenRouter.BaseURL != "" {
		dst.OpenRouter.BaseURL = src.OpenRouter.BaseURL
	}
}
