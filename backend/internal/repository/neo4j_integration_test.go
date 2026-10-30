package repository_test

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/repository"
	"github.com/local/exam408-assistant/backend/internal/service"
	"github.com/local/exam408-assistant/backend/migrations"
	"github.com/stretchr/testify/require"
	"net/url"
	"os"
	"strings"
	"testing"
)

type quoteOnlyModel struct{ quote string }

func (m quoteOnlyModel) Answer(context.Context, string, string) (string, error) {
	raw, _ := json.Marshal(map[string]any{"passages": []map[string]any{{"source": 1, "quote": m.quote}}})
	return string(raw), nil
}
func TestNeo4jGraphKAGIsolationRecoveryAndDeletion(t *testing.T) {
	raw := os.Getenv("TEST_DATABASE_URL")
	endpoint := os.Getenv("NEO4J_TEST_URL")
	password := os.Getenv("NEO4J_TEST_PASSWORD")
	if raw == "" || endpoint == "" || password == "" {
		t.Skip("isolated PostgreSQL and Neo4j required")
	}
	ctx := context.Background()
	admin, e := repository.Open(ctx, raw)
	require.NoError(t, e)
	defer admin.DB.Close()
	dbName := "exam408_neo4j_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, e = admin.DB.Exec(ctx, `CREATE DATABASE `+dbName)
	require.NoError(t, e)
	dsn, e := url.Parse(raw)
	require.NoError(t, e)
	dsn.Path = "/" + dbName
	p, e := repository.Open(ctx, dsn.String())
	require.NoError(t, e)
	defer func() {
		p.DB.Close()
		_, e := admin.DB.Exec(ctx, `DROP DATABASE `+dbName+` WITH(FORCE)`)
		require.NoError(t, e)
	}()
	require.NoError(t, migrations.Up(ctx, p.DB))
	require.NoError(t, migrations.Up(ctx, p.DB))
	n := &repository.Neo4jGraph{URL: endpoint, Username: "neo4j", Password: password}
	require.NoError(t, n.Initialize(ctx))
	p.Neo4j = n
	p.EmbeddingSpace = "synthetic"
	a, e := p.CreateUser(ctx, domain.User{Username: "graph-a", Email: "a@example.invalid", PasswordHash: "synthetic"})
	require.NoError(t, e)
	b, e := p.CreateUser(ctx, domain.User{Username: "graph-b", Email: "b@example.invalid", PasswordHash: "synthetic"})
	require.NoError(t, e)
	kb, e := p.CreateKnowledgeBase(ctx, a.ID, "private A", "")
	require.NoError(t, e)
	kbB, e := p.CreateKnowledgeBase(ctx, b.ID, "private B", "")
	require.NoError(t, e)
	defer n.Replace(ctx, kb.ID, domain.KnowledgeGraph{})
	defer n.Replace(ctx, kbB.ID, domain.KnowledgeGraph{})
	d, e := p.CreateDocument(ctx, domain.Document{UserID: a.ID, KnowledgeBaseID: kb.ID, Filename: "source.txt", OriginalFilename: "操作系统教材.txt", MimeType: "text/plain", StoragePath: "synthetic", ResourceType: "textbook", Status: "ready"})
	require.NoError(t, e)
	text := "虚拟内存：虚拟内存通过按需调入提高内存利用率，内存管理负责地址空间。\n请求分页：请求分页通过缺页中断调入页面，虚拟内存保留未调入的页面。\n缺页中断：缺页中断由页面尚未装入内存触发，虚拟内存支持这一过程。"
	embed := service.LexicalEmbedding{Dimension: 64}
	vec, e := embed.Embed(ctx, []string{text})
	require.NoError(t, e)
	chunk := domain.Chunk{ID: uuid.NewString(), DocumentID: d.ID, KnowledgeBaseID: kb.ID, UserID: a.ID, Content: text, Embedding: vec[0], Metadata: map[string]any{"page": 1, "embedding_space": "synthetic"}}
	require.NoError(t, p.ReplaceChunks(ctx, a.ID, d.ID, []domain.Chunk{chunk}))
	g, e := p.KnowledgeGraph(ctx, a.ID, kb.ID)
	require.NoError(t, e)
	require.Equal(t, "neo4j", g.Storage)
	require.NotEmpty(t, g.Nodes)
	require.NotEmpty(t, g.Edges)
	_, e = p.KnowledgeGraph(ctx, b.ID, kb.ID)
	require.Error(t, e, "other user cannot read private Neo4j scope")
	secretGraph := domain.KnowledgeGraph{Nodes: []domain.GraphNode{{ID: g.Nodes[0].ID, Name: "仅B可见的秘密节点", Description: "private-B"}}}
	require.NoError(t, n.Replace(ctx, kbB.ID, secretGraph))
	secretPaths, err := n.Paths(ctx, kb.ID, []string{"仅B可见的秘密节点"})
	require.NoError(t, err)
	require.Empty(t, secretPaths)
	attackPaths, err := n.Paths(ctx, kb.ID, []string{"') MATCH (other) DETACH DELETE other //"})
	require.NoError(t, err)
	require.Empty(t, attackPaths)
	stillPresent, err := n.Read(ctx, kb.ID)
	require.NoError(t, err)
	require.Len(t, stillPresent.Nodes, len(g.Nodes))
	paths, e := n.Paths(ctx, kb.ID, []string{"虚拟内存"})
	require.NoError(t, e)
	twoHops := false
	for _, path := range paths {
		if len(path.Edges) == 2 {
			twoHops = true
		}
	}
	require.True(t, twoHops, "real Neo4j must execute two-hop traversal")
	rag := service.RAGService{KAGEnabled: true, Store: p, Embedder: embed, LLM: quoteOnlyModel{"虚拟内存通过按需调入提高内存利用率，内存管理负责地址空间。"}, Knowledge: service.KnowledgeService{Store: p}, Learning: service.LearningService{Store: p}, TopK: 3, MinScore: .35}
	result, e := rag.Chat(ctx, a.ID, kb.ID, "", "为什么需要虚拟内存？")
	require.NoError(t, e)
	require.NotNil(t, result.Reasoning)
	require.NotEmpty(t, result.Reasoning.Paths)
	require.NotEmpty(t, result.Sources)
	require.Contains(t, result.Answer, "虚拟内存")
	for _, c := range result.Sources {
		require.Equal(t, d.ID, c.DocumentID)
	}
	_, e = rag.Chat(ctx, b.ID, kb.ID, "", "为什么需要虚拟内存？")
	require.Error(t, e)
	// Lost graph storage is detected through the namespace generation and rebuilt.
	require.NoError(t, n.Replace(ctx, kb.ID, domain.KnowledgeGraph{}, -1))
	g, e = p.KnowledgeGraph(ctx, a.ID, kb.ID)
	require.NoError(t, e)
	require.NotEmpty(t, g.Nodes)
	p.Neo4j = &repository.Neo4jGraph{URL: endpoint, Username: "neo4j", Password: "incorrect-test-password"}
	_, e = p.KnowledgeGraph(ctx, a.ID, kb.ID)
	require.Error(t, e)
	require.NotContains(t, e.Error(), password)
	p.Neo4j = n
	require.NoError(t, p.DeleteDocument(ctx, a.ID, d.ID))
	g, e = p.KnowledgeGraph(ctx, a.ID, kb.ID)
	require.NoError(t, e)
	for _, node := range g.Nodes {
		require.NotEqual(t, d.ID, node.EvidenceDocumentID)
	}
	require.NoError(t, p.DeleteKnowledgeBase(ctx, a.ID, kb.ID))
	require.NoError(t, p.SyncKnowledgeGraph(ctx, kb.ID))
	g, e = n.Read(ctx, kb.ID)
	require.NoError(t, e)
	require.Empty(t, g.Nodes)
}
