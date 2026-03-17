package tool

import (
	"fmt"
	"strconv"
	"strings"
)

// applyUnifiedDiff applies a unified diff to the original text and returns the
// patched result. It supports standard unified diff format as produced by
// "diff -u", git diff, etc.
//
// The diff may contain multiple hunks. Lines starting with "---", "+++", and
// "diff " are treated as header lines and skipped. Each hunk starts with a
// line matching "@@ -<old>,<count> +<new>,<count> @@".
func applyUnifiedDiff(original, diff string) (string, error) {
	hunks, err := parseHunks(diff)
	if err != nil {
		return "", err
	}
	if len(hunks) == 0 {
		return "", fmt.Errorf("no hunks found in diff")
	}

	origLines := splitLines(original)

	// Resolve any hunks with unspecified positions (bare "@@" headers)
	// by searching for matching content in the original file.
	for i := range hunks {
		if hunks[i].oldStart == 0 {
			pos, err := findHunkPosition(origLines, hunks[i])
			if err != nil {
				return "", fmt.Errorf("hunk %d: %w", i+1, err)
			}
			hunks[i].oldStart = pos
		}
	}

	// Apply hunks in reverse order so that line numbers in earlier hunks
	// remain valid after later hunks are applied.
	for i := len(hunks) - 1; i >= 0; i-- {
		origLines, err = applyHunk(origLines, hunks[i])
		if err != nil {
			return "", fmt.Errorf("hunk %d: %w", i+1, err)
		}
	}

	return joinLines(origLines), nil
}

// hunk represents a single hunk from a unified diff.
type hunk struct {
	oldStart int // 1-based starting line in the original file
	oldCount int
	lines    []diffLine
}

type diffOp byte

const (
	diffContext diffOp = ' '
	diffAdd     diffOp = '+'
	diffRemove  diffOp = '-'
)

type diffLine struct {
	op   diffOp
	text string
}

// parseHunks extracts all hunks from a unified diff string.
func parseHunks(diff string) ([]hunk, error) {
	lines := strings.Split(diff, "\n")
	// Remove trailing empty string caused by a final newline — it is a
	// splitting artifact, not a real diff line.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	var hunks []hunk
	var cur *hunk

	for i := range lines {
		line := lines[i]

		// Skip diff headers.
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "diff ") || strings.HasPrefix(line, "index ") {
			continue
		}

		// Hunk header: @@ -oldStart[,oldCount] +newStart[,newCount] @@
		// Also support bare "@@" (no line numbers; position inferred from content).
		if strings.HasPrefix(line, "@@ ") || line == "@@" {
			h, err := parseHunkHeader(line)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", i+1, err)
			}
			hunks = append(hunks, h)
			cur = &hunks[len(hunks)-1]
			continue
		}

		if cur == nil {
			// Lines before the first hunk (e.g. file headers) are ignored.
			continue
		}

		// Parse diff body lines.
		if len(line) == 0 {
			// Empty line in diff body is treated as context with empty text.
			cur.lines = append(cur.lines, diffLine{op: diffContext, text: ""})
			continue
		}

		switch line[0] {
		case ' ':
			cur.lines = append(cur.lines, diffLine{op: diffContext, text: line[1:]})
		case '+':
			cur.lines = append(cur.lines, diffLine{op: diffAdd, text: line[1:]})
		case '-':
			cur.lines = append(cur.lines, diffLine{op: diffRemove, text: line[1:]})
		case '\\':
			// "\ No newline at end of file" — skip.
			continue
		default:
			// Tolerate lines without a leading marker by treating them as context.
			cur.lines = append(cur.lines, diffLine{op: diffContext, text: line})
		}
	}

	return hunks, nil
}

// parseHunkHeader parses a unified diff hunk header line like:
//
//	@@ -1,5 +1,6 @@
//	@@ -10 +10,2 @@ optional section heading
//	@@                             (bare — position inferred from content)
func parseHunkHeader(line string) (hunk, error) {
	// Bare "@@" with no line numbers: oldStart=0 signals that the position
	// must be inferred from the file content.
	if line == "@@" {
		return hunk{oldStart: 0, oldCount: 0}, nil
	}

	// Trim the leading "@@ " and find the closing " @@".
	rest := strings.TrimPrefix(line, "@@ ")
	before, _, ok := strings.Cut(rest, " @@")
	if !ok {
		return hunk{}, fmt.Errorf("malformed hunk header: %q", line)
	}
	rangePart := before // e.g. "-1,5 +1,6"

	parts := strings.SplitN(rangePart, " ", 2)
	if len(parts) != 2 {
		return hunk{}, fmt.Errorf("malformed hunk header ranges: %q", line)
	}

	oldStart, oldCount, err := parseRange(parts[0], '-')
	if err != nil {
		return hunk{}, fmt.Errorf("old range in %q: %w", line, err)
	}

	return hunk{
		oldStart: oldStart,
		oldCount: oldCount,
	}, nil
}

