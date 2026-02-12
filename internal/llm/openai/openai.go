package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	oai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
	"github.com/parhamdb/llmsh/internal/llm"
)

// Provider implements llm.Provider for OpenAI-compatible APIs.
type Provider struct {
	client      oai.Client
	model       string
	temperature float64
	system      string
	name        string // "openai" or "ollama"
}

type Option func(*Provider)

func WithModel(model string) Option       { return func(p *Provider) { p.model = model } }
func WithTemperature(t float64) Option     { return func(p *Provider) { p.temperature = t } }
func WithSystem(s string) Option           { return func(p *Provider) { p.system = s } }
func WithName(n string) Option             { return func(p *Provider) { p.name = n } }

// New creates a new OpenAI-compatible provider.
func New(apiKey string, opts ...Option) *Provider {
	var clientOpts []option.RequestOption
	if apiKey != "" {
		clientOpts = append(clientOpts, option.WithAPIKey(apiKey))
	}

	p := &Provider{
		client: oai.NewClient(clientOpts...),
		model:  "gpt-4o",
		name:   "openai",
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// NewOllama creates a provider configured for Ollama's OpenAI-compatible endpoint.
func NewOllama(host, model string, opts ...Option) *Provider {
	baseURL := strings.TrimRight(host, "/") + "/v1"

	p := &Provider{
		client: oai.NewClient(
			option.WithBaseURL(baseURL),
			option.WithAPIKey("ollama"), // Ollama doesn't require a real key
		),
		model: model,
		name:  "ollama",
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *Provider) Name() string { return p.name }

func (p *Provider) SetSystem(system string) { p.system = system }

func (p *Provider) Generate(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (*llm.Message, error) {
	params := p.buildParams(messages, tools)

	resp, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("%s API error: %w", p.name, err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("%s: no choices in response", p.name)
	}

	return convertResponse(&resp.Choices[0].Message), nil
}

func (p *Provider) Stream(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (<-chan llm.StreamEvent, error) {
	params := p.buildParams(messages, tools)

	stream := p.client.Chat.Completions.NewStreaming(ctx, params)
	ch := make(chan llm.StreamEvent, 64)

	go func() {
		defer close(ch)

		acc := oai.ChatCompletionAccumulator{}

		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)

			// Stream text deltas
			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta
				if delta.Content != "" {
					ch <- llm.StreamEvent{
						Type:      llm.StreamEventTextDelta,
						TextDelta: delta.Content,
					}
				}
			}

			// Check for completed tool calls
			if tool, ok := acc.JustFinishedToolCall(); ok {
				ch <- llm.StreamEvent{
					Type: llm.StreamEventToolCall,
					ToolCall: &llm.ToolCall{
						ID:   tool.ID,
						Name: tool.Name,
						Args: json.RawMessage(tool.Arguments),
					},
				}
			}
		}

		if stream.Err() != nil {
			ch <- llm.StreamEvent{Type: llm.StreamEventError, Error: stream.Err()}
			return
		}

		ch <- llm.StreamEvent{Type: llm.StreamEventDone}
	}()

	return ch, nil
}

func (p *Provider) buildParams(messages []llm.Message, tools []llm.ToolDefinition) oai.ChatCompletionNewParams {
	params := oai.ChatCompletionNewParams{
		Model:    oai.ChatModel(p.model),
		Messages: convertMessages(messages, p.system),
	}

	if p.temperature > 0 {
		params.Temperature = oai.Float(p.temperature)
	}

	if len(tools) > 0 {
		params.Tools = convertTools(tools)
	}

	return params
}

func convertMessages(msgs []llm.Message, system string) []oai.ChatCompletionMessageParamUnion {
	var out []oai.ChatCompletionMessageParamUnion

	// Add system message if present
	if system != "" {
		out = append(out, oai.SystemMessage(system))
	}

	for _, msg := range msgs {
		switch msg.Role {
		case llm.RoleSystem:
			for _, part := range msg.Content {
				if part.Type == llm.ContentPartText {
					out = append(out, oai.SystemMessage(part.Text))
				}
			}

		case llm.RoleUser:
			// Check if this contains tool results
			hasToolResults := false
			for _, part := range msg.Content {
				if part.Type == llm.ContentPartToolResult {
					hasToolResults = true
					break
				}
			}

			if hasToolResults {
				for _, part := range msg.Content {
					if part.Type == llm.ContentPartToolResult && part.ToolResult != nil {
						out = append(out, oai.ToolMessage(part.ToolResult.Output, part.ToolResult.ToolCallID))
					}
				}
			} else {
				text := extractTextFromParts(msg.Content)
				if text != "" {
					out = append(out, oai.UserMessage(text))
				}
			}

		case llm.RoleAssistant:
			// Build assistant message with possible tool calls
			text := extractTextFromParts(msg.Content)
			var toolCalls []oai.ChatCompletionMessageToolCallParam

			for _, part := range msg.Content {
				if part.Type == llm.ContentPartToolCall && part.ToolCall != nil {
					toolCalls = append(toolCalls, oai.ChatCompletionMessageToolCallParam{
						ID: part.ToolCall.ID,
						Function: oai.ChatCompletionMessageToolCallFunctionParam{
							Name:      part.ToolCall.Name,
							Arguments: string(part.ToolCall.Args),
						},
					})
				}
			}

			if len(toolCalls) > 0 {
				assistantMsg := oai.ChatCompletionAssistantMessageParam{
					ToolCalls: toolCalls,
				}
				if text != "" {
					assistantMsg.Content = oai.ChatCompletionAssistantMessageParamContentUnion{
						OfString: oai.String(text),
					}
				}
				out = append(out, oai.ChatCompletionMessageParamUnion{
					OfAssistant: &assistantMsg,
				})
			} else if text != "" {
				out = append(out, oai.AssistantMessage(text))
			}

		case llm.RoleTool:
			for _, part := range msg.Content {
				if part.Type == llm.ContentPartToolResult && part.ToolResult != nil {
					out = append(out, oai.ToolMessage(part.ToolResult.Output, part.ToolResult.ToolCallID))
				}
			}
		}
	}

	return out
}

func convertTools(defs []llm.ToolDefinition) []oai.ChatCompletionToolParam {
	var out []oai.ChatCompletionToolParam

	for _, def := range defs {
		params := shared.FunctionParameters{}
		json.Unmarshal(def.Schema, &params)

		out = append(out, oai.ChatCompletionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name:        def.Name,
				Description: oai.String(def.Description),
				Parameters:  params,
			},
		})
	}

	return out
}

func convertResponse(msg *oai.ChatCompletionMessage) *llm.Message {
	result := &llm.Message{Role: llm.RoleAssistant}

	if msg.Content != "" {
		result.Content = append(result.Content, llm.ContentPart{
			Type: llm.ContentPartText,
			Text: msg.Content,
		})
	}

	for _, tc := range msg.ToolCalls {
		result.Content = append(result.Content, llm.ContentPart{
			Type: llm.ContentPartToolCall,
			ToolCall: &llm.ToolCall{
				ID:   tc.ID,
				Name: tc.Function.Name,
				Args: json.RawMessage(tc.Function.Arguments),
			},
		})
	}

	return result
}

func extractTextFromParts(parts []llm.ContentPart) string {
	var texts []string
	for _, p := range parts {
		if p.Type == llm.ContentPartText && p.Text != "" {
			texts = append(texts, p.Text)
		}
	}
	return strings.Join(texts, "")
}
