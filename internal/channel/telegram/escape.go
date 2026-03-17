package telegram

import "strings"

// escapeMarkdownV2 escapes special characters for Telegram MarkdownV2 while
// preserving recognised Markdown formatting constructs such as *bold*,
// _italic_, __underline__, ~strikethrough~, ||spoiler||, `inline code`,
// ```code blocks```, [links](url), and > blockquotes.
//
// Characters inside code spans/blocks are left untouched (Telegram requires
// no escaping there except for ` and \). All other special characters that
// appear outside of recognised constructs are escaped with a preceding '\'.
func escapeMarkdownV2(s string) string {
	runes := []rune(s)
	n := len(runes)
	var out strings.Builder
	out.Grow(n + n/4) // pre-allocate with some room for escapes

	i := 0
	for i < n {
		r := runes[i]

		// --- Code blocks: ```...``` ---
		if r == '`' && i+2 < n && runes[i+1] == '`' && runes[i+2] == '`' {
			end := findCodeBlockEnd(runes, i+3)
			// Write opening ```, escape only ` and \ inside, write closing ```.
			out.WriteString("```")
			closingStart := end - 3
			if closingStart < i+3 {
				// Unclosed code block: no closing ``` found.
				closingStart = end
			}
			escapeCodeContent(&out, runes[i+3:closingStart])
			if closingStart < end {
				out.WriteString("```")
			}
			i = end
			continue
		}

		// --- Inline code: `...` ---
		if r == '`' {
			end := findInlineCodeEnd(runes, i+1)
			if end > i+1 {
				// Matched closing `: write opening, escape content, write closing.
				out.WriteRune('`')
				escapeCodeContent(&out, runes[i+1:end-1])
				out.WriteRune('`')
			} else {
				// No closing ` found; escape the lone backtick.
				out.WriteString("\\`")
			}
			i = end
			continue
		}

		// --- Blockquote: > at the beginning of a line ---
		if r == '>' && isLineStart(runes, i) {
			out.WriteRune('>')
			i++
			continue
		}

		// --- Links: [text](url) ---
		if r == '[' {
			if end, ok := matchLink(runes, i); ok {
				// Write the link construct, escaping text inside [...] but
				// not the structural brackets/parens.
				writeLink(&out, runes[i:end])
				i = end
				continue
			}
		}

		// --- Paired inline formatting ---
		// ||spoiler||
		if r == '|' && i+1 < n && runes[i+1] == '|' {
			if end, ok := matchPaired(runes, i+2, "||"); ok {
				out.WriteString("||")
				writeEscapedRunes(&out, runes[i+2:end])
				out.WriteString("||")
				i = end + 2
				continue
			}
		}

		// __underline__
		if r == '_' && i+1 < n && runes[i+1] == '_' {
			if end, ok := matchPaired(runes, i+2, "__"); ok {
				out.WriteString("__")
				writeEscapedRunes(&out, runes[i+2:end])
				out.WriteString("__")
				i = end + 2
				continue
			}
		}

		// **bold** (standard Markdown double-asterisk bold → Telegram *bold*)
		if r == '*' && i+1 < n && runes[i+1] == '*' {
			if end, ok := matchPaired(runes, i+2, "**"); ok {
				out.WriteRune('*')
				writeEscapedRunes(&out, runes[i+1:end+1])
				out.WriteRune('*')
				i = end + 2
				continue
			}
		}

		// *bold*
		if r == '*' {
			if end, ok := matchPaired(runes, i+1, "*"); ok {
				out.WriteRune('*')
				writeEscapedRunes(&out, runes[i+1:end])
				out.WriteRune('*')
				i = end + 1
				continue
			}
		}

		// _italic_ (single underscore, but not __ which is handled above)
		if r == '_' {
			if end, ok := matchPaired(runes, i+1, "_"); ok {
				out.WriteRune('_')
				writeEscapedRunes(&out, runes[i+1:end])
				out.WriteRune('_')
				i = end + 1
				continue
			}
		}

		// ~strikethrough~
		if r == '~' {
			if end, ok := matchPaired(runes, i+1, "~"); ok {
				out.WriteRune('~')
				writeEscapedRunes(&out, runes[i+1:end])
				out.WriteRune('~')
				i = end + 1
				continue
			}
		}

		// --- Already-escaped character ---
		if r == '\\' && i+1 < n {
			out.WriteRune('\\')
			out.WriteRune(runes[i+1])
			i += 2
			continue
		}

		// --- Escape special characters ---
		if shouldEscape(r) {
			out.WriteRune('\\')
		}
		out.WriteRune(r)
		i++
	}

	return out.String()
}

const specialChars = "_*[]()~`>#+-=|{}.!"

func shouldEscape(r rune) bool {
	// it's more efficient to use a switch here than strings.ContainsRune for every character
	switch r {
	case '_', '*', '[', ']', '(', ')', '~', '`', '>', '#', '+', '-', '=', '|', '{', '}', '.', '!':
		return true
	}
	return false
}

// isLineStart reports whether position i is at the start of a line
// (i.e. i == 0 or runes[i-1] == '\n').
func isLineStart(runes []rune, i int) bool {
	return i == 0 || runes[i-1] == '\n'
}

