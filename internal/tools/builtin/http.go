package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/parhamdb/llmsh/internal/llm"
)

// HTTPRequestTool performs HTTP requests.
type HTTPRequestTool struct{}

type httpRequestArgs struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Body    string            `json:"body,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

func (t *HTTPRequestTool) Name() string { return "http_request" }
func (t *HTTPRequestTool) Description() string { return "Perform an HTTP request." }

func (t *HTTPRequestTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"method": { "type": "string", "enum": ["GET", "POST", "PUT", "DELETE", "PATCH", "HEAD"], "description": "HTTP method" },
				"url": { "type": "string", "description": "The URL to request" },
				"body": { "type": "string", "description": "Request body for POST/PUT/PATCH" },
				"headers": { "type": "object", "additionalProperties": { "type": "string" }, "description": "HTTP headers" }
			},
			"required": ["method", "url"]
		}`),
	}
}

func (t *HTTPRequestTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a httpRequestArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(a.Method), a.URL, strings.NewReader(a.Body))
	if err != nil {
		return "", err
	}

	for k, v := range a.Headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body failed: %v", err)
	}

	result := fmt.Sprintf("Status: %s\n\n%s", resp.Status, string(bodyBytes))
	return result, nil
}
