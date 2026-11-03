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

func TestTCPQuestionDoesNotPresentHDLCWhenModelFailsOrSucceeds(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "model composition", true: "provider500"}[failure], func(t *testing.T) {
			ctx := context.Background()
			st := testutil.NewStore()
			kb, e := st.CreateKnowledgeBase(ctx, "u", "教材", "")
			require.NoError(t, e)
			link := "在通信线路质量较差的年代，HDLC使用确认和重传机制实现可靠传输。对于无线传输链路，数据链路层协议也提供可靠传输。"
			source := "TCP通过序号、确认和重传机制实现可靠传输。发送方对字节流编号，接收方通过确认号说明期望收到的数据。若等待确认超时，发送方重传尚未确认的数据。"
			for _, id := range []string{"link", "tcp"} {
				st.Docs[id] = domain.Document{ID: id, UserID: "u", KnowledgeBaseID: kb.ID, Status: "ready"}
			}
			st.Chunks = []domain.Chunk{{ID: "link", DocumentID: "link", UserID: "u", KnowledgeBaseID: kb.ID, Content: link, Embedding: []float32{1, 0}, Metadata: map[string]any{"resource_type": "textbook", "is_public": true}}, {ID: "tcp", DocumentID: "tcp", UserID: "u", KnowledgeBaseID: kb.ID, Content: source, Embedding: []float32{1, 0}, Metadata: map[string]any{"resource_type": "textbook", "is_public": true}}}
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct{ Messages []struct{ Content string } }
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				last := body.Messages[len(body.Messages)-1].Content
				require.NotContains(t, last, "HDLC使用")
				require.NotContains(t, last, "无线传输链路")
				if failure {
					w.WriteHeader(500)
					w.Write([]byte(`{"error":"model requires more system memory PRIVATE_PATH"}`))
					return
				}
				content := `{"supported":true}`
				if !strings.Contains(last, `"supported"`) {
					raw, _ := json.Marshal(map[string]any{"claims": []any{map[string]any{"text": source, "kind": "explanation", "evidence": []any{map[string]any{"source": 1, "quote": source}}}}})
					content = string(raw)
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}, "finish_reason": "stop"}}})
			}))
			defer model.Close()
			rag := service.RAGService{Store: st, Embedder: fixedEmbedding{}, LLM: service.OpenAILLM{BaseURL: model.URL, APIKey: "fixture", Model: "fixture"}, Knowledge: service.KnowledgeService{Store: st}, Learning: service.LearningService{Store: st}, TopK: 5, MinScore: .35}
			result, e := rag.Chat(ctx, "u", kb.ID, "", "TCP 如何实现可靠传输？")
			require.NoError(t, e)
			require.Contains(t, result.Answer, "序号")
			require.Contains(t, result.Answer, "确认")
			require.Contains(t, result.Answer, "重传")
			require.NotContains(t, result.Answer, "HDLC")
			require.NotContains(t, result.Answer, "无线")
			require.NotContains(t, result.Answer, "PRIVATE")
			require.NotEmpty(t, result.Sources)
			for _, s := range result.Sources {
				require.Equal(t, "tcp", s.DocumentID)
			}
			if failure {
				require.Contains(t, result.Answer, "内存或显存不足")
				require.Contains(t, result.Answer, "不是模型生成答案")
			} else {
				require.NotContains(t, result.Answer, "模型整理未通过")
			}
		})
	}
}
func TestCheckProvidersUsesJSONAndRejectsFalseOrInvalidProbe(t *testing.T) {
	for _, content := range []string{`{"ok":true}`, `not JSON`, `{"ok":false}`} {
		model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.NotNil(t, body["response_format"])
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}, "finish_reason": "stop"}}})
		}))
		report := (service.RAGService{Embedder: fixedEmbedding{}, LLM: service.OpenAILLM{BaseURL: model.URL, APIKey: "fixture"}}).CheckProviders(context.Background())
		require.Equal(t, content == `{"ok":true}`, report.Passed)
		model.Close()
	}
}

type topicGraphStore struct {
	*testutil.Store
	seeds []string
}

