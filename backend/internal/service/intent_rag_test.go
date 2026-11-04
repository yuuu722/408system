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

type intentStore struct {
	*testutil.Store
	extra []domain.Chunk
	kids  []string
}

func (s *intentStore) SearchTextbookEvidence(_ context.Context, _ string, kid string, topics, needs []string, _ int) ([]domain.Chunk, error) {
	s.kids = append(s.kids, kid)
	return s.extra, nil
}
func TestWhyQuestionRecoversTraditionalMemoryReasonOutsideVectorHits(t *testing.T) {
	ctx := context.Background()
	st := &intentStore{Store: testutil.NewStore()}
	kb, e := st.CreateKnowledgeBase(ctx, "u", "教材", "")
	require.NoError(t, e)
	reason := "传统存储管理要求作业一次性全部装入并驻留在内存中。作业大于可用物理内存时无法运行，暂时不用的程序也会占用内存。"
	mechanism := "基于局部性原理，应用程序只需将当前要运行的少数页面装入内存便可运行，其余部分暂留磁盘。"
	for _, id := range []string{"cause", "mechanism"} {
		st.Docs[id] = domain.Document{ID: id, UserID: "u", KnowledgeBaseID: kb.ID, Status: "ready"}
	}
	st.Chunks = []domain.Chunk{{ID: "m", DocumentID: "mechanism", UserID: "u", KnowledgeBaseID: kb.ID, Content: mechanism, Embedding: []float32{1, 0}, Metadata: map[string]any{"resource_type": "textbook", "heading_path": "3.2 虚拟内存"}}}
	st.extra = []domain.Chunk{{ID: "r", DocumentID: "cause", UserID: "u", KnowledgeBaseID: kb.ID, Content: reason, Score: 1, Metadata: map[string]any{"resource_type": "textbook", "heading_path": "3.2 虚拟内存"}}}
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Messages []struct{ Content string } }
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		last := body.Messages[1].Content
		content := `{"supported":true}`
		if !strings.Contains(last, `"supported"`) {
			require.Contains(t, last, "一次性")
			require.Contains(t, last, "驻留")
			require.Contains(t, last, "局部性")
			content = `{"claims":[{"text":"传统存储管理要求作业一次性装入并驻留，作业大小受到物理内存容量限制，暂时不用的部分也会占用内存。虚拟内存基于局部性，只需调入当前使用的部分，其余部分留在磁盘。","kind":"explanation","facts":[1,2,3]}]}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}, "finish_reason": "stop"}}})
	}))
	defer model.Close()
	rag := service.RAGService{Store: st, Embedder: fixedEmbedding{}, LLM: service.OpenAILLM{BaseURL: model.URL, APIKey: "fixture"}, Knowledge: service.KnowledgeService{Store: st}, Learning: service.LearningService{Store: st}, TopK: 3, MinScore: .35}
	result, e := rag.Chat(ctx, "u", kb.ID, "", "为什么需要虚拟内存？")
	require.NoError(t, e)
	require.Equal(t, []string{kb.ID}, st.kids)
	require.NotContains(t, result.Answer, "原句（不是模型生成答案）")
	require.Contains(t, result.Answer, "物理内存容量限制")
	require.NotNil(t, result.Generation)
	require.True(t, result.Generation.Attempted)
	require.True(t, result.Generation.ProviderReturned)
	require.True(t, result.Generation.Verified)
	require.Equal(t, 1, result.Generation.Attempts)
	seen := map[string]bool{}
	for _, s := range result.Sources {
		seen[s.DocumentID] = true
	}
	require.True(t, seen["cause"])
	require.True(t, seen["mechanism"])
}
func TestGenerationTraceShowsReturnedModelButInvalidReview(t *testing.T) {
	ctx := context.Background()
	st := testutil.NewStore()
	kb, e := st.CreateKnowledgeBase(ctx, "u", "教材", "")
	require.NoError(t, e)
	source := "虚拟内存允许作业按需调入内存。"
	st.Docs["d"] = domain.Document{ID: "d", UserID: "u", KnowledgeBaseID: kb.ID, Status: "ready"}
	st.Chunks = []domain.Chunk{{ID: "c", DocumentID: "d", UserID: "u", KnowledgeBaseID: kb.ID, Content: source, Embedding: []float32{1, 0}}}
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Messages []struct{ Content string } }
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		content := `{"claims":[{"text":"虚拟内存支持作业按需调入内存。","facts":[1]}]}`
		if strings.Contains(body.Messages[1].Content, `"supported"`) {
			content = `{"verdict":"PRIVATE"}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}, "finish_reason": "stop"}}})
	}))
	defer model.Close()
	rag := service.RAGService{Store: st, Embedder: fixedEmbedding{}, LLM: service.OpenAILLM{BaseURL: model.URL, APIKey: "fixture"}, Knowledge: service.KnowledgeService{Store: st}, Learning: service.LearningService{Store: st}, TopK: 3, MinScore: .35}
	result, e := rag.Chat(ctx, "u", kb.ID, "", "解释虚拟内存")
	require.NoError(t, e)
	require.True(t, result.Generation.Attempted)
	require.True(t, result.Generation.ProviderReturned)
	require.False(t, result.Generation.Verified)
	require.Equal(t, "invalid_review_format", result.Generation.FailureCode)
	require.Contains(t, result.Generation.Notice, "格式无效")
	require.NotContains(t, result.Generation.Notice, "PRIVATE")
	require.NotContains(t, result.Answer, "PRIVATE")
}

func TestInvalidReviewIsRetriedWithoutRegeneratingSupportedAnswer(t *testing.T) {
	ctx := context.Background()
	st := testutil.NewStore()
	kb, e := st.CreateKnowledgeBase(ctx, "u", "教材", "")
	require.NoError(t, e)
	source := "虚拟内存允许作业按需调入内存。"
	st.Docs["d"] = domain.Document{ID: "d", UserID: "u", KnowledgeBaseID: kb.ID, Status: "ready"}
	st.Chunks = []domain.Chunk{{ID: "c", DocumentID: "d", UserID: "u", KnowledgeBaseID: kb.ID, Content: source, Embedding: []float32{1, 0}}}
	generationCalls, reviewCalls := 0, 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct{ Content string }
			Format   map[string]any `json:"response_format"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		content := `{"claims":[{"text":"虚拟内存支持作业按需调入内存。","facts":[1]}]}`
		if strings.Contains(body.Messages[1].Content, `"supported"`) {
			reviewCalls++
			require.Equal(t, "json_schema", body.Format["type"])
			content = `{"supported":true}`
			if reviewCalls == 1 {
				content = `{"wrong_field":true}`
			}
		} else {
			generationCalls++
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}, "finish_reason": "stop"}}})
	}))
	defer model.Close()
	rag := service.RAGService{Store: st, Embedder: fixedEmbedding{}, LLM: service.OpenAILLM{BaseURL: model.URL, APIKey: "fixture"}, Knowledge: service.KnowledgeService{Store: st}, Learning: service.LearningService{Store: st}, TopK: 3, MinScore: .35}
	result, e := rag.Chat(ctx, "u", kb.ID, "", "解释虚拟内存")
	require.NoError(t, e)
	require.Equal(t, 1, generationCalls)
	require.Equal(t, 2, reviewCalls)
	require.True(t, result.Generation.Verified)
	require.Equal(t, 1, result.Generation.Attempts)
	require.NotContains(t, result.Answer, "原句（不是模型生成答案）")
}
