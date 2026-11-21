package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/service"
	"github.com/local/exam408-assistant/backend/internal/testutil"
	"github.com/stretchr/testify/require"
)

type fixtureSearch struct {
	query string
	fail  bool
}

func (s *fixtureSearch) Search(_ context.Context, question string) ([]service.WebResult, error) {
	s.query = question
	if s.fail {
		return nil, errors.New("search unavailable")
	}
	return []service.WebResult{
		{Title: "TCP · RFC 9293", URL: "https://www.rfc-editor.org/rfc/rfc9293.html", Text: "TCP uses a sequence number to identify bytes. The TCP acknowledgment number indicates the next sequence number expected. On timeout TCP retransmits unacknowledged data."},
		{Title: "TCP", URL: "https://rfc-editor.org.invalid/example", Text: "TCP uses a magic sequence number."},
	}, nil
}

func TestAnswerCombinesKnowledgeAndWebAndFallsBackWhenSearchFails(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "hybrid", true: "knowledge fallback"}[fail], func(t *testing.T) {
			ctx := context.Background()
			store := testutil.NewStore()
			kb, err := store.CreateKnowledgeBase(ctx, "u", "教材", "")
			require.NoError(t, err)
			source := "TCP 通过字节序号将数据按序交付，实现可靠传输。"
			store.Docs["book"] = domain.Document{ID: "book", UserID: "u", KnowledgeBaseID: kb.ID, Status: "ready"}
			store.Chunks = []domain.Chunk{{ID: "chunk", DocumentID: "book", DocumentTitle: "教材.pdf", UserID: "u", KnowledgeBaseID: kb.ID, Content: source, Embedding: []float32{1, 0}, Metadata: map[string]any{"resource_type": "textbook", "page": 8}}}
			search := &fixtureSearch{fail: fail}
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct{ Messages []struct{ Content string } }
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				last := body.Messages[len(body.Messages)-1].Content
				content := `{"supported":true}`
				if !strings.Contains(last, `"supported"`) {
					require.Contains(t, last, "通过字节序号")
					require.NotContains(t, last, "magic sequence number")
					content = `{"claims":[{"text":"TCP 通过字节序号将数据按序交付。","facts":[1]}]}`
					if !fail {
						require.Contains(t, last, "The TCP acknowledgment number")
						content = `{"claims":[{"text":"TCP 通过字节序号将数据按序交付。","facts":[1]},{"text":"确认号表示下一个期望的序号；超时后重传未确认的数据。","facts":[3,4]}]}`
					}
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}, "finish_reason": "stop"}}})
			}))
			defer model.Close()
			rag := service.RAGService{Web: search, Store: store, Embedder: fixedEmbedding{}, LLM: service.OpenAILLM{BaseURL: model.URL, APIKey: "fixture", Model: "fixture"}, Knowledge: service.KnowledgeService{Store: store}, Learning: service.LearningService{Store: store}, TopK: 3, MinScore: .35}
			question := "TCP 如何实现可靠传输？"
			result, err := rag.Chat(ctx, "u", kb.ID, "", question)
			require.NoError(t, err)
			require.Equal(t, question, search.query)
			require.Contains(t, result.Answer, "通过字节序号")
			require.NotEmpty(t, result.Sources)
			require.Equal(t, "book", result.Sources[0].DocumentID)
			if fail {
				require.False(t, result.Generation.Verified)
				require.Equal(t, "failed", result.Reasoning.WebStatus)
				require.Len(t, result.Sources, 1)
			} else {
				require.True(t, result.Generation.Verified, result.Generation)
				require.Equal(t, "results", result.Reasoning.WebStatus)
				require.Len(t, result.Sources, 2)
				require.Equal(t, "https://www.rfc-editor.org/rfc/rfc9293.html", result.Sources[1].WebURL)
			}
		})
	}
}
