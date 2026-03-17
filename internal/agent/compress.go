package agent

import (
	"context"
	"fmt"
	"log/slog"

	"cyclaw/internal/llm"
)

// estimateTokens provides a rough token count for a list of items.
// It uses ~4 characters per token as a heuristic, which is reasonable
// for English text with GPT-style tokenizers.
// Each item also adds a small overhead for role/formatting (~4 tokens).
func estimateTokens(items []llm.Item) int {
	total := 0

	for _, it := range items {
		// Item overhead (role, formatting)
		total += 4

		switch it.Type {
		case llm.ItemTypeMessage:
			total += len(it.Message.Content) / 4
		case llm.ItemTypeFunctionCall:
			total += len(it.FunctionCall.Name)/4 + len(it.FunctionCall.Arguments)/4 + 16
		case llm.ItemTypeFunctionCallOutput:
			total += len(it.FunctionCallOutput.Output) / 4
		case llm.ItemTypeOther:
			// Opaque items (e.g. reasoning) — rough estimate based on type.
			// Encrypted content is variable-length but we have no text to measure.
			total += 100
		}
	}
	return total
}

// compressionThreshold returns the token count at which compression should
// be triggered. The ratio (e.g. 0.8) determines what fraction of maxTokens
// triggers compression, leaving room for the response.
func compressionThreshold(maxTokens int, ratio float64) int {
	return int(float64(maxTokens) * ratio)
}

// summaryPrompt is used to instruct the LLM to summarize conversation history.
const summaryPrompt = `You are a conversation summarizer. Your job is to create a concise but comprehensive summary of the conversation history provided below.

Requirements:
- Preserve all important facts, decisions, user preferences, and action items
- Maintain the chronological flow of key events
- Include any tool calls and their results that are relevant to ongoing context
- Keep the summary concise but do not lose critical information
- Write in third person (e.g., "The user asked...", "The assistant responded...")
- Output ONLY the summary, nothing else

Summarize the following conversation:`

// compressHistory calls the LLM to summarize older items when the context
// is too large. It preserves recent items and replaces older ones with a
// summary. Returns the compressed history.
func compressHistory(ctx context.Context, provider llm.Provider, model string, history []llm.Item, maxTokens int, compressionRatio float64) ([]llm.Item, error) {
	threshold := compressionThreshold(maxTokens, compressionRatio)
	estimated := estimateTokens(history)

	if estimated <= threshold {
		return history, nil
	}

	slog.Info("context compression triggered",
		"estimated_tokens", estimated,
		"threshold", threshold,
		"history_len", len(history),
	)

	// Find a split point: keep the most recent ~30% of items, summarize the rest.
	// We need to keep enough recent context for the conversation to make sense,
	// but compress enough to get below the threshold.
	keepCount := min(max(len(history)/3, 4), len(history))

	// Ensure we don't split in the middle of a tool call sequence.
	// Tool results must stay with their corresponding assistant tool-call item.
	splitIdx := len(history) - keepCount
	splitIdx = adjustSplitPoint(history, splitIdx)

	if splitIdx <= 0 {
		// Nothing to compress
		return history, nil
	}

	toCompress := history[:splitIdx]
	toKeep := history[splitIdx:]

	conversationText := messagesToText(toCompress)

	summaryReq := &llm.ChatRequest{
		Model:        model,
		Instructions: summaryPrompt,
		Items: []llm.Item{
			llm.NewMessage(llm.RoleUser, conversationText),
		},
	}

	resp, err := provider.Chat(ctx, summaryReq)
	if err != nil {
		slog.Error("context compression failed, falling back to truncation",
			"error", err,
		)
		// Fallback: just keep the recent items without summary
		return toKeep, nil
	}

	slog.Info("context compressed",
		"original_items", len(toCompress),
		"summary_tokens", resp.Usage.CompletionTokens,
		"kept_items", len(toKeep),
	)

	// Build new history: summary as a system-like user message + kept items
	compressed := make([]llm.Item, 0, len(toKeep)+1)
	compressed = append(compressed, llm.NewMessage(llm.RoleUser, fmt.Sprintf("[Previous conversation summary]\n%s", resp.Content)))
	compressed = append(compressed, toKeep...)

	return compressed, nil
}

// adjustSplitPoint ensures we don't split in the middle of a tool call sequence.
// It moves the split point forward (keeping more items) to avoid orphaning
// tool result items from their corresponding assistant tool-call item.
func adjustSplitPoint(history []llm.Item, splitIdx int) int {
	if splitIdx >= len(history) {
		return splitIdx
	}

	// Move forward past any tool result items at the split point.
	for splitIdx < len(history) && (history[splitIdx].Type == llm.ItemTypeFunctionCallOutput || history[splitIdx].Type == llm.ItemTypeFunctionCall) {
		splitIdx++
	}

	// Also check: if the item just before splitIdx is an assistant message
	// with tool calls, include it in the compress set (it's already there since
	// splitIdx is exclusive for toCompress, so this is fine).

	return splitIdx
}
