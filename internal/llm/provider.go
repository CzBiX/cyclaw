package llm

import "context"

// StreamDelta represents a single chunk from a streaming response.
type StreamDelta struct {
	Content       string              // incremental text content
	FunctionCalls []*ItemFunctionCall // function call info, if applicable
	Done          bool                // true if this is the final chunk
}

// StreamCallback is called for each chunk of a streaming response.
// Return a non-nil error to abort the stream.
type StreamCallback func(delta StreamDelta) error

// Provider defines the interface for LLM providers.
type Provider interface {
	// Chat sends a chat request and returns the response.
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

	// StreamChat sends a chat request and streams the response via the callback.
	// The callback is invoked for each delta chunk. The final accumulated result
	// is returned as a ChatResponse when the stream completes.
	StreamChat(ctx context.Context, req *ChatRequest, cb StreamCallback) (*ChatResponse, error)
}
