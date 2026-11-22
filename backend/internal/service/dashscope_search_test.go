package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type dashScopeTransport func(*http.Request) (*http.Response, error)

func (f dashScopeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDashScopeSearchUsesItsOwnRequestAndTrustedCitations(t *testing.T) {
	require.Nil(t, NewDashScopeSearch("https://dashscope.aliyuncs.com.invalid/compatible-mode/v1", "fixture", "qwen-plus"))
	search := NewDashScopeSearch("https://dashscope.aliyuncs.com/compatible-mode/v1", "fixture", "qwen-plus").(DashScopeSearch)
	search.client = &http.Client{Transport: dashScopeTransport(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "https://dashscope.aliyuncs.com/api/v1/services/aigc/text-generation/generation", r.URL.String())
		require.Equal(t, "Bearer fixture", r.Header.Get("Authorization"))
		var body struct {
			Model      string
			Input      struct{ Messages []map[string]string }
			Parameters map[string]any
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "qwen-plus", body.Model)
		require.Equal(t, "TCP 如何实现可靠传输？", body.Input.Messages[1]["content"])
		require.Equal(t, true, body.Parameters["enable_search"])
		options := body.Parameters["search_options"].(map[string]any)
		require.Equal(t, true, options["forced_search"])
		require.Equal(t, true, options["enable_source"])
		reply := `{"output":{"choices":[{"message":{"content":"TCP 累计确认号表示期望的下一个序号。[ref_1][ref_1]发送方在超时后重传未确认数据[1]。\n不可信结果。[ref_2]\n未注明出处。\n伪造编号。[ref_4]"}}],"search_info":{"search_results":[{"index":1,"title":"RFC 9293","url":"https://www.rfc-editor.org/rfc/rfc9293.html"},{"index":2,"title":"lookalike","url":"https://rfc-editor.org.example.com"}]}}}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(reply)), Header: make(http.Header)}, nil
	})}
	results, err := search.Search(context.Background(), "TCP 如何实现可靠传输？")
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "https://www.rfc-editor.org/rfc/rfc9293.html", results[0].URL)
	require.Equal(t, "TCP 累计确认号表示期望的下一个序号。\n发送方在超时后重传未确认数据。\n", results[0].Text)
}

func TestTCPWebSourceCanExplainOneMissingMechanism(t *testing.T) {
	q := "TCP 如何实现可靠传输？"
	require.True(t, webEvidenceRelated(q, "TCP · RFC 9293", "The acknowledgment number is the next sequence number expected."))
	require.False(t, webEvidenceRelated(q, "HDLC", "The acknowledgment number is the next sequence number expected."))
}

func TestVirtualMemoryWebEvidenceCanUseEnglishUniversitySources(t *testing.T) {
	q := "为什么需要虚拟内存？"
	require.True(t, webEvidenceRelated(q, "Virtual Memory", "Demand paging uses locality to keep only actively used pages in physical memory."))
	require.False(t, webEvidenceRelated(q, "Virtual Memory", "The next lecture starts at noon."))
	require.False(t, webEvidenceRelated(q, "CPU Cache", "Caches exploit locality to improve average access time."))
}