func (s *topicGraphStore) RetrieveGraphEvidence(_ context.Context, _ string, _ string, terms []string) (domain.GraphRetrieval, error) {
	s.seeds = append([]string{}, terms...)
	tcp := domain.GraphNode{ID: "tcp", Name: "TCP可靠传输与拥塞控制"}
	udp := domain.GraphNode{ID: "udp", Name: "UDP与传输层端口"}
	layer := domain.GraphNode{ID: "layer", Name: "传输层"}
	p := domain.KnowledgePath{Nodes: []domain.GraphNode{tcp, layer}, Edges: []domain.GraphEdge{{RelationType: "part_of"}}}
	return domain.GraphRetrieval{Paths: []domain.KnowledgePath{p, p, {Nodes: []domain.GraphNode{udp, layer, tcp}}}, Terms: []string{"UDP", "HDLC", "传输层"}, Chunks: s.Chunks}, nil
}

type topicEmbedding struct{ queries []string }

func (e *topicEmbedding) Embed(_ context.Context, texts []string) ([][]float32, error) {
	e.queries = append(e.queries, texts...)
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = []float32{1, 0}
	}
	return out, nil
}
func TestKAGSeedsAndExpandedQueriesStayOnAskedProtocol(t *testing.T) {
	ctx := context.Background()
	store := &topicGraphStore{Store: testutil.NewStore()}
	kb, e := store.CreateKnowledgeBase(ctx, "u", "教材", "")
	require.NoError(t, e)
	source := "TCP通过序号、确认和重传机制实现可靠传输。"
	store.Docs["d"] = domain.Document{ID: "d", UserID: "u", KnowledgeBaseID: kb.ID, Status: "ready"}
	store.Chunks = []domain.Chunk{{ID: "c", DocumentID: "d", UserID: "u", KnowledgeBaseID: kb.ID, Content: source, Embedding: []float32{1, 0}}}
	embed := &topicEmbedding{}
	rag := service.RAGService{KAGEnabled: true, Store: store, Embedder: embed, LLM: editorialLLM{source}, Knowledge: service.KnowledgeService{Store: store}, Learning: service.LearningService{Store: store}, TopK: 3, MinScore: .35}
	result, e := rag.Chat(ctx, "u", kb.ID, "", "TCP 如何实现可靠传输？")
	require.NoError(t, e)
	require.Equal(t, []string{"TCP"}, store.seeds)
	require.NotNil(t, result.Reasoning)
	require.Len(t, result.Reasoning.Paths, 1)
	for _, q := range embed.queries {
		require.NotContains(t, q, "UDP")
		require.NotContains(t, q, "HDLC")
	}
}
func TestTCPDefinitionOnlyCannotBePresentedAsMechanismAnswer(t *testing.T) {
	ctx := context.Background()
	st := testutil.NewStore()
	kb, e := st.CreateKnowledgeBase(ctx, "u", "教材", "")
	require.NoError(t, e)
	source := "TCP能够实现可靠传输，确保接收方读取的字节流与发送方一致。"
	st.Docs["d"] = domain.Document{ID: "d", UserID: "u", KnowledgeBaseID: kb.ID, Status: "ready"}
	st.Chunks = []domain.Chunk{{ID: "c", DocumentID: "d", UserID: "u", KnowledgeBaseID: kb.ID, Content: source, Embedding: []float32{1, 0}}}
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer model.Close()
	rag := service.RAGService{Store: st, Embedder: fixedEmbedding{}, LLM: service.OpenAILLM{BaseURL: model.URL, APIKey: "fixture"}, Knowledge: service.KnowledgeService{Store: st}, Learning: service.LearningService{Store: st}, TopK: 3, MinScore: .35}
	result, e := rag.Chat(ctx, "u", kb.ID, "", "TCP 如何实现可靠传输？")
	require.NoError(t, e)
	require.Contains(t, result.Answer, "不足以完整解释TCP")
	require.NotContains(t, result.Answer, "确保接收方读取")
	require.NotContains(t, result.Answer, "完整原句（不是模型生成答案）")
}