// parseRange parses a range like "-1,5" or "+1" with the given prefix character.
func parseRange(s string, prefix byte) (start, count int, err error) {
	if len(s) == 0 || s[0] != prefix {
		return 0, 0, fmt.Errorf("expected %q prefix in %q", string(prefix), s)
	}
	s = s[1:] // strip prefix

	if idx := strings.Index(s, ","); idx >= 0 {
		start, err = strconv.Atoi(s[:idx])
		if err != nil {
			return 0, 0, fmt.Errorf("parse start: %w", err)
		}
		count, err = strconv.Atoi(s[idx+1:])
		if err != nil {
			return 0, 0, fmt.Errorf("parse count: %w", err)
		}
	} else {
		start, err = strconv.Atoi(s)
		if err != nil {
			return 0, 0, fmt.Errorf("parse start: %w", err)
		}
		count = 1
	}
	return start, count, nil
}

// findHunkPosition searches origLines for a contiguous sequence that matches
// the context and remove lines of the hunk. It returns the 1-based start
// position. This is used when a hunk header has no line numbers (bare "@@").
func findHunkPosition(origLines []string, h hunk) (int, error) {
	// Extract the "old side" lines (context + remove) in order.
	var oldSide []string
	for _, dl := range h.lines {
		if dl.op == diffContext || dl.op == diffRemove {
			oldSide = append(oldSide, dl.text)
		}
	}

	if len(oldSide) == 0 {
		// Pure addition with no context — place at the beginning.
		return 1, nil
	}

	// Scan for the first position where oldSide matches contiguously.
	for i := 0; i <= len(origLines)-len(oldSide); i++ {
		match := true
		for j, s := range oldSide {
			if origLines[i+j] != s {
				match = false
				break
			}
		}
		if match {
			return i + 1, nil // convert to 1-based
		}
	}

	return 0, fmt.Errorf("could not find matching position for hunk content in original file")
}

// applyHunk applies a single hunk to the lines slice and returns the result.
func applyHunk(lines []string, h hunk) ([]string, error) {
	// Convert to 0-based index.
	pos := h.oldStart - 1
	if pos < 0 {
		pos = 0
	}

	// Build the replacement segment and verify context/remove lines match.
	var newSegment []string
	origPos := pos
	for _, dl := range h.lines {
		switch dl.op {
		case diffContext:
			if origPos >= len(lines) {
				return nil, fmt.Errorf("context line beyond end of file at line %d", origPos+1)
			}
			if lines[origPos] != dl.text {
				return nil, fmt.Errorf("context mismatch at line %d: expected %q, got %q", origPos+1, dl.text, lines[origPos])
			}
			newSegment = append(newSegment, dl.text)
			origPos++
		case diffRemove:
			if origPos >= len(lines) {
				return nil, fmt.Errorf("remove line beyond end of file at line %d", origPos+1)
			}
			if lines[origPos] != dl.text {
				return nil, fmt.Errorf("remove mismatch at line %d: expected %q, got %q", origPos+1, dl.text, lines[origPos])
			}
			// Skip this line (don't add to newSegment).
			origPos++
		case diffAdd:
			newSegment = append(newSegment, dl.text)
		}
	}

	// Splice: lines[:pos] + newSegment + lines[origPos:]
	result := make([]string, 0, len(lines)-int(origPos-pos)+len(newSegment))
	result = append(result, lines[:pos]...)
	result = append(result, newSegment...)
	result = append(result, lines[origPos:]...)

	return result, nil
}

// splitLines splits text into lines. A trailing newline does NOT produce an
// extra empty element (matching the convention that text files end with \n).
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	// If the file ends with a newline, the split produces a trailing empty
	// string which we keep — it represents the fact that the last line is
	// terminated.
	return lines
}

// joinLines joins lines back with newline separators.
func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}
