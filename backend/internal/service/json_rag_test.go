package service_test

import (
	"context"
	"encoding/json"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/service"
	"github.com/local/exam408-assistant/backend/internal/testutil"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRealProviderRepairsJSONThenReviewsBeforeShowingAnswer(t *testing.T) {
	for _, repairOK := range []bool{true, false} {
		t.Run(map[bool]string{true: "repair", false: "reject-half"}[repairOK], func(t *testing.T) {
			ctx := context.Background()
			st := testutil.NewStore()
			kb, e := st.CreateKnowledgeBase(ctx, "u", "教材", "")
			require.NoError(t, e)
			source := "虚拟内存利用程序的局部性，只将当前需要的程序和数据调入内存即可运行。"
			st.Docs["d"] = domain.Document{ID: "d", UserID: "u", KnowledgeBaseID: kb.ID, Status: "ready"}
			st.Chunks = []domain.Chunk{{ID: "c", DocumentID: "d", UserID: "u", KnowledgeBaseID: kb.ID, Content: source, Embedding: []float32{1, 0}}}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body struct{ Messages []struct{ Content string } }
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				last := body.Messages[len(body.Messages)-1].Content
				content := "基于局部性原理，仅须将当前要运行的那部分"
				if repairOK && strings.Contains(last, "上次输出") {
					raw, _ := json.Marshal(map[string]any{"claims": []map[string]any{{"text": "虚拟内存利用局部性，让程序按需调入并运行。", "kind": "explanation", "evidence": []map[string]any{{"source": 1, "quote": source}}}}})
					content = string(raw)
				}
				if repairOK && strings.Contains(last, `"supported"`) {
					content = `{"supported":true}`
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": content}, "finish_reason": "stop"}}})
			}))
			defer server.Close()
			rag := service.RAGService{Store: st, Embedder: fixedEmbedding{}, LLM: service.OpenAILLM{BaseURL: server.URL, APIKey: "synthetic", Model: "fixture"}, Knowledge: service.KnowledgeService{Store: st}, Learning: service.LearningService{Store: st}, TopK: 3, MinScore: .35}
			result, e := rag.Chat(ctx, "u", kb.ID, "", "解释虚拟内存")
			require.NoError(t, e)
			require.NotContains(t, result.Answer, "那部分")
			if repairOK {
				require.Contains(t, result.Answer, "程序按需调入")
				require.Equal(t, 3, calls)
			} else {
				require.Contains(t, result.Answer, "完整原句（不是模型生成答案）")
				require.Equal(t, 2, calls)
			}
		})
	}
}

func TestModelFailureStillReturnsCompleteAttributedTextbookSentences(t *testing.T) {
	ctx := context.Background()
	st := testutil.NewStore()
	kb, e := st.CreateKnowledgeBase(ctx, "u", "教材", "")
	require.NoError(t, e)
	source := "传统一次性和驻留性要求作业全部装入并驻留内存，物理内存不足会使程序无法运行。\nSeas 唯一官方淘宝店\n虚 拟 内 存利用局部性，允许只将当前需要的程序和数据调入内存即可运行。"
	st.Docs["d"] = domain.Document{ID: "d", UserID: "u", KnowledgeBaseID: kb.ID, Status: "ready"}
	st.Chunks = []domain.Chunk{{ID: "c", DocumentID: "d", UserID: "u", KnowledgeBaseID: kb.ID, Content: source, Embedding: []float32{1, 0}}}
	calls := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		raw, _ := json.Marshal(body)
		require.NotContains(t, string(raw), "淘宝")
		w.WriteHeader(504)
	}))
	defer model.Close()
	rag := service.RAGService{Store: st, Embedder: fixedEmbedding{}, LLM: service.OpenAILLM{BaseURL: model.URL, APIKey: "synthetic", Model: "fixture"}, Knowledge: service.KnowledgeService{Store: st}, Learning: service.LearningService{Store: st}, TopK: 3, MinScore: .35}
	result, e := rag.Chat(ctx, "u", kb.ID, "", "为什么需要虚拟内存？")
	require.NoError(t, e)
	require.Equal(t, 1, calls)
	require.Contains(t, result.Answer, "完整原句（不是模型生成答案）")
	require.Contains(t, result.Answer, "物理内存不足")
	require.Contains(t, result.Answer, "局部性")
	require.NotContains(t, result.Answer, "淘宝")
	require.NotEmpty(t, result.Sources)
}
