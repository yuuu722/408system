package service_test

import (
	"context"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/service"
	"github.com/local/exam408-assistant/backend/internal/testutil"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProvidersRequireExplicitDemo(t *testing.T) {
	ctx := context.Background()
	_, err := (service.OpenAILLM{}).Answer(ctx, "问题", "资料")
	var public *service.PublicError
	require.ErrorAs(t, err, &public)
	require.Equal(t, "model_not_configured", public.Code)
	_, err = (service.OpenAIEmbedding{Dimension: 1536}).Embed(ctx, []string{"资料"})
	require.ErrorAs(t, err, &public)
	answer, err := (service.OpenAILLM{DemoMode: true}).Answer(ctx, "问题", "资料")
	require.NoError(t, err)
	require.Contains(t, answer, "检索片段预览，非 AI 回答")
	vectors, err := (service.OpenAIEmbedding{Dimension: 1536, DemoMode: true}).Embed(ctx, []string{"资料"})
	require.NoError(t, err)
	require.Len(t, vectors[0], 1536)
}

func TestProviderFailuresNeverFallbackOrLeakResponse(t *testing.T) {
	for _, status := range []int{401, 429, 500} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			w.Write([]byte("SECRET_RESPONSE_BODY"))
		}))
		answer, err := (service.OpenAILLM{BaseURL: server.URL, APIKey: "test"}).Answer(context.Background(), "q", "evidence")
		require.Error(t, err)
		require.Empty(t, answer)
		require.NotContains(t, err.Error(), "SECRET_RESPONSE_BODY")
		_, err = (service.OpenAIEmbedding{BaseURL: server.URL, APIKey: "test", Dimension: 2}).Embed(context.Background(), []string{"q"})
		require.Error(t, err)
		require.NotContains(t, err.Error(), "SECRET_RESPONSE_BODY")
		server.Close()
	}
}

func TestInvalidAndEmptyModelResponsesAreErrors(t *testing.T) {
	for _, body := range []string{"not json", `{"choices":[]}`, `{"choices":[{"message":{"content":" "}}]}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		answer, err := (service.OpenAILLM{BaseURL: server.URL, APIKey: "test"}).Answer(context.Background(), "q", "e")
		require.Error(t, err)
		require.Empty(t, answer)
		server.Close()
	}
}

type forbiddenLLM struct{ t *testing.T }

func (p forbiddenLLM) Answer(context.Context, string, string) (string, error) {
	p.t.Fatal("must not call model for unrelated evidence")
	return "", nil
}
func TestRAGRejectsUnrelatedAndLowScoreEvidenceWithoutCallingLLM(t *testing.T) {
	for _, content := range []string{"连接建立 数据传送 报文 连接释放", ""} {
		st := testutil.NewStore()
		kb, _ := st.CreateKnowledgeBase(context.Background(), "u1", "网络", "")
		st.Docs["d1"] = domain.Document{ID: "d1", UserID: "u1", KnowledgeBaseID: kb.ID, Status: "ready"}
		st.Chunks = []domain.Chunk{{ID: "c1", DocumentID: "d1", UserID: "u1", KnowledgeBaseID: kb.ID, Content: content, Embedding: []float32{1, 0}}}
		rag := service.RAGService{Store: st, Embedder: fixedEmbedding{}, LLM: forbiddenLLM{t}, Knowledge: service.KnowledgeService{Store: st}, Learning: service.LearningService{Store: st}, TopK: 5, MinScore: .35}
		result, err := rag.Chat(context.Background(), "u1", kb.ID, "", "为什么需要虚拟内存？")
		require.NoError(t, err)
		require.Contains(t, result.Answer, "依据不足")
		require.Empty(t, result.Sources)
		st.Chunks[0].Content = "虚拟内存用于地址隔离"
		st.Chunks[0].Embedding = []float32{0, 1}
		result, err = rag.Chat(context.Background(), "u1", kb.ID, "", "为什么需要虚拟内存？")
		require.NoError(t, err)
		require.True(t, strings.Contains(result.Answer, "依据不足"))
		require.Empty(t, result.Sources)
	}
}
