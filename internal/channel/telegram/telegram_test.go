package telegram

import (
	"strings"
	"testing"
)

func TestSplitMessage_Short(t *testing.T) {
	text := "Hello, world!"
	chunks := splitMessage(text)

	if len(chunks) != 1 {
		t.Fatalf("splitMessage() returned %d chunks, want 1", len(chunks))
	}
	if chunks[0] != text {
		t.Errorf("chunks[0] = %q, want %q", chunks[0], text)
	}
}

func TestSplitMessage_ExactLimit(t *testing.T) {
	text := strings.Repeat("x", maxMessageLength)
	chunks := splitMessage(text)

	if len(chunks) != 1 {
		t.Fatalf("splitMessage() returned %d chunks, want 1", len(chunks))
	}
}

func TestSplitMessage_LongMessage(t *testing.T) {
	// Create a message that exceeds maxMessageLength with multiple lines
	var lines []string
	for range 100 {
		lines = append(lines, strings.Repeat("A", 80))
	}
	text := strings.Join(lines, "\n")

	if len(text) <= maxMessageLength {
		t.Skip("test message is not long enough")
	}

	chunks := splitMessage(text)

	if len(chunks) < 2 {
		t.Fatalf("splitMessage() returned %d chunks, want >= 2", len(chunks))
	}

	// Each chunk should not exceed maxMessageLength
	for i, chunk := range chunks {
		if len(chunk) > maxMessageLength {
			t.Errorf("chunk[%d] length = %d, exceeds %d", i, len(chunk), maxMessageLength)
		}
	}

	// Reconstructed text should match original
	reconstructed := strings.Join(chunks, "\n")
	if reconstructed != text {
		t.Error("reconstructed text does not match original")
	}
}

func TestSplitMessage_Empty(t *testing.T) {
	chunks := splitMessage("")
	if len(chunks) != 1 {
		t.Fatalf("splitMessage(\"\") returned %d chunks, want 1", len(chunks))
	}
	if chunks[0] != "" {
		t.Errorf("chunks[0] = %q, want empty", chunks[0])
	}
}

func TestEscapeMarkdownV2(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output string
	}{
		{
			name:   "plain text with period",
			input:  "No special characters here.",
			output: "No special characters here\\.",
		},
		{
			name:   "standalone special chars escaped",
			input:  "Price is 9.99! See #details",
			output: "Price is 9\\.99\\! See \\#details",
		},
		{
			name:   "bold preserved",
			input:  "This is *bold* text.",
			output: "This is *bold* text\\.",
		},
		{
			name:   "italic preserved",
			input:  "This is _italic_ text.",
			output: "This is _italic_ text\\.",
		},
		{
			name:   "underline preserved",
			input:  "This is __underline__ text.",
			output: "This is __underline__ text\\.",
		},
		{
			name:   "strikethrough preserved",
			input:  "This is ~deleted~ text.",
			output: "This is ~deleted~ text\\.",
		},
		{
			name:   "spoiler preserved",
			input:  "This is ||spoiler|| text.",
			output: "This is ||spoiler|| text\\.",
		},
		{
			name:   "inline code preserved",
			input:  "Run `go test` now.",
			output: "Run `go test` now\\.",
		},
		{
			name:   "code block preserved",
			input:  "```go\nfmt.Println(\"hello\")\n```",
			output: "```go\nfmt.Println(\"hello\")\n```",
		},
		{
			name:   "code block with special chars preserved",
			input:  "```\na.b!c#d`e\\ f\n```",
			output: "```\na.b!c#d\\`e\\\\ f\n```",
		},
		{
			name:   "link preserved",
			input:  "Visit [Google](https://google.com) today!",
			output: "Visit [Google](https://google.com) today\\!",
		},
		{
			name:   "blockquote preserved",
			input:  "> This is a quote",
			output: "> This is a quote",
		},
		{
			name:   "blockquote mid-text",
			input:  "Hello\n> quoted line",
			output: "Hello\n> quoted line",
		},
		{
			name:   "mixed formatting",
			input:  "*bold* and _italic_ with a [link](http://example.com)!",
			output: "*bold* and _italic_ with a [link](http://example.com)\\!",
		},
		{
			name:   "already escaped char preserved",
			input:  "already \\! escaped",
			output: "already \\! escaped",
		},
		{
			name:   "special chars inside code not double-escaped",
			input:  "`a.b!c`",
			output: "`a.b!c`",
		},
		{
			name:   "special chars inside code block not escaped",
			input:  "```\na.b!c#d\n```",
			output: "```\na.b!c#d\n```",
		},
		{
			name:   "unmatched star escaped",
			input:  "5 * 3 = 15",
			output: "5 \\* 3 \\= 15",
		},
		{
			name:   "dash and plus escaped",
			input:  "- item one\n+ item two",
			output: "\\- item one\n\\+ item two",
		},
		{
			name:   "pipe escaped outside formatting",
			input:  "a | b",
			output: "a \\| b",
		},
		{
			name:   "pipe escaped outside formatting 2",
			input:  "*a* | 2026",
			output: "*a* \\| 2026",
		},
		{
			name:   "pipe escaped outside formatting 3",
			input:  "**a** | 2026",
			output: "*a* \\| 2026",
		},
		{
			name:   "pipe inside bold preserved",
			input:  "*a | b*",
			output: "*a \\| b*",
		},
		{
			name:   "dash inside bold preserved",
			input:  "*a - b*",
			output: "*a \\- b*",
		},
		{
			name:   "curly braces escaped",
			input:  "{key}",
			output: "\\{key\\}",
		},
		{
			name:   "bold with special chars inside",
			input:  "*hello.world!*",
			output: "*hello\\.world\\!*",
		},
		{
			name:   "nested formatting: bold inside italic content",
			input:  "Use _italic_ and *bold* together.",
			output: "Use _italic_ and *bold* together\\.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			escaped := escapeMarkdownV2(tt.input)
			if escaped != tt.output {
				t.Errorf("escapeMarkdownV2(%q)\n got  %q\n want %q", tt.input, escaped, tt.output)
			}
		})
	}
}