// findCodeBlockEnd finds the index just past the closing ``` starting search
// from position start. If no closing ``` is found, returns len(runes).
func findCodeBlockEnd(runes []rune, start int) int {
	n := len(runes)
	for i := start; i+2 < n; i++ {
		if runes[i] == '`' && runes[i+1] == '`' && runes[i+2] == '`' {
			return i + 3
		}
	}
	// Unclosed code block — return everything.
	return n
}

// findInlineCodeEnd finds the index just past the closing ` starting from
// position start. If no closing ` is found, returns start (so the caller
// falls through to escape the opening `).
func findInlineCodeEnd(runes []rune, start int) int {
	for i := start; i < len(runes); i++ {
		if runes[i] == '`' {
			return i + 1
		}
	}
	return start
}

// matchLink checks if runes starting at pos form a valid [text](url)
// construct. Returns the end index (just past ')') and true if matched.
func matchLink(runes []rune, pos int) (int, bool) {
	n := len(runes)
	if pos >= n || runes[pos] != '[' {
		return 0, false
	}

	// Find the closing ']'.
	closeBracket := -1
	depth := 0
	for i := pos + 1; i < n; i++ {
		if runes[i] == '\\' {
			i++ // skip escaped char
			continue
		}
		if runes[i] == '[' {
			depth++
		}
		if runes[i] == ']' {
			if depth == 0 {
				closeBracket = i
				break
			}
			depth--
		}
		if runes[i] == '\n' {
			// Links don't span lines.
			return 0, false
		}
	}
	if closeBracket < 0 {
		return 0, false
	}

	// Must be immediately followed by '('.
	if closeBracket+1 >= n || runes[closeBracket+1] != '(' {
		return 0, false
	}

	// Find the closing ')'.
	parenDepth := 0
	for i := closeBracket + 2; i < n; i++ {
		if runes[i] == '\\' {
			i++ // skip escaped char
			continue
		}
		if runes[i] == '(' {
			parenDepth++
		}
		if runes[i] == ')' {
			if parenDepth == 0 {
				return i + 1, true
			}
			parenDepth--
		}
		if runes[i] == '\n' {
			return 0, false
		}
	}

	return 0, false
}

// writeLink writes a [text](url) construct to out, escaping special characters
// inside the link text but leaving the URL part untouched (except for ')' and
// '\' which must be escaped in URLs per MarkdownV2 rules).
func writeLink(out *strings.Builder, runes []rune) {
	// Find the boundary between [text] and (url).
	closeBracket := -1
	depth := 0
	for i := 1; i < len(runes); i++ {
		if runes[i] == '\\' {
			i++
			continue
		}
		if runes[i] == '[' {
			depth++
		}
		if runes[i] == ']' {
			if depth == 0 {
				closeBracket = i
				break
			}
			depth--
		}
	}

	out.WriteRune('[')
	writeEscapedRunes(out, runes[1:closeBracket])
	out.WriteRune(']')

	// Write (url) — the URL portion. We write it verbatim; Telegram is
	// lenient about URL contents as long as parens/backslashes are escaped.
	appendRunes(out, runes[closeBracket+1:])
}

// matchPaired searches for a closing delimiter starting from position start.
// The delimiter must not be preceded by a backslash. Returns the start index
// of the closing delimiter and true if found. The closing delimiter must
// appear on the same line (no newlines allowed in inline formatting).
func matchPaired(runes []rune, start int, delim string) (int, bool) {
	delimRunes := []rune(delim)
	dLen := len(delimRunes)
	n := len(runes)

	// Must have at least one character of content between delimiters.
	for i := start; i+dLen <= n; i++ {
		if runes[i] == '\n' {
			return 0, false
		}
		if runes[i] == '\\' {
			i++ // skip escaped char
			continue
		}
		if i > start && matchRunesAt(runes, i, delimRunes) {
			// Ensure the closing delimiter is not at position start (empty content).
			return i, true
		}
	}
	return 0, false
}

// matchRunesAt checks if runes at position pos match the target sequence.
func matchRunesAt(runes []rune, pos int, target []rune) bool {
	if pos+len(target) > len(runes) {
		return false
	}
	for j, t := range target {
		if runes[pos+j] != t {
			return false
		}
	}
	return true
}

// writeEscapedRunes writes runes to out, escaping special characters.
// This is used for text content inside Markdown constructs.
func writeEscapedRunes(out *strings.Builder, runes []rune) {
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\\' && i+1 < len(runes) {
			out.WriteRune('\\')
			out.WriteRune(runes[i+1])
			i++
			continue
		}
		if shouldEscape(r) {
			out.WriteRune('\\')
		}
		out.WriteRune(r)
	}
}

// escapeCodeContent escapes only ` and \ inside code spans/blocks,
// as required by Telegram MarkdownV2.
func escapeCodeContent(out *strings.Builder, runes []rune) {
	for _, r := range runes {
		if r == '`' || r == '\\' {
			out.WriteRune('\\')
		}
		out.WriteRune(r)
	}
}

// appendRunes writes a slice of runes to the builder without modification.
func appendRunes(out *strings.Builder, runes []rune) {
	for _, r := range runes {
		out.WriteRune(r)
	}
}
