package service_test

import (
	"context"
	"encoding/json"
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

func TestActualModelEvidenceBudgetKeepsWholeSourceGroups(t *testing.T) {
	ctx := context.Background()
	st := testutil.NewStore()
	kb, e := st.CreateKnowledgeBase(ctx, "u", "教材", "")
	require.NoError(t, e)
	for i := 0; i < 3; i++ {
		id := fmt.Sprint(i)
		st.Docs[id] = domain.Document{ID: id, UserID: "u", KnowledgeBaseID: kb.ID, Status: "ready"}
		text := fmt.Sprintf("TCP通过序号、确认和重传实现可靠传输，机制%d需要说明。", i) + strings.Repeat("发送方等待接收方确认，超时后重传未确认的数据。", 45)
		st.Chunks = append(st.Chunks, domain.Chunk{ID: id, DocumentID: id, UserID: "u", KnowledgeBaseID: kb.ID, Content: text, Embedding: []float32{1, 0}})
	}
	calls := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct{ Messages []struct{ Content string } }
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		input := body.Messages[len(body.Messages)-1].Content
		require.Less(t, len([]rune(input)), 4500)
		require.NotContains(t, input, "[3]")
		w.WriteHeader(500)
	}))
	defer model.Close()
	rag := service.RAGService{Store: st, Embedder: fixedEmbedding{}, LLM: service.OpenAILLM{BaseURL: model.URL, APIKey: "fixture"}, Knowledge: service.KnowledgeService{Store: st}, Learning: service.LearningService{Store: st}, TopK: 5, MinScore: .35}
	result, e := rag.Chat(ctx, "u", kb.ID, "", "TCP如何实现可靠传输？")
	require.NoError(t, e)
	require.Equal(t, 1, calls)
	require.NotEmpty(t, result.Sources)
}
