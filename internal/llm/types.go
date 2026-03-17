package llm

import (
	"log/slog"
)

// Role represents the role of a message in a conversation.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleDeveloper Role = "developer"
)

// ItemType discriminates which variant of an Item is populated.
type ItemType string

const (
	ItemTypeMessage            ItemType = "message"
	ItemTypeFunctionCall       ItemType = "function_call"
	ItemTypeFunctionCallOutput ItemType = "function_call_output"
	ItemTypeOther              ItemType = "other"
)

// Item represents a single element in a conversation history.
// It is a tagged union: the Type field indicates which pointer field is
// populated. Exactly one of Message, FunctionCall, FunctionCallOutput,
// or Other is non-nil.
type Item struct {
	Type               ItemType                `json:"type"`
	Message            *ItemMessage            `json:"message,omitempty"`
	FunctionCall       *ItemFunctionCall       `json:"function_call,omitempty"`
	FunctionCallOutput *ItemFunctionCallOutput `json:"function_call_output,omitempty"`
	Other              map[string]any          `json:"other,omitempty"`
}

// ItemMessage represents a plain text message from a user, assistant, or
// developer.
type ItemMessage struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// FunctionCall describes a single function/tool invocation requested by the
// assistant.
type ItemFunctionCall struct {
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ItemFunctionCallOutput carries the result returned by a tool execution.
type ItemFunctionCallOutput struct {
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

// ---------------------------------------------------------------------------
// Item constructors – convenience helpers
// ---------------------------------------------------------------------------

// NewMessage creates a message Item.
func NewMessage(role Role, content string) Item {
	return Item{
		Type:    ItemTypeMessage,
		Message: &ItemMessage{Role: role, Content: content},
	}
}

// NewFunctionCall creates a function-call Item.
func NewFunctionCall(content string, call ItemFunctionCall) Item {
	return Item{
		Type:         ItemTypeFunctionCall,
		FunctionCall: &call,
	}
}

// NewFunctionCallOutput creates a function-call-output Item.
func NewFunctionCallOutput(callID, output string) Item {
	return Item{
		Type:               ItemTypeFunctionCallOutput,
		FunctionCallOutput: &ItemFunctionCallOutput{CallID: callID, Output: output},
	}
}

// NewOtherItem creates an opaque "other" Item.
func NewOtherItem(data map[string]any) Item {
	return Item{
		Type:  ItemTypeOther,
		Other: data,
	}
}

// ---------------------------------------------------------------------------
// Function / tool definitions
// ---------------------------------------------------------------------------

type FunctionDef struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// ---------------------------------------------------------------------------
// Chat request / response
// ---------------------------------------------------------------------------

type ChatRequest struct {
	Model        string
	Instructions string
	Items        []Item
	Functions    []FunctionDef
}

// ChatResponse is the result of a Chat or StreamChat call.
//
// Items collects ALL output items returned by the provider, converted to
// the appropriate Item variants.
//
// Content and ToolCalls are convenience fields that extract the text content
// and tool calls from the LAST output item only, which is the most common
// access pattern for callers.
type ChatResponse struct {
	// Content is the text content extracted from the last output message.
	Content string
	// FunctionCalls are the function calls extracted from the last output items.
	FunctionCalls []*ItemFunctionCall

	// Items collects all returned output items.
	Items []Item
	Usage Usage
}

// ---------------------------------------------------------------------------
// Usage
// ---------------------------------------------------------------------------

type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

func (u Usage) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("prompt", u.PromptTokens),
		slog.Int("completion", u.CompletionTokens),
		slog.Int("total", u.TotalTokens),
	)
}
