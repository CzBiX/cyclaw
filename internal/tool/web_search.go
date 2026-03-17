package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	searchTimeout    = 30 * time.Second
	maxSearchResults = 20
	userAgent        = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

type WebSearchTool struct{}

func NewWebSearchTool() *WebSearchTool {
	return &WebSearchTool{}
}

func (t *WebSearchTool) Name() string { return "web_search" }

func (t *WebSearchTool) Description() string {
	return "Search the web via DuckDuckGo."
}

func (t *WebSearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "Search query."
			},
			"max_results": {
				"type": "integer",
				"description": "Max results to return (default 5, max 20)."
			}
		},
		"required": ["query"]
	}`)
}

// DuckDuckGo HTML search result
type ddgResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// DuckDuckGo Instant Answer API response
type ddgInstantAnswer struct {
	Abstract       string `json:"Abstract"`
	AbstractText   string `json:"AbstractText"`
	AbstractSource string `json:"AbstractSource"`
	AbstractURL    string `json:"AbstractURL"`
	Heading        string `json:"Heading"`
	Answer         string `json:"Answer"`
	AnswerType     string `json:"AnswerType"`
	RelatedTopics  []struct {
		Text     string `json:"Text"`
		FirstURL string `json:"FirstURL"`
		Result   string `json:"Result"`
		Name     string `json:"Name"`
		Topics   []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
			Result   string `json:"Result"`
		} `json:"Topics"`
	} `json:"RelatedTopics"`
	Results []struct {
		Text     string `json:"Text"`
		FirstURL string `json:"FirstURL"`
		Result   string `json:"Result"`
	} `json:"Results"`
}

func (t *WebSearchTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("parse params: %w", err)
	}

	if p.Query == "" {
		return "", fmt.Errorf("query is required")
	}

	if p.MaxResults <= 0 {
		p.MaxResults = 5
	}
	if p.MaxResults > maxSearchResults {
		p.MaxResults = maxSearchResults
	}

	// Try DuckDuckGo HTML search to get real web results
	results, err := t.scrapeHTMLResults(ctx, p.Query, p.MaxResults)
	if err != nil {
		// Fallback to the Instant Answer API
		return t.instantAnswerSearch(ctx, p.Query, p.MaxResults)
	}

	if len(results) == 0 {
		// Fallback to the Instant Answer API if no HTML results
		return t.instantAnswerSearch(ctx, p.Query, p.MaxResults)
	}

	return formatResults(p.Query, results), nil
}

// scrapeHTMLResults fetches DuckDuckGo's HTML lite page and extracts results
func (t *WebSearchTool) scrapeHTMLResults(ctx context.Context, query string, maxResults int) ([]ddgResult, error) {
	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()

	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch search page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchSize))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return parseHTMLResults(string(body), maxResults), nil
}

// parseHTMLResults does simple string-based extraction from DuckDuckGo's HTML lite page.
// The HTML lite page has a predictable structure with result links in <a class="result__a"> tags
// and snippets in <a class="result__snippet"> tags.
func parseHTMLResults(html string, maxResults int) []ddgResult {
	var results []ddgResult

	// Split by result blocks - each result is in a div with class "result"
	parts := strings.Split(html, "class=\"result__a\"")
	if len(parts) <= 1 {
		// Try alternative class name
		parts = strings.Split(html, "class=\"result__title")
	}

	for i := 1; i < len(parts) && len(results) < maxResults; i++ {
		part := parts[i]

		// Extract URL from href attribute
		resultURL := extractAttr(part, "href=\"")
		if resultURL == "" {
			continue
		}

		// DuckDuckGo wraps URLs through a redirect - extract the actual URL
		if strings.Contains(resultURL, "uddg=") {
			if u, err := url.Parse(resultURL); err == nil {
				if actual := u.Query().Get("uddg"); actual != "" {
					resultURL = actual
				}
			}
		}

		// Skip ad links
		if strings.Contains(resultURL, "duckduckgo.com/y.js") {
			continue
		}

		// Extract title (text between > and </a>)
		title := extractText(part)
		if title == "" {
			title = resultURL
		}

		// Extract snippet - look for result__snippet in the remainder
		snippet := ""
		snippetIdx := strings.Index(part, "result__snippet")
		if snippetIdx >= 0 {
			snippetPart := part[snippetIdx:]
			// Find the first > after the class
			gtIdx := strings.Index(snippetPart, ">")
			if gtIdx >= 0 {
				endIdx := strings.Index(snippetPart[gtIdx:], "</")
				if endIdx >= 0 {
					snippet = stripHTML(snippetPart[gtIdx+1 : gtIdx+endIdx])
				}
			}
		}

		results = append(results, ddgResult{
			Title:   cleanText(title),
			URL:     resultURL,
			Snippet: cleanText(snippet),
		})
	}

	return results
}

// instantAnswerSearch uses DuckDuckGo's official Instant Answer API as a fallback
func (t *WebSearchTool) instantAnswerSearch(ctx context.Context, query string, maxResults int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()

	apiURL := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_redirect=1&no_html=1", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Cyclaw/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch DDG API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d from DDG API", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchSize))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	var answer ddgInstantAnswer
	if err := json.Unmarshal(body, &answer); err != nil {
		return "", fmt.Errorf("parse DDG response: %w", err)
	}

	var results []ddgResult

	// Add the abstract as a top result if available
	if answer.AbstractText != "" {
		results = append(results, ddgResult{
			Title:   answer.Heading,
			URL:     answer.AbstractURL,
			Snippet: answer.AbstractText,
		})
	}

	// Add direct answer if available
	if answer.Answer != "" && len(results) == 0 {
		results = append(results, ddgResult{
			Title:   "Direct Answer",
			URL:     "",
			Snippet: answer.Answer,
		})
	}

	// Add related topics
	for _, topic := range answer.RelatedTopics {
		if len(results) >= maxResults {
			break
		}
		if topic.Text != "" {
			results = append(results, ddgResult{
				Title:   extractTopicTitle(topic.Text),
				URL:     topic.FirstURL,
				Snippet: topic.Text,
			})
		}
		// Handle nested topic groups
		for _, sub := range topic.Topics {
			if len(results) >= maxResults {
				break
			}
			if sub.Text != "" {
				results = append(results, ddgResult{
					Title:   extractTopicTitle(sub.Text),
					URL:     sub.FirstURL,
					Snippet: sub.Text,
				})
			}
		}
	}

	// Add direct results
	for _, r := range answer.Results {
		if len(results) >= maxResults {
			break
		}
		results = append(results, ddgResult{
			Title:   extractTopicTitle(r.Text),
			URL:     r.FirstURL,
			Snippet: r.Text,
		})
	}

	if len(results) == 0 {
		return fmt.Sprintf("No results found for query: %s", query), nil
	}

	return formatResults(query, results), nil
}

func formatResults(query string, results []ddgResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for: %s\n\n", query))

	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, r.Title))
		if r.URL != "" {
			sb.WriteString(fmt.Sprintf("   URL: %s\n", r.URL))
		}
		if r.Snippet != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", r.Snippet))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// Helper functions

func extractAttr(s, attr string) string {
	idx := strings.Index(s, attr)
	if idx < 0 {
		return ""
	}
	start := idx + len(attr)
	end := strings.IndexByte(s[start:], '"')
	if end < 0 {
		return ""
	}
	return s[start : start+end]
}

func extractText(s string) string {
	// Find first > which closes the tag
	gtIdx := strings.IndexByte(s, '>')
	if gtIdx < 0 {
		return ""
	}
	// Find the closing </a>
	endIdx := strings.Index(s[gtIdx:], "</a>")
	if endIdx < 0 {
		endIdx = strings.Index(s[gtIdx:], "</")
		if endIdx < 0 {
			return ""
		}
	}
	return stripHTML(s[gtIdx+1 : gtIdx+endIdx])
}

func stripHTML(s string) string {
	var result strings.Builder
	inTag := false
	for _, c := range s {
		if c == '<' {
			inTag = true
			continue
		}
		if c == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(c)
		}
	}
	return result.String()
}

func cleanText(s string) string {
	s = strings.TrimSpace(s)
	// Decode common HTML entities
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#x27;", "'")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	return s
}

func extractTopicTitle(text string) string {
	// DuckDuckGo topic texts often start with the title in bold or before a dash
	if idx := strings.Index(text, " - "); idx > 0 && idx < 80 {
		return text[:idx]
	}
	// Truncate long texts for titles
	if len(text) > 80 {
		return text[:77] + "..."
	}
	return text
}
