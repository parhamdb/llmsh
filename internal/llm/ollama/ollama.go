package ollama

import (
	oaiProvider "github.com/parhamdb/llmsh/internal/llm/openai"
)

// New creates a new Ollama provider using OpenAI-compatible API.
func New(host, model string, opts ...oaiProvider.Option) *oaiProvider.Provider {
	return oaiProvider.NewOllama(host, model, opts...)
}
