package service_test

import (
	"context"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/service"
	"github.com/local/exam408-assistant/backend/internal/testutil"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestAuthRegisterLoginAndJWT(t *testing.T) {
	st := testutil.NewStore()
	a := service.AuthService{Store: st, Secret: []byte("12345678901234567890123456789012"), TTL: time.Hour}
	u, tok, err := a.Register(context.Background(), "alice", "alice@example.com", "password1")
	require.NoError(t, err)
	require.NotEmpty(t, tok)
	got, tok2, err := a.Login(context.Background(), "alice", "password1")
	require.NoError(t, err)
	require.Equal(t, u.ID, got.ID)
	id, err := a.Parse(tok2)
	require.NoError(t, err)
	require.Equal(t, u.ID, id)
}
func TestKnowledgeBaseAndUserIsolation(t *testing.T) {
	st := testutil.NewStore()
	k, _ := st.CreateKnowledgeBase(context.Background(), "user-a", "OS", "")
	_, err := st.KnowledgeBase(context.Background(), "user-b", k.ID)
	require.Error(t, err)
	docs, err := st.ListDocuments(context.Background(), "user-b", k.ID)
	require.NoError(t, err)
	require.Empty(t, docs)
}
func TestDocumentChunkingPreservesOverlap(t *testing.T) {
	input := strings.Repeat("虚拟内存用于扩展地址空间。", 120)
	chunks := service.ChunkText(input, 120, 20)
	require.Greater(t, len(chunks), 2)
	require.LessOrEqual(t, len([]rune(chunks[0])), 122)
}
func TestVectorRetrievalOrdersAndIsolates(t *testing.T) {
	st := testutil.NewStore()
	st.Docs["d1"] = domain.Document{ID: "d1", UserID: "u1", KnowledgeBaseID: "k1", Status: "ready"}
	st.Docs["d2"] = domain.Document{ID: "d2", UserID: "u1", KnowledgeBaseID: "k1", Status: "ready"}
	st.Docs["dx"] = domain.Document{ID: "dx", UserID: "u2", KnowledgeBaseID: "k1", Status: "ready"}
	st.Chunks = []domain.Chunk{{ID: "a", DocumentID: "d1", UserID: "u1", KnowledgeBaseID: "k1", Embedding: []float32{1, 0}, Content: "best"}, {ID: "b", DocumentID: "d2", UserID: "u1", KnowledgeBaseID: "k1", Embedding: []float32{0, 1}, Content: "other"}, {ID: "x", DocumentID: "dx", UserID: "u2", KnowledgeBaseID: "k1", Embedding: []float32{1, 0}, Content: "secret"}}
	got, err := st.SearchChunks(context.Background(), "u1", "k1", []float32{1, 0}, 2)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "best", got[0].Content)
	require.NotEqual(t, "secret", got[0].Content)
}
func TestVectorRetrievalExcludesNonReadyDocuments(t *testing.T) {
	st := testutil.NewStore()
	st.Docs["ready"] = domain.Document{ID: "ready", UserID: "u1", KnowledgeBaseID: "k1", Status: "ready"}
	st.Docs["failed"] = domain.Document{ID: "failed", UserID: "u1", KnowledgeBaseID: "k1", Status: "failed"}
	st.Chunks = []domain.Chunk{
		{ID: "safe", DocumentID: "ready", UserID: "u1", KnowledgeBaseID: "k1", Embedding: []float32{.8, .2}, Content: "ready evidence"},
		{ID: "stale", DocumentID: "failed", UserID: "u1", KnowledgeBaseID: "k1", Embedding: []float32{1, 0}, Content: "stale evidence"},
	}
	got, err := st.SearchChunks(context.Background(), "u1", "k1", []float32{1, 0}, 5)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "ready evidence", got[0].Content)
}
func TestKnowledgePointDetectionAndRelation(t *testing.T) {
	st := testutil.NewStore()
	svc := service.KnowledgeService{Store: st}
	points, err := svc.Detect(context.Background(), "为什么虚拟内存需要页面置换")
	require.NoError(t, err)
	require.Equal(t, "p1", points[0].ID)
	rels, err := st.PointRelations(context.Background(), "p1")
	require.NoError(t, err)
	require.Equal(t, "prerequisite", rels[0].RelationType)
}
func TestLearningMasteryWeaknessAndRecommendation(t *testing.T) {
	st := testutil.NewStore()
	learn := service.LearningService{Store: st}
	for i := 0; i < 3; i++ {
		_, err := learn.Record(context.Background(), domain.LearningEvent{UserID: "u1", KnowledgePointID: "p1", EventType: "quiz_wrong"})
		require.NoError(t, err)
	}
	weak, _ := st.ListMastery(context.Background(), "u1", true)
	require.Len(t, weak, 1)
	require.Greater(t, weak[0].WeaknessScore, .5)
	recs, err := (service.RecommendationService{Store: st}).Generate(context.Background(), "u1")
	require.NoError(t, err)
	require.NotEmpty(t, recs)
	require.Contains(t, recs[0].Reason, "掌握度")
}

func TestPublicLearningEventsCannotForgeOutcomes(t *testing.T) {
	st := testutil.NewStore()
	learn := service.LearningService{Store: st}
	_, err := learn.RecordUserEvent(context.Background(), domain.LearningEvent{UserID: "u1", KnowledgePointID: "p1", EventType: "quiz_correct"})
	require.ErrorIs(t, err, service.ErrUntrustedLearningEvent)
	require.Empty(t, st.Events)

	_, err = learn.RecordUserEvent(context.Background(), domain.LearningEvent{UserID: "u1", KnowledgePointID: "p1", EventType: "knowledge_point_view"})
	require.NoError(t, err)
	require.Len(t, st.Events, 1)
}
func TestRAGReturnsCitationAndLearningEvent(t *testing.T) {
	st := testutil.NewStore()
	kb, _ := st.CreateKnowledgeBase(context.Background(), "u1", "OS", "")
	st.Docs["d1"] = domain.Document{ID: "d1", UserID: "u1", KnowledgeBaseID: kb.ID, Status: "ready"}
	st.Chunks = []domain.Chunk{{ID: "c1", DocumentID: "d1", DocumentTitle: "os.md", UserID: "u1", KnowledgeBaseID: kb.ID, Content: "虚拟内存使用磁盘扩展逻辑地址空间。", SourceLocation: "chunk 1", Embedding: []float32{1, 0}}}
	emb := fixedEmbedding{}
	learn := service.LearningService{Store: st}
	rag := service.RAGService{Store: st, Embedder: emb, LLM: fixedLLM{}, Knowledge: service.KnowledgeService{Store: st}, Learning: learn, TopK: 3, MinScore: 0}
	got, err := rag.Chat(context.Background(), "u1", kb.ID, "", "为什么需要虚拟内存")
	require.NoError(t, err)
	require.Len(t, got.Sources, 1)
	require.NotEmpty(t, got.Answer)
	require.NotEmpty(t, st.Events)
}

type fixedEmbedding struct{}

func (fixedEmbedding) Embed(context.Context, []string) ([][]float32, error) {
	return [][]float32{{1, 0}}, nil
}

type fixedLLM struct{}

func (fixedLLM) Answer(_ context.Context, _ string, e string) (string, error) {
	return "answer: " + e, nil
}
