package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ModelSpec holds the raw model field which can be a single string or a list.
type ModelSpec struct {
	Entries []string
}

// IsEmpty returns true if no model entries are specified.
func (ms ModelSpec) IsEmpty() bool {
	return len(ms.Entries) == 0
}

// UnmarshalYAML implements yaml.Unmarshaler, accepting a string or a list of strings.
func (ms *ModelSpec) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		if value.Value != "" {
			ms.Entries = []string{value.Value}
		}
		return nil
	case yaml.SequenceNode:
		var items []string
		if err := value.Decode(&items); err != nil {
			return err
		}
		ms.Entries = items
		return nil
	default:
		return fmt.Errorf("model must be a string or list of strings")
	}
}

// MarshalYAML implements yaml.Marshaler.
func (ms ModelSpec) MarshalYAML() (interface{}, error) {
	if len(ms.Entries) == 1 {
		return ms.Entries[0], nil
	}
	return ms.Entries, nil
}

// ModelSpecFromInterface converts an interface{} (from YAML unmarshaling) to a ModelSpec.
// Handles string, []interface{}, and []string.
func ModelSpecFromInterface(v interface{}) ModelSpec {
	if v == nil {
		return ModelSpec{}
	}
	switch val := v.(type) {
	case string:
		if val != "" {
			return ModelSpec{Entries: []string{val}}
		}
	case []interface{}:
		entries := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok && s != "" {
				entries = append(entries, s)
			}
		}
		return ModelSpec{Entries: entries}
	case []string:
		return ModelSpec{Entries: val}
	}
	return ModelSpec{}
}

// parseModelEntry splits a "provider:model" entry into provider and model parts.
// If no prefix, returns ("", entry).
func parseModelEntry(entry string) (provider, model string) {
	idx := strings.IndexByte(entry, ':')
	if idx < 0 {
		return "", entry
	}
	// Only treat as prefix if the part before : looks like a provider name.
	// This avoids splitting on model names like "qwen/qwen-2.5:72b".
	prefix := entry[:idx]
	switch prefix {
	case "anthropic", "openai", "ollama", "openrouter":
		return prefix, entry[idx+1:]
	default:
		return "", entry
	}
}

// detectProvider auto-detects the provider from the model name.
func detectProvider(model string) string {
	switch {
	case strings.HasPrefix(model, "claude-"):
		return "anthropic"
	case strings.HasPrefix(model, "gpt-"),
		strings.HasPrefix(model, "o1-"),
		strings.HasPrefix(model, "o3-"),
		strings.HasPrefix(model, "o4-"):
		return "openai"
	default:
		return ""
	}
}

// providerConfigured checks if a provider has credentials/host configured.
func providerConfigured(name string, providers ProvidersConfig) bool {
	switch name {
	case "anthropic":
		return providers.Anthropic.APIKey != ""
	case "openai":
		return providers.OpenAI.APIKey != ""
	case "ollama":
		return providers.Ollama.Host != ""
	case "openrouter":
		return providers.OpenRouter.APIKey != ""
	default:
		return false
	}
}

// resolveModel iterates the ModelSpec entries and finds the first with a configured provider.
// It sets cfg.Provider and cfg.Model. If the spec is empty, it leaves them unchanged.
func resolveModel(cfg *Config) error {
	if cfg.RawModel.IsEmpty() {
		return nil
	}

	entries := cfg.RawModel.Entries

	// Single entry — use it directly (no fallback logic)
	if len(entries) == 1 {
		prov, model := parseModelEntry(entries[0])
		if prov == "" {
			prov = detectProvider(model)
		}
		if prov != "" {
			cfg.Provider = prov
		}
		cfg.Model = model
		return nil
	}

	// Multiple entries — fallback list: first with configured credentials wins
	for _, entry := range entries {
		prov, model := parseModelEntry(entry)
		if prov == "" {
			prov = detectProvider(model)
		}
		if prov == "" {
			prov = cfg.Provider // use default provider
		}
		if providerConfigured(prov, cfg.Providers) {
			cfg.Provider = prov
			cfg.Model = model
			return nil
		}
	}

	return fmt.Errorf("no configured provider found for any model in fallback list: %v", entries)
}
