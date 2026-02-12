package anthropic

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/parhamdb/llmsh/internal/llm"
)

// Provider implements llm.Provider for Anthropic Claude.
type Provider struct {
	client      anthropic.Client
	model       string
	temperature float64
	maxTokens   int64
	system      string
}

// Option configures the Anthropic provider.
type Option func(*Provider)

func WithModel(model string) Option       { return func(p *Provider) { p.model = model } }
func WithTemperature(t float64) Option     { return func(p *Provider) { p.temperature = t } }
func WithMaxTokens(n int64) Option         { return func(p *Provider) { p.maxTokens = n } }
func WithSystem(s string) Option           { return func(p *Provider) { p.system = s } }

// New creates a new Anthropic provider.
func New(apiKey string, opts ...Option) *Provider {
	var clientOpts []option.RequestOption
	if apiKey != "" {
		clientOpts = append(clientOpts, option.WithAPIKey(apiKey))
	}

	p := &Provider{
		client:    anthropic.NewClient(clientOpts...),
		model:     "claude-sonnet-4-5-20250929",
		maxTokens: 8192,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *Provider) Name() string { return "anthropic" }

func (p *Provider) SetSystem(system string) { p.system = system }

// Generate sends a non-streaming request and returns the full response.
func (p *Provider) Generate(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (*llm.Message, error) {
	params := p.buildParams(messages, tools)

	resp, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic API error: %w", err)
	}

	return convertResponse(resp), nil
}

// Stream sends a streaming request and returns a channel of events.
func (p *Provider) Stream(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (<-chan llm.StreamEvent, error) {
	params := p.buildParams(messages, tools)

	stream := p.client.Messages.NewStreaming(ctx, params)
	ch := make(chan llm.StreamEvent, 64)

	go func() {
		defer close(ch)

		accumulated := anthropic.Message{}

		for stream.Next() {
			event := stream.Current()
			if err := accumulated.Accumulate(event); err != nil {
				ch <- llm.StreamEvent{Type: llm.StreamEventError, Error: err}
				return
			}

			switch ev := event.AsAny().(type) {
			case anthropic.ContentBlockDeltaEvent:
				switch delta := ev.Delta.AsAny().(type) {
				case anthropic.TextDelta:
					ch <- llm.StreamEvent{
						Type:      llm.StreamEventTextDelta,
						TextDelta: delta.Text,
					}
				}
			case anthropic.MessageStopEvent:
				// Emit tool calls from accumulated message
				for _, block := range accumulated.Content {
					if tu, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
						ch <- llm.StreamEvent{
							Type: llm.StreamEventToolCall,
							ToolCall: &llm.ToolCall{
								ID:   tu.ID,
								Name: tu.Name,
								Args: json.RawMessage(tu.Input),
							},
						}
					}
				}
				ch <- llm.StreamEvent{Type: llm.StreamEventDone}
			}
		}

		if err := stream.Err(); err != nil {
			ch <- llm.StreamEvent{Type: llm.StreamEventError, Error: err}
		}
	}()

	return ch, nil
}

func (p *Provider) buildParams(messages []llm.Message, tools []llm.ToolDefinition) anthropic.MessageNewParams {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(p.model),
		MaxTokens: p.maxTokens,
		Messages:  convertMessages(messages),
	}

	if p.system != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: p.system},
		}
	}

	if p.temperature > 0 {
		params.Temperature = anthropic.Float(p.temperature)
	}

	if len(tools) > 0 {
		params.Tools = convertTools(tools)
	}

	return params
}

func convertMessages(msgs []llm.Message) []anthropic.MessageParam {
	var out []anthropic.MessageParam

	for _, msg := range msgs {
		if msg.Role == llm.RoleSystem {
			continue // System messages handled separately
		}

		var blocks []anthropic.ContentBlockParamUnion

		for _, part := range msg.Content {
			switch part.Type {
			case llm.ContentPartText:
				blocks = append(blocks, anthropic.NewTextBlock(part.Text))

			case llm.ContentPartToolCall:
				if part.ToolCall != nil {
					blocks = append(blocks, anthropic.ContentBlockParamUnion{
						OfToolUse: &anthropic.ToolUseBlockParam{
							ID:    part.ToolCall.ID,
							Name:  part.ToolCall.Name,
							Input: json.RawMessage(part.ToolCall.Args),
						},
					})
				}

			case llm.ContentPartToolResult:
				if part.ToolResult != nil {
					blocks = append(blocks,
						anthropic.NewToolResultBlock(
							part.ToolResult.ToolCallID,
							part.ToolResult.Output,
							part.ToolResult.IsError,
						),
					)
				}
			}
		}

		if len(blocks) == 0 {
			continue
		}

		switch msg.Role {
		case llm.RoleUser, llm.RoleTool:
			out = append(out, anthropic.NewUserMessage(blocks...))
		case llm.RoleAssistant:
			out = append(out, anthropic.NewAssistantMessage(blocks...))
		}
	}

	return out
}

func convertTools(defs []llm.ToolDefinition) []anthropic.ToolUnionParam {
	var out []anthropic.ToolUnionParam

	for _, def := range defs {
		var schema anthropic.ToolInputSchemaParam
		// Parse the JSON schema into the SDK's expected format
		var rawSchema map[string]any
		if err := json.Unmarshal(def.Schema, &rawSchema); err == nil {
			if props, ok := rawSchema["properties"]; ok {
				schema.Properties = props
			}
			if req, ok := rawSchema["required"].([]any); ok {
				for _, r := range req {
					if s, ok := r.(string); ok {
						schema.Required = append(schema.Required, s)
					}
				}
			}
		}

		out = append(out, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        def.Name,
				Description: anthropic.String(def.Description),
				InputSchema: schema,
			},
		})
	}

	return out
}

func convertResponse(resp *anthropic.Message) *llm.Message {
	msg := &llm.Message{
		Role: llm.RoleAssistant,
	}

	for _, block := range resp.Content {
		switch v := block.AsAny().(type) {
		case anthropic.TextBlock:
			msg.Content = append(msg.Content, llm.ContentPart{
				Type: llm.ContentPartText,
				Text: v.Text,
			})
		case anthropic.ToolUseBlock:
			msg.Content = append(msg.Content, llm.ContentPart{
				Type: llm.ContentPartToolCall,
				ToolCall: &llm.ToolCall{
					ID:   v.ID,
					Name: v.Name,
					Args: json.RawMessage(v.Input),
				},
			})
		}
	}

	return msg
}
