package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/parhamdb/llmsh/internal/llm"
	"github.com/parhamdb/llmsh/internal/tools"
	"golang.org/x/sync/errgroup"
)

// Agent orchestrates the LLM ↔ tool execution loop.
type Agent struct {
	provider llm.Provider
	executor tools.Executor
	output   *OutputRouter
	maxTurns int
}

// New creates a new agent.
func New(provider llm.Provider, executor tools.Executor, output *OutputRouter, maxTurns int) *Agent {
	if maxTurns <= 0 {
		maxTurns = 10
	}
	return &Agent{
		provider: provider,
		executor: executor,
		output:   output,
		maxTurns: maxTurns,
	}
}

// Run executes the agent loop: send messages to LLM, handle tool calls, repeat.
// Returns the final text response.
func (a *Agent) Run(ctx context.Context, messages []llm.Message) (string, error) {
	toolDefs := a.executor.ListDefinitions()

	for turn := 0; turn < a.maxTurns; turn++ {
		a.output.Debug("turn %d/%d", turn+1, a.maxTurns)

		// Stream the response
		ch, err := a.provider.Stream(ctx, messages, toolDefs)
		if err != nil {
			return "", fmt.Errorf("provider error: %w", err)
		}

		// Collect streaming response
		response, err := a.collectStream(ch)
		if err != nil {
			return "", err
		}

		// Add assistant response to conversation
		messages = append(messages, *response)

		// Extract tool calls
		toolCalls := extractToolCalls(response)
		if len(toolCalls) == 0 {
			// No tool calls — we're done. Return the final text.
			return extractText(response), nil
		}

		// Execute tool calls (in parallel)
		results, err := a.executeTools(ctx, toolCalls)
		if err != nil {
			return "", fmt.Errorf("tool execution error: %w", err)
		}

		// Add tool results as a user message
		var parts []llm.ContentPart
		for _, result := range results {
			parts = append(parts, llm.ContentPart{
				Type:       llm.ContentPartToolResult,
				ToolResult: &result,
			})
		}
		messages = append(messages, llm.Message{
			Role:    llm.RoleUser,
			Content: parts,
		})
	}

	return "", fmt.Errorf("max turns (%d) exceeded", a.maxTurns)
}

// RunNonStreaming uses Generate instead of Stream (for providers without streaming).
func (a *Agent) RunNonStreaming(ctx context.Context, messages []llm.Message) (string, error) {
	toolDefs := a.executor.ListDefinitions()

	for turn := 0; turn < a.maxTurns; turn++ {
		a.output.Debug("turn %d/%d", turn+1, a.maxTurns)

		response, err := a.provider.Generate(ctx, messages, toolDefs)
		if err != nil {
			return "", fmt.Errorf("provider error: %w", err)
		}

		messages = append(messages, *response)

		// Stream text to stderr for visibility
		text := extractText(response)
		if text != "" {
			a.output.StreamText(text)
		}

		toolCalls := extractToolCalls(response)
		if len(toolCalls) == 0 {
			return text, nil
		}

		results, err := a.executeTools(ctx, toolCalls)
		if err != nil {
			return "", fmt.Errorf("tool execution error: %w", err)
		}

		var parts []llm.ContentPart
		for _, result := range results {
			parts = append(parts, llm.ContentPart{
				Type:       llm.ContentPartToolResult,
				ToolResult: &result,
			})
		}
		messages = append(messages, llm.Message{
			Role:    llm.RoleUser,
			Content: parts,
		})
	}

	return "", fmt.Errorf("max turns (%d) exceeded", a.maxTurns)
}

// collectStream reads all events from the stream and builds the response message.
func (a *Agent) collectStream(ch <-chan llm.StreamEvent) (*llm.Message, error) {
	msg := &llm.Message{Role: llm.RoleAssistant}
	var textParts []string

	for event := range ch {
		switch event.Type {
		case llm.StreamEventTextDelta:
			a.output.StreamText(event.TextDelta)
			textParts = append(textParts, event.TextDelta)

		case llm.StreamEventToolCall:
			if event.ToolCall != nil {
				msg.Content = append(msg.Content, llm.ContentPart{
					Type:     llm.ContentPartToolCall,
					ToolCall: event.ToolCall,
				})
			}

		case llm.StreamEventError:
			return nil, event.Error

		case llm.StreamEventDone:
			// done
		}
	}

	// Prepend collected text as the first content part
	if len(textParts) > 0 {
		fullText := strings.Join(textParts, "")
		msg.Content = append([]llm.ContentPart{{
			Type: llm.ContentPartText,
			Text: fullText,
		}}, msg.Content...)
	}

	// Add newline after streaming text for cleanliness
	if len(textParts) > 0 {
		a.output.StreamText("\n")
	}

	return msg, nil
}

// executeTools runs tool calls in parallel using errgroup.
func (a *Agent) executeTools(ctx context.Context, calls []llm.ToolCall) ([]llm.ToolResult, error) {
	results := make([]llm.ToolResult, len(calls))
	var mu sync.Mutex
	g, ctx := errgroup.WithContext(ctx)

	for i, call := range calls {
		i, call := i, call
		g.Go(func() error {
			a.output.ToolStart(call.Name)

			output, err := a.executor.ExecuteTool(ctx, call.Name, json.RawMessage(call.Args))

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				a.output.ToolError(call.Name, err.Error())
				results[i] = llm.ToolResult{
					ToolCallID: call.ID,
					Name:       call.Name,
					Output:     fmt.Sprintf("Error: %v", err),
					IsError:    true,
				}
			} else {
				a.output.ToolDone(call.Name)
				isError := strings.HasPrefix(output, "Error executing tool")
				results[i] = llm.ToolResult{
					ToolCallID: call.ID,
					Name:       call.Name,
					Output:     output,
					IsError:    isError,
				}
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}

func extractToolCalls(msg *llm.Message) []llm.ToolCall {
	var calls []llm.ToolCall
	for _, part := range msg.Content {
		if part.Type == llm.ContentPartToolCall && part.ToolCall != nil {
			calls = append(calls, *part.ToolCall)
		}
	}
	return calls
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
