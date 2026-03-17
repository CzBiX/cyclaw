package agent

import (
	"testing"

	"cyclaw/internal/llm"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name    string
		items   []llm.Item
		wantMin int
		wantMax int
	}{
		{
			name:    "empty items",
			items:   nil,
			wantMin: 0,
			wantMax: 0,
		},
		{
			name: "single short message",
			items: []llm.Item{
				llm.NewMessage(llm.RoleUser, "Hello"),
			},
			// 4 (overhead) + 5/4 = 4 + 1 = 5
			wantMin: 5,
			wantMax: 5,
		},
		{
			name: "message with tool calls",
			items: []llm.Item{
				llm.NewFunctionCall("Let me check that", llm.ItemFunctionCall{
					CallID:    "call_1",
					Name:      "read_file",
					Arguments: `{"path":"@memory/MEMORY.md"}`,
				}),
			},
			wantMin: 10,
			wantMax: 30,
		},
		{
			name: "multiple messages",
			items: []llm.Item{
				llm.NewMessage(llm.RoleUser, "Hello, how are you doing today?"),
				llm.NewMessage(llm.RoleAssistant, "I'm doing well, thank you for asking!"),
			},
			wantMin: 15,
			wantMax: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateTokens(tt.items)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("estimateTokens() = %d, want between %d and %d", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestCompressionThreshold(t *testing.T) {
	tests := []struct {
		name      string
		maxTokens int
		ratio     float64
		want      int
	}{
		{"default ratio", 128000, 0.8, 102400},
		{"full ratio", 128000, 1.0, 128000},
		{"half ratio", 100000, 0.5, 50000},
		{"zero tokens", 0, 0.8, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compressionThreshold(tt.maxTokens, tt.ratio)
			if got != tt.want {
				t.Errorf("compressionThreshold(%d, %f) = %d, want %d", tt.maxTokens, tt.ratio, got, tt.want)
			}
		})
	}
}

func TestAdjustSplitPoint(t *testing.T) {
	history := []llm.Item{
		llm.NewMessage(llm.RoleUser, "Hello"),
		llm.NewFunctionCall("Hi", llm.ItemFunctionCall{CallID: "1"}),
		llm.NewFunctionCall("Hi", llm.ItemFunctionCall{CallID: "2"}),
		llm.NewFunctionCallOutput("1", "result1"),
		llm.NewFunctionCallOutput("2", "result2"),
		llm.NewMessage(llm.RoleAssistant, "Here is the answer"),
		llm.NewMessage(llm.RoleUser, "Thanks"),
	}

	tests := []struct {
		name     string
		splitIdx int
		want     int
	}{
		{
			name:     "split at user message - no change",
			splitIdx: 0,
			want:     0,
		},
		{
			name:     "split at tool result - moves past tool results",
			splitIdx: 2,
			want:     5,
		},
		{
			name:     "split at second tool result - moves past it",
			splitIdx: 4,
			want:     5,
		},
		{
			name:     "split at assistant message - no change",
			splitIdx: 5,
			want:     5,
		},
		{
			name:     "split beyond history - no change",
			splitIdx: 10,
			want:     10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustSplitPoint(history, tt.splitIdx)
			if got != tt.want {
				t.Errorf("adjustSplitPoint(_, %d) = %d, want %d", tt.splitIdx, got, tt.want)
			}
		})
	}
}
