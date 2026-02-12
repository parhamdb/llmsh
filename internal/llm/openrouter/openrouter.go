package openrouter

import (
	oaiProvider "github.com/parhamdb/llmsh/internal/llm/openai"
)

// New creates a new OpenRouter provider using the OpenAI-compatible API.
func New(apiKey, model string, opts ...oaiProvider.Option) *oaiProvider.Provider {
	return oaiProvider.NewOpenRouter(apiKey, model, opts...)
}
