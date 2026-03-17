package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
)

const (
	maxFetchSize = 1024 * 1024 // 1MB
	fetchTimeout = 30 * time.Second
)

type WebFetchTool struct{}

func NewWebFetchTool() *WebFetchTool {
	return &WebFetchTool{}
}

func (t *WebFetchTool) Name() string { return "web_fetch" }

func (t *WebFetchTool) Description() string {
	return "Fetch content from a URL, optionally converting HTML to markdown."
}

func (t *WebFetchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {
				"type": "string",
				"description": "URL to fetch."
			},
			"format": {
				"type": "string",
				"enum": ["markdown", "text", "html"],
				"description": "Output format (default: markdown)."
			}
		},
		"required": ["url"]
	}`)
}

func (t *WebFetchTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		URL    string `json:"url"`
		Format string `json:"format"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("parse params: %w", err)
	}

	if p.Format == "" {
		p.Format = "markdown"
	}

	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Cyclaw/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", p.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d for %s", resp.StatusCode, p.URL)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchSize))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	content := string(body)
	if content == "" {
		return "", fmt.Errorf("empty content from %s", p.URL)
	}

	switch p.Format {
	case "html":
		return content, nil
	case "text":
		return content, nil
	case "markdown":
		contentType := resp.Header.Get("Content-Type")
		if strings.Contains(contentType, "text/html") {
			md, err := htmltomarkdown.ConvertString(content)
			if err != nil {
				// Fall back to raw content if conversion fails
				return content, nil
			}
			return md, nil
		}
		return content, nil
	default:
		return content, nil
	}
}
