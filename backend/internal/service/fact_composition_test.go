package service

import (
	"context"
	"encoding/json"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFactIDsResolveExactOCRQuotesWithoutAskingModelToCopyThem(t *testing.T) {
	source := "TCP通过检验、序号、确认和重传等机制实现可靠传输。TCP在不可靠的IP层之上提供可靠传输服务。这里不再痪述。"
	refs := []domain.Citation{{DocumentID: "textbook", Page: 122, Location: "第122页·TCP"}}
	facts := buildAnswerFacts("TCP如何实现可靠传输？", []string{source}, refs, nil)
	require.Len(t, facts.Facts, 2)
	require.NotContains(t, facts.JSON(), "痪述")
	require.NotContains(t, facts.JSON(), `"source"`)
	s := RAGService{LLM: &reviewLLM{answer: `{"supported":"true"}`}}
	text, sources, ok, reason := s.factComposedAnswer(context.Background(), `{"claims":[{"text":"TCP利用检验、序号、确认和重传机制提供可靠传输服务","facts":[1,2]}]}`, "TCP如何实现可靠传输？", facts, []string{source}, refs, nil)
	require.True(t, ok, reason)
	require.Contains(t, text, "TCP利用")
	require.Len(t, sources, 1)
	require.Contains(t, sources[0].Excerpt, "TCP通过检验")
	require.NotContains(t, sources[0].Excerpt, "TCP利用")
	require.Equal(t, 122, sources[0].Page)
	_, _, ok, reason = s.factComposedAnswer(context.Background(), `{"claims":[{"text":"TCP通过序号、确认和重传实现可靠传输。","facts":[999]}]}`, "TCP如何实现可靠传输？", facts, []string{source}, refs, nil)
	require.False(t, ok)
	require.Equal(t, "unknown_fact_id", reason)
}
func TestFactsAutomaticallyAttachLiteralShortPremises(t *testing.T) {
	contents := []string{"当窗口为零，", "TCP发送方等待接收方更新窗口后再发送数据。"}
	refs := []domain.Citation{{DocumentID: "book", Page: 1, Location: "TCP"}, {DocumentID: "book", Page: 2, Location: "TCP"}}
	deps := map[int][]int{2: {1}}
	facts := buildAnswerFacts("TCP何时发送数据？", contents, refs, deps)
	require.Len(t, facts.Facts, 1)
	require.Equal(t, 1, facts.Facts[0].Premises[0].Source)
	s := RAGService{LLM: &reviewLLM{answer: `{"supported":true}`}}
	text, sources, ok, why := s.factComposedAnswer(context.Background(), `{"claims":[{"text":"窗口为零时，TCP发送方等待窗口更新后再发送数据。","facts":[1]}]}`, "TCP何时发送数据？", facts, contents, refs, deps)
	require.True(t, ok, why)
	require.Contains(t, text, "窗口为零")
	require.Len(t, sources, 2)
	require.Contains(t, sources[1].Excerpt, "当窗口为零")
}
func TestFactReferenceDoesNotBypassSemanticReview(t *testing.T) {
	source := "虚拟内存允许作业按需调入内存。"
	refs := []domain.Citation{{DocumentID: "book", Page: 1}}
	facts := buildAnswerFacts("解释虚拟内存", []string{source}, refs, nil)
	s := RAGService{LLM: &reviewLLM{answer: `{"supported":false}`}}
	_, _, ok, _ := s.factComposedAnswer(context.Background(), `{"claims":[{"text":"虚拟内存能保证所有程序永远不会缺页。","facts":[1]}]}`, "解释虚拟内存", facts, []string{source}, refs, nil)
	require.False(t, ok)
	require.False(t, supportedReview(`{"supported":"yes"}`))
	require.False(t, supportedReview(`{"supported":1}`))
}

func TestFactSchemaCanDowngradeWithoutRelaxingReferenceValidation(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		switch calls {
		case 1:
			require.Equal(t, "json_schema", body["response_format"].(map[string]any)["type"])
			w.WriteHeader(400)
		case 2:
			require.Equal(t, "json_object", body["response_format"].(map[string]any)["type"])
			w.WriteHeader(422)
		default:
			require.Nil(t, body["response_format"])
			w.Write([]byte(`{"choices":[{"message":{"content":"{\"claims\":[]}"},"finish_reason":"stop"}]}`))
		}
	}))
	defer server.Close()
	raw, e := (OpenAILLM{BaseURL: server.URL, APIKey: "fixture"}).AnswerJSON(context.Background(), "解释"+factCompositionInstruction, `{"facts":[{"id":1,"source":1,"text":"原文事实。"}]}`)
	require.NoError(t, e)
	require.Equal(t, 3, calls)
	require.Equal(t, `{"claims":[]}`, raw)
}

// Budgeted-out source details must not become requirements the generator never saw.
func TestVirtualMemoryCoverageUsesSuppliedCatalog(t *testing.T) {
	source := "虚拟内存允许作业按需调入内存。基于局部性原理，程序只需部分装入。"
	refs := []domain.Citation{{DocumentID: "book", Page: 1}}
	catalog := factCatalog{Facts: []answerFact{{ID: 1, Source: 1, Text: "虚拟内存允许作业按需调入内存。"}}, Locations: map[int]string{1: "第1页"}}
	s := RAGService{LLM: &reviewLLM{answer: `{"supported":true}`}}
	_, _, ok, why := s.factComposedAnswer(context.Background(), `{"claims":[{"text":"虚拟内存允许作业按需调入内存。","facts":[1]}]}`, "为什么需要虚拟内存？", catalog, []string{source}, refs, nil)
	require.True(t, ok, why)
}
