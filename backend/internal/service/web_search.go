package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/local/exam408-assistant/backend/internal/providerhttp"
)

const webSearchTimeout = 18 * time.Second

type WebResult struct {
	Title, URL, Text string
}

type WebSearcher interface {
	Search(context.Context, string) ([]WebResult, error)
}

type DashScopeSearch struct {
	endpoint, apiKey, model string
	client                  *http.Client
}

// Only send the shared model credential to an official DashScope endpoint.
func NewDashScopeSearch(baseURL, apiKey, model string) WebSearcher {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || strings.TrimSpace(apiKey) == "" {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	switch host {
	case "dashscope.aliyuncs.com", "dashscope-intl.aliyuncs.com", "dashscope-us.aliyuncs.com":
	default:
		return nil
	}
	return DashScopeSearch{
		endpoint: "https://" + host + "/api/v1/services/aigc/text-generation/generation",
		apiKey:   apiKey, model: model,
	}
}

var dashScopeRef = regexp.MustCompile(`\[ref_([0-9]+)\]`)

func (s DashScopeSearch) Search(ctx context.Context, question string) ([]WebResult, error) {
	client := s.client
	if client == nil {
		client = providerhttp.NewClient(webSearchTimeout)
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	query := []rune(strings.Join(strings.Fields(question), " "))
	if len(query) > 800 {
		query = query[:800]
	}
	payload := map[string]any{
		"model": s.model,
		"input": map[string]any{"messages": []map[string]string{
			{"role": "system", "content": "联网查找能直接回答问题的权威来源，如 RFC、官方文档或大学课程。每行只写一句完整的事实，准确保留搜索系统提供的真实出处引用。每句只对应一个来源。不使用模型记忆，不生成链接或猜测出处，找不到则留空。"},
			{"role": "user", "content": string(query)},
		}},
		"parameters": map[string]any{
			"result_format": "message", "enable_search": true, "max_tokens": 1100,
			"search_options": map[string]any{"forced_search": true, "enable_source": true, "enable_citation": true, "citation_format": "[ref_<number>]"},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DashScope web search request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DashScope web search returned HTTP %d", resp.StatusCode)
	}
	return dashScopeWebResults(io.LimitReader(resp.Body, 512<<10))
}

func dashScopeWebResults(body io.Reader) ([]WebResult, error) {
	var data struct {
		Code   string
		Output struct {
			Choices []struct {
				Message struct{ Content string }
			}
			SearchInfo struct {
				SearchResults []struct {
					Index      int
					Title, URL string
				} `json:"search_results"`
			} `json:"search_info"`
		}
	}
	if err := json.NewDecoder(body).Decode(&data); err != nil || data.Code != "" {
		return nil, fmt.Errorf("invalid DashScope web search response")
	}
	if len(data.Output.Choices) == 0 {
		return nil, nil
	}
	results := []WebResult{}
	positions := map[int]int{}
	for _, source := range data.Output.SearchInfo.SearchResults {
		if source.Index <= 0 || !trustedWebURL(source.URL) {
			continue
		}
		positions[source.Index] = len(results)
		results = append(results, WebResult{Title: html.UnescapeString(source.Title), URL: source.URL})
	}
	for _, line := range strings.Split(data.Output.Choices[0].Message.Content, "\n") {
		refs := dashScopeRef.FindAllStringSubmatch(line, -1)
		if len(refs) != 1 {
			continue
		}
		index, err := strconv.Atoi(refs[0][1])
		position, known := positions[index]
		if err != nil || !known {
			continue
		}
		text := strings.TrimSpace(dashScopeRef.ReplaceAllString(line, ""))
		text = strings.TrimLeft(text, "-* ")
		if text != "" {
			results[position].Text += text + "\n"
		}
	}
	usable := results[:0]
	for _, result := range results {
		if result.Text != "" {
			usable = append(usable, result)
		}
	}
	return usable, nil
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
