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

func TestFactBasedRealProviderPathReturnsComposedAnswerWithoutModelCopiedQuotes(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewStore()
	kb, e := store.CreateKnowledgeBase(ctx, "u", "教材", "")
	require.NoError(t, e)
	source := "TCP通过检验、序号、确认和重传等机制实现可靠传输。TCP在不可靠的IP层之上提供可靠传输服务，确保接收方从缓存中读取的字节流与发送方发出的字节流完全一致。其中，TCP的检验机制与UDP类似，这里不再痪述。"
	store.Docs["book"] = domain.Document{ID: "book", UserID: "u", KnowledgeBaseID: kb.ID, Status: "ready"}
	store.Chunks = []domain.Chunk{{ID: "chunk", DocumentID: "book", DocumentTitle: "计算机网络教材.pdf", UserID: "u", KnowledgeBaseID: kb.ID, Content: source, Embedding: []float32{1, 0}, Metadata: map[string]any{"resource_type": "textbook", "page": 123}}}
	calls := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct{ Messages []struct{ Content string } }
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		last := body.Messages[len(body.Messages)-1].Content
		content := `{"supported":"true"}`
		if !strings.Contains(last, `"supported"`) {
			require.Contains(t, last, `"facts"`)
			require.NotContains(t, last, "痪述")
			require.Contains(t, last, "不要抄写原文quote")
			content = `{"claims":[{"text":"TCP利用检验、序号、确认和重传机制，在不可靠的IP网络上提供可靠的字节流传输服务。","facts":[1,2]}]}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}, "finish_reason": "stop"}}})
	}))
	defer model.Close()
	rag := service.RAGService{Store: store, Embedder: fixedEmbedding{}, LLM: service.OpenAILLM{BaseURL: model.URL, APIKey: "fixture", Model: "fixture"}, Knowledge: service.KnowledgeService{Store: store}, Learning: service.LearningService{Store: store}, TopK: 3, MinScore: .35}
	result, e := rag.Chat(ctx, "u", kb.ID, "", "TCP如何实现可靠传输？")
	require.NoError(t, e)
	require.Equal(t, 2, calls)
	require.Contains(t, result.Answer, "TCP利用")
	require.NotContains(t, result.Answer, "原句")
	require.NotContains(t, result.Answer, "痪述")
	require.Len(t, result.Sources, 1)
	require.Equal(t, "book", result.Sources[0].DocumentID)
	require.Equal(t, 123, result.Sources[0].Page)
	require.Contains(t, result.Sources[0].Excerpt, "TCP通过检验")
}
