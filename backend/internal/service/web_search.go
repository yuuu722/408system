package service

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/local/exam408-assistant/backend/internal/providerhttp"
)

const webSearchTimeout = 5 * time.Second

type WebResult struct {
	Title, URL, Text string
}

type WebSearcher interface {
	Search(context.Context, string) ([]WebResult, error)
}

// BraveSearch uses a separate search credential. Only the current question is
// sent to the search provider. Raw pages are never fetched by the application.
type BraveSearch struct {
	APIKey string
	Client *http.Client
}

func (s BraveSearch) Search(ctx context.Context, question string) ([]WebResult, error) {
	client := s.Client
	if client == nil {
		client = providerhttp.NewClient(webSearchTimeout)
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	queryText := strings.Join(strings.Fields(question), " ")
	if tcpReliabilityOnly(question) {
		queryText += " TCP sequence number acknowledgment retransmission RFC"
	}
	query := []rune(queryText)
	if len(query) > 220 {
		query = query[:220]
	}
	params := url.Values{"q": {string(query)}, "count": {"12"}, "text_decorations": {"false"}, "extra_snippets": {"true"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.search.brave.com/res/v1/web/search?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", s.APIKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("web search request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("web search returned HTTP %d", resp.StatusCode)
	}
	var data struct {
		Web struct {
			Results []struct {
				Title, URL, Description string
				ExtraSnippets           []string `json:"extra_snippets"`
			}
		}
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 512<<10)).Decode(&data); err != nil {
		return nil, fmt.Errorf("invalid web search response")
	}
	results := make([]WebResult, 0, len(data.Web.Results))
	for _, item := range data.Web.Results {
		if !trustedWebURL(item.URL) {
			continue
		}
		text := html.UnescapeString(strings.Join(append([]string{item.Description}, item.ExtraSnippets...), "\n\n"))
		if text = strings.TrimSpace(text); text != "" {
			results = append(results, WebResult{html.UnescapeString(item.Title), item.URL, text})
		}
	}
	return results, nil
}

func trustedWebURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, domain := range []string{"rfc-editor.org", "ietf.org", "ieee.org", "acm.org", "w3.org", "kernel.org", "docs.python.org", "docs.oracle.com", "learn.microsoft.com", "edu", "edu.cn", "ac.uk"} {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}
