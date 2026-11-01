package service

import (
	"context"
	"encoding/json"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReportedHalfSentenceIsNeverFinishedProse(t *testing.T) {
	half := "基于局部性原理，应用程序在运行之前并不必全部装入内存，仅须将当前要运行的那部分"
	require.Empty(t, completeSourceSentences(half))
	require.False(t, completeAnswerText(half))
	require.False(t, completeAnswerText(half+"。"))
	full := "虚拟内存利用局部性，只将当前需要的程序和数据调入内存即可运行。"
	require.True(t, completeAnswerText(full))
	require.Equal(t, []string{full}, completeSourceSentences(full+half))
	llm := &reviewLLM{answer: `{"supported":true}`}
	s := RAGService{LLM: llm}
	raw, _ := json.Marshal(map[string]any{"claims": []map[string]any{{"text": half, "kind": "explanation", "evidence": []map[string]any{{"source": 1, "quote": half}}}}})
	_, _, ok := s.composedAnswer(context.Background(), string(raw), "为什么需要虚拟内存？", []string{half}, []domain.Citation{{DocumentID: "book"}}, nil)
	require.False(t, ok)
	require.Zero(t, llm.calls)
}
func TestJSONProviderUsesDedicatedSchemaAndRejectsLengthFinish(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "json_object", body["response_format"].(map[string]any)["type"])
		require.NotContains(t, body["messages"].([]any)[0].(map[string]any)["content"], "每个实质性结论后必须标注")
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"claims\":[]}"},"finish_reason":"length"}]}`))
	}))
	defer server.Close()
	answer, e := (OpenAILLM{BaseURL: server.URL, APIKey: "synthetic", Model: "fixture"}).AnswerJSON(context.Background(), "返回JSON", "教材")
	require.Empty(t, answer)
	var failure *PublicError
	require.ErrorAs(t, e, &failure)
	require.Equal(t, "answer_incomplete", failure.Code)
}
func TestStandaloneQuestionDoesNotInheritUnrelatedHistory(t *testing.T) {
	q := "为什么需要虚拟内存？"
	require.Equal(t, q, contextualQuestion(q, []domain.Message{{Role: "user", Content: "TCP是什么？"}}))
	require.Contains(t, contextualQuestion("它为什么需要重传？", []domain.Message{{Role: "user", Content: "TCP是什么？"}}), "TCP")
}
func TestStreamLengthFinishIsNotACompletedAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"那部分\"},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n\n"))
	}))
	defer server.Close()
	answer, e := (OpenAILLM{BaseURL: server.URL, APIKey: "synthetic"}).AnswerStream(context.Background(), "q", "e", nil, func(string) error { return nil })
	require.Error(t, e)
	require.Equal(t, "", strings.TrimSpace(answer))
}

func TestMultipleClaimsKeepAllQuotesFromSameSource(t *testing.T) {
	first := "内存容量有限会限制程序的运行。"
	second := "虚拟内存允许按需装入程序和数据。"
	s := RAGService{LLM: &reviewLLM{answer: `{"supported":true}`}}
	raw, _ := json.Marshal(map[string]any{"claims": []map[string]any{
		{"text": first, "kind": "explanation", "evidence": []map[string]any{{"source": 1, "quote": first}}},
		{"text": second, "kind": "explanation", "evidence": []map[string]any{{"source": 1, "quote": second}}},
	}})
	_, citations, ok := s.composedAnswer(context.Background(), string(raw), "解释虚拟内存。", []string{first + second}, []domain.Citation{{DocumentID: "book", Page: 1}}, nil)
	require.True(t, ok)
	require.Len(t, citations, 1)
	require.Contains(t, citations[0].Excerpt, first)
	require.Contains(t, citations[0].Excerpt, second)
}
