package llm

import (
	"bufio"
	"bytes"
	"context"
	"cyclaw/internal/config"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// OpenAI implements the Provider interface using the OpenAI Responses API.
type OpenAI struct {
	cfg        *config.LLMConfig
	httpClient *http.Client
}

// NewOpenAI creates a new OpenAI provider.
func NewOpenAI(cfg *config.LLMConfig) *OpenAI {
	return &OpenAI{
		cfg:        cfg,
		httpClient: &http.Client{},
	}
}

// ---------------------------------------------------------------------------
// Responses API request types
// ---------------------------------------------------------------------------

type inputItem map[string]any
type outputItem map[string]any

// responsesRequest is the request body for the OpenAI Responses API (POST /responses).
type responsesRequest struct {
	Model        string            `json:"model"`
	Instructions string            `json:"instructions,omitempty"`
	Input        []inputItem       `json:"input"`
	Tools        []responseTool    `json:"tools,omitempty"`
	Reasoning    map[string]string `json:"reasoning,omitempty"`
	Store        bool              `json:"store"`
	Include      []string          `json:"include,omitempty"`
	Stream       bool              `json:"stream,omitempty"`
}

type responseTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// ---------------------------------------------------------------------------
// Responses API response types (non-streaming)
// ---------------------------------------------------------------------------

// responsesResponse is the response from the OpenAI Responses API.
type responsesResponse struct {
	ID     string          `json:"id"`
	Output []outputItem    `json:"output"`
	Usage  *responsesUsage `json:"usage,omitempty"`
	Error  *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// responsesUsage contains token usage information from the Responses API.
type responsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// ---------------------------------------------------------------------------
// Responses API streaming event types
// ---------------------------------------------------------------------------

type textDeltaEvent struct {
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"`
}

type responseDoneEvent struct {
	Response responsesResponse `json:"response"`
}

// ---------------------------------------------------------------------------
// Item ↔ Input conversion
// ---------------------------------------------------------------------------

func itemsToInput(items []Item) []inputItem {
	var result []inputItem
	for _, it := range items {
		switch it.Type {
		case ItemTypeMessage:
			v := it.Message
			result = append(result, inputItem{
				"type":    "message",
				"role":    string(v.Role),
				"content": v.Content,
			})
		case ItemTypeFunctionCall:
			v := it.FunctionCall
			// If the assistant produced text alongside tool calls, include it
			// as a separate message item first.
			// Echo tool calls back as individual input items.
			result = append(result, inputItem{
				"type":      "function_call",
				"call_id":   v.CallID,
				"name":      v.Name,
				"arguments": v.Arguments,
			})
		case ItemTypeFunctionCallOutput:
			v := it.FunctionCallOutput
			result = append(result, inputItem{
				"type":    "function_call_output",
				"call_id": v.CallID,
				"output":  v.Output,
			})
		case ItemTypeOther:
			v := it.Other
			// Preserve opaque items (e.g. reasoning with encrypted content).
			result = append(result, v)
		}
	}
	return result
}

// toolDefsToResponsesTools converts FunctionDef slice to the Responses API tool
// format (flatter structure with name/description/parameters at top level).
func (o *OpenAI) toolDefsToResponsesTools(defs []FunctionDef) []responseTool {
	tools := make([]responseTool, 0, len(defs)+len(o.cfg.ExtraTools))

	for _, t := range o.cfg.ExtraTools {
		tools = append(tools, responseTool{
			Type: t,
		})
	}

	for _, d := range defs {
		tools = append(tools, responseTool{
			Type:        "function",
			Name:        d.Name,
			Description: d.Description,
			Parameters:  d.Parameters,
		})
	}

	return tools
}

// outputItemToItem converts a single Responses API outputItem into an Item.
func outputItemsToItems(outputItems []outputItem) []Item {
	items := make([]Item, 0, len(outputItems))
	for _, item := range outputItems {
		switch item["type"] {
		case "message":
			var parts []string
			for _, c := range item["content"].([]any) {
				cMap := c.(map[string]any)
				if cMap["type"] == "output_text" && cMap["text"] != "" {
					parts = append(parts, cMap["text"].(string))
				}
			}
			items = append(items, NewMessage(Role(item["role"].(string)), strings.Join(parts, "")))

		case "function_call":
			items = append(items, NewFunctionCall("", ItemFunctionCall{
				CallID:    item["call_id"].(string),
				Name:      item["name"].(string),
				Arguments: item["arguments"].(string),
			}))
		case "reasoning":
			if _, ok := item["encrypted_content"]; ok {
				// ignore it since it can't be used for mutiple providers
				continue
			}

			items = append(items, NewOtherItem(item))
		default:
			// Preserve the raw item as MessageOther (e.g. "reasoning").
			delete(item, "id")
			items = append(items, NewOtherItem(item))
		}
	}

	return items
}

func parseResponseOutput(output []outputItem) (items []Item, content string, functionCalls []*ItemFunctionCall) {
	items = outputItemsToItems(output)
	functionCalls = make([]*ItemFunctionCall, 0)

	for _, item := range items {
		switch item.Type {
		case ItemTypeFunctionCall:
			v := item.FunctionCall
			functionCalls = append(functionCalls, v)
		case ItemTypeMessage:
			return items, item.Message.Content, functionCalls
		}
	}

	return items, content, functionCalls
}

// ---------------------------------------------------------------------------
// Provider implementation
// ---------------------------------------------------------------------------

func (o *OpenAI) getReasoning() map[string]string {
	if o.cfg.ReasoningEffort != "" {
		return map[string]string{
			"effort": o.cfg.ReasoningEffort,
		}
	}
	return nil
}

func (o *OpenAI) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if o.cfg.AlwaysStream {
		return o.StreamChat(ctx, req, func(delta StreamDelta) error { return nil })
	}

	body := responsesRequest{
		Model:        req.Model,
		Instructions: req.Instructions,
		Input:        itemsToInput(req.Items),
		Tools:        o.toolDefsToResponsesTools(req.Functions),
		Reasoning:    o.getReasoning(),
	}

	resp, err := o.doRequest(ctx, &body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	start := time.Now()
	respBody, err := io.ReadAll(resp.Body)
	duration := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var rResp responsesResponse
	if err := json.Unmarshal(respBody, &rResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w (body: %s)", err, string(respBody))
	}

	if rResp.Error != nil {
		slog.Warn("llm api error",
			"error_code", rResp.Error.Code,
			"error_message", rResp.Error.Message,
		)
		return nil, fmt.Errorf("api error [%s]: %s", rResp.Error.Code, rResp.Error.Message)
	}

	outputItems, content, functionCalls := parseResponseOutput(rResp.Output)

	usage := Usage{}
	if rResp.Usage != nil {
		usage = Usage{
			PromptTokens:     rResp.Usage.InputTokens,
			CompletionTokens: rResp.Usage.OutputTokens,
			TotalTokens:      rResp.Usage.TotalTokens,
		}
	}

	slog.Info("llm response",
		"model", body.Model,
		"response_id", rResp.ID,
		"duration_ms", duration.Milliseconds(),
		"content_len", len(content),
		"function_calls", len(functionCalls),
		"usage", usage,
	)

	return &ChatResponse{
		Content:       content,
		FunctionCalls: functionCalls,
		Items:         outputItems,
		Usage:         usage,
	}, nil
}

func (o *OpenAI) StreamChat(ctx context.Context, req *ChatRequest, cb StreamCallback) (*ChatResponse, error) {
	if cb == nil {
		return nil, fmt.Errorf("stream callback cannot be nil when streaming is enabled")
	}

	body := responsesRequest{
		Model:        req.Model,
		Instructions: req.Instructions,
		Input:        itemsToInput(req.Items),
		Tools:        o.toolDefsToResponsesTools(req.Functions),
		Stream:       true,
		Reasoning:    o.getReasoning(),
	}

	resp, err := o.doRequest(ctx, &body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	start := time.Now()

	// Accumulators
	var (
		contentBuilder strings.Builder
		fullResp       responsesResponse
	)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		eventType, err := parseEventType(data)
		if err != nil {
			slog.Warn("failed to parse stream event type", "error", err, "data", data)
			continue
		}

		switch eventType {
		case "response.output_text.delta":
			var ev textDeltaEvent
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				slog.Warn("failed to parse text delta", "error", err)
				continue
			}
			contentBuilder.WriteString(ev.Delta)
			if ev.Delta != "" {
				if err := cb(StreamDelta{Content: ev.Delta}); err != nil {
					return nil, fmt.Errorf("stream callback: %w", err)
				}
			}

		case "response.completed", "response.failed":
			var ev responseDoneEvent
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				slog.Warn("failed to parse response done", "error", err)
			} else {
				fullResp = ev.Response
			}
			contentBuilder.Reset()
			if err := cb(StreamDelta{Done: true}); err != nil {
				return nil, fmt.Errorf("stream callback on done: %w", err)
			}

		case "error":
			var errEv struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal([]byte(data), &errEv); err == nil {
				return nil, fmt.Errorf("stream error [%s]: %s", errEv.Code, errEv.Message)
			}
			return nil, fmt.Errorf("stream error: %s", data)

		default:
			// Lifecycle / content-part events — no action needed.
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read stream: %w", err)
	}
	if fullResp.Error != nil {
		slog.Warn("llm api error in stream",
			"code", fullResp.Error.Code,
			"message", fullResp.Error.Message,
		)
		return nil, fmt.Errorf("api error [%s]: %s", fullResp.Error.Code, fullResp.Error.Message)
	}

	outputItems, content, functionCalls := parseResponseOutput(fullResp.Output)

	slog.Info("llm stream response",
		"model", body.Model,
		"duration_ms", time.Since(start).Milliseconds(),
		"content_len", contentBuilder.Len(),
		"tool_calls", len(functionCalls),
		"usage", fullResp.Usage,
	)

	usage := Usage{}
	if fullResp.Usage != nil {
		usage = Usage{
			PromptTokens:     fullResp.Usage.InputTokens,
			CompletionTokens: fullResp.Usage.OutputTokens,
			TotalTokens:      fullResp.Usage.TotalTokens,
		}
	}

	return &ChatResponse{
		Content:       content,
		FunctionCalls: functionCalls,
		Items:         outputItems,
		Usage:         usage,
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// doRequest marshals the request body, sends it to the Responses API endpoint,
// and returns the HTTP response. The caller is responsible for closing the body.
// For non-2xx responses the body is read and an error is returned.
func (o *OpenAI) doRequest(ctx context.Context, body *responsesRequest) (*http.Response, error) {
	if body.Model == "" {
		body.Model = o.cfg.Model
	}

	body.Include = []string{"reasoning.encrypted_content"}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.cfg.BaseURL+"/responses", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.cfg.APIKey)
	if body.Stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}

	resp, err := o.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		var rResp responsesResponse
		if err := json.Unmarshal(respBody, &rResp); err == nil && rResp.Error != nil {
			return nil, fmt.Errorf("api error [%s]: %s", rResp.Error.Code, rResp.Error.Message)
		}
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	return resp, nil
}

// parseEventType extracts the "type" field from a JSON SSE data payload.
func parseEventType(data string) (string, error) {
	var ev struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		return "", err
	}
	return ev.Type, nil
}
