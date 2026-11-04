package repository

import (
	"context"
	"encoding/json"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLegacyAdvertisingNodesAndIncidentEdgesAreQuarantined(t *testing.T) {
	g := domain.KnowledgeGraph{Nodes: []domain.GraphNode{{ID: "a", Name: "虚拟内存", Source: "canonical"}, {ID: "b", Name: "Seas 唯一官方淘宝店", Source: "rule_candidate"}, {ID: "c", Name: "WES", Source: "rule_candidate"}, {ID: "d", Name: "页表", Source: "llm_candidate"}}, Edges: []domain.GraphEdge{{ID: "ab", FromEntityID: "a", ToEntityID: "b"}, {ID: "ac", FromEntityID: "a", ToEntityID: "c"}, {ID: "ad", FromEntityID: "a", ToEntityID: "d"}}}
	out := qualityGraph(g)
	require.Len(t, out.Nodes, 2)
	require.Len(t, out.Edges, 1)
	require.Equal(t, "ad", out.Edges[0].ID)
	require.Len(t, g.Nodes, 4)
}

func TestOldNeo4jPathsCannotInjectAdvertisingOrCooccurrence(t *testing.T) {
	good := domain.GraphNode{ID: "a", Name: "虚拟内存", Source: "canonical"}
	ad := domain.GraphNode{ID: "b", Name: "买纸质版认准竟成唯一官方淘宝店", Source: "rule_candidate"}
	page := domain.GraphNode{ID: "c", Name: "页表", Source: "llm_candidate"}
	encode := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rows := []map[string]any{
			{"row": []any{[]string{encode(good), encode(ad)}, []string{encode(domain.GraphEdge{Source: "llm_candidate"})}}},
			{"row": []any{[]string{encode(good), encode(page)}, []string{encode(domain.GraphEdge{Source: "rule_candidate"})}}},
			{"row": []any{[]string{encode(good), encode(page)}, []string{encode(domain.GraphEdge{Source: "llm_candidate"})}}},
		}
		json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"data": rows}}, "errors": []any{}})
	}))
	defer server.Close()
	paths, e := (&Neo4jGraph{URL: server.URL}).Paths(context.Background(), "fixture", []string{"虚拟内存"})
	require.NoError(t, e)
	require.Len(t, paths, 1)
	require.Equal(t, "页表", paths[0].Nodes[1].Name)
}

func TestNonNounLegacyNodesCannotAppearInGraphOrPaths(t *testing.T) {
	g := domain.KnowledgeGraph{Nodes: []domain.GraphNode{
		{ID: "a", Name: "线性表", Source: "canonical"},
		{ID: "b", Name: "一个结点包括三部分内容", Source: "llm_candidate"},
		{ID: "c", Name: "发生异常的可能", Source: "rule_candidate", Description: "表示发生异常的可能。"},
		{ID: "d", Name: "循环队列", Source: "llm_candidate"},
	}, Edges: []domain.GraphEdge{{FromEntityID: "a", ToEntityID: "b"}, {FromEntityID: "a", ToEntityID: "c"}, {FromEntityID: "a", ToEntityID: "d"}}}
	filtered := qualityGraph(g)
	require.Len(t, filtered.Nodes, 2)
	require.Len(t, filtered.Edges, 1)
	require.Equal(t, "d", filtered.Edges[0].ToEntityID)
	require.Len(t, g.Nodes, 4, "original evidence retained")
}
