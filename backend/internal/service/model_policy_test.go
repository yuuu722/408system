package service_test

import (
	"context"
	"fmt"
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

type editorialLLM struct{ answer string }

func (p editorialLLM) Answer(context.Context, string, string) (string, error) { return p.answer, nil }

func TestRAGWithholdsInvalidCitationsAndReturnsOriginalEvidence(t *testing.T) {
	for _, answer := range []string{"模型编造的内容", "模型编造的内容[99]"} {
		ctx := context.Background()
		st := testutil.NewStore()
		kb, err := st.CreateKnowledgeBase(ctx, "u1", "操作系统", "")
		require.NoError(t, err)
		st.Docs["d1"] = domain.Document{ID: "d1", UserID: "u1", KnowledgeBaseID: kb.ID, Status: "ready"}
		st.Chunks = []domain.Chunk{{ID: "c1", DocumentID: "d1", UserID: "u1", KnowledgeBaseID: kb.ID, DocumentTitle: "操作系统教材.txt", Content: "虚拟内存用于地址隔离", SourceLocation: "第1页", Embedding: []float32{1, 0}}}
		rag := service.RAGService{Store: st, Embedder: fixedEmbedding{}, LLM: editorialLLM{answer}, Knowledge: service.KnowledgeService{Store: st}, Learning: service.LearningService{Store: st}, TopK: 5, MinScore: .35}
		var visible strings.Builder
		result, err := rag.ChatStream(ctx, "u1", kb.ID, "", "为什么需要虚拟内存？", func(event string, data any) error {
			if event == "delta" {
				visible.WriteString(data.(map[string]string)["text"])
			}
			return nil
		})
		require.NoError(t, err)
		require.NotContains(t, result.Answer, "模型编造的内容")
		require.Contains(t, result.Answer, "虚拟内存用于地址隔离 [1]")
		require.Equal(t, result.Answer, visible.String(), "unchecked model tokens must not reach the browser")
		require.Len(t, result.Sources, 1)
		require.Equal(t, "c1", result.Sources[0].ChunkID)
		require.Equal(t, "第1页", result.Sources[0].Location)
	}
}

func TestLocalRetrievalDoesNotRejectRelevantLongTextBySemanticCosineThreshold(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewStore()
	kb, err := store.CreateKnowledgeBase(ctx, "u1", "教材", "")
	require.NoError(t, err)
	var body strings.Builder
	body.WriteString("虚拟内存为进程提供独立地址空间，并使用页表转换虚拟地址。\n")
	for i := 0; i < 800; i++ {
		fmt.Fprintf(&body, "syntheticfiller%d ", i)
	}
	embed := service.LexicalEmbedding{Dimension: 1024}
	vectors, err := embed.Embed(ctx, []string{body.String(), "请根据教材说明为什么需要虚拟内存？"})
	require.NoError(t, err)
	var cosine float64
	for i, v := range vectors[0] {
		cosine += float64(v * vectors[1][i])
	}
	require.Less(t, cosine, .35, "fixture must expose the long-text cosine dilution problem")
	store.Docs["book"] = domain.Document{ID: "book", UserID: "u1", KnowledgeBaseID: kb.ID, Status: "ready"}
	store.Chunks = []domain.Chunk{{ID: "part", DocumentID: "book", KnowledgeBaseID: kb.ID, UserID: "u1", Content: body.String(), Embedding: vectors[0]}}
	rag := service.RAGService{Store: store, Embedder: embed, LLM: editorialLLM{"虚拟内存为进程提供独立地址空间。[1]"}, Knowledge: service.KnowledgeService{Store: store}, Learning: service.LearningService{Store: store}, TopK: 5, MinScore: .35}
	answer, err := rag.Chat(ctx, "u1", kb.ID, "", "请根据教材说明为什么需要虚拟内存？")
	require.NoError(t, err)
	require.Len(t, answer.Sources, 1)
	require.Contains(t, answer.Answer, "[1]")
}
