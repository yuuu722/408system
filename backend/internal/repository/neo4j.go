package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"io"
	"net/http"
	"strings"
	"time"
)

// Neo4j stores the queryable graph. PostgreSQL retains validated provenance and
// a durable synchronization journal so graph state can be rebuilt after outages.
type Neo4jGraph struct {
	URL, Username, Password string
	Client                  *http.Client
}
type cypherStatement struct {
	Statement  string         `json:"statement"`
	Parameters map[string]any `json:"parameters"`
}

func (n *Neo4jGraph) query(ctx context.Context, statements ...cypherStatement) ([][]json.RawMessage, error) {
	raw, e := json.Marshal(map[string]any{"statements": statements})
	if e != nil {
		return nil, e
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(n.URL, "/")+"/db/neo4j/tx/commit", bytes.NewReader(raw))
	if e != nil {
		return nil, e
	}
	req.SetBasicAuth(n.Username, n.Password)
	req.Header.Set("Content-Type", "application/json")
	client := n.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	res, e := client.Do(req)
	if e != nil {
		return nil, graphUnavailable()
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, graphUnavailable()
	}
	var body struct {
		Results []struct {
			Data []struct {
				Row []json.RawMessage `json:"row"`
			} `json:"data"`
		} `json:"results"`
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 32<<20)).Decode(&body) != nil || len(body.Errors) > 0 || len(body.Results) != len(statements) {
		return nil, graphUnavailable()
	}
	rows := [][]json.RawMessage{}
	for _, result := range body.Results {
		for _, d := range result.Data {
			rows = append(rows, d.Row)
		}
	}
	return rows, nil
}
func graphUnavailable() error {
	return &domain.RequestError{Status: 503, Message: "Neo4j图谱服务不可用或同步失败，请检查图谱服务后重试"}
}
func (n *Neo4jGraph) Initialize(ctx context.Context) error {
	_, e := n.query(ctx, cypherStatement{Statement: `CREATE CONSTRAINT exam408_graph_node IF NOT EXISTS FOR (n:Exam408Entity) REQUIRE (n.scope,n.id) IS UNIQUE`})
	return e
}
func (n *Neo4jGraph) Replace(ctx context.Context, kid string, g domain.KnowledgeGraph, versions ...int64) error {
	nodes := []map[string]any{}
	edges := []map[string]any{}
	for _, v := range g.Nodes {
		payload, _ := json.Marshal(v)
		nodes = append(nodes, map[string]any{"id": v.ID, "name": v.Name, "payload": string(payload)})
	}
	for _, v := range g.Edges {
		payload, _ := json.Marshal(v)
		edges = append(edges, map[string]any{"id": v.ID, "from": v.FromEntityID, "to": v.ToEntityID, "kind": v.RelationType, "payload": string(payload)})
	}
	version := int64(0)
	if len(versions) > 0 {
		version = versions[0]
	}
	_, e := n.query(ctx,
		cypherStatement{`MERGE (g:Exam408Namespace {scope:$scope}) SET g.version=$version`, map[string]any{"scope": kid, "version": version}},
		cypherStatement{`MATCH (n:Exam408Entity {scope:$scope}) DETACH DELETE n`, map[string]any{"scope": kid}},
		cypherStatement{`UNWIND $nodes AS row CREATE (n:Exam408Entity {scope:$scope,id:row.id,name:row.name,payload:row.payload})`, map[string]any{"scope": kid, "nodes": nodes}},
		cypherStatement{`UNWIND $edges AS row MATCH (a:Exam408Entity {scope:$scope,id:row.from}),(b:Exam408Entity {scope:$scope,id:row.to}) CREATE (a)-[:EXAM408_REL {scope:$scope,id:row.id,kind:row.kind,payload:row.payload}]->(b)`, map[string]any{"scope": kid, "edges": edges}},
	)
	return e
}
func (n *Neo4jGraph) Read(ctx context.Context, kid string) (domain.KnowledgeGraph, error) {
	out := domain.KnowledgeGraph{KnowledgeBaseID: kid, Nodes: []domain.GraphNode{}, Edges: []domain.GraphEdge{}}
	rows, e := n.query(ctx, cypherStatement{`MATCH (g:Exam408Namespace {scope:$scope}) SET g.readCounter=coalesce(g.readCounter,0)+1 WITH g CALL {WITH g MATCH (n:Exam408Entity {scope:$scope}) RETURN collect(n.payload) AS nodes} CALL {WITH g MATCH (:Exam408Entity {scope:$scope})-[r:EXAM408_REL {scope:$scope}]->(:Exam408Entity {scope:$scope}) RETURN collect(r.payload) AS edges} RETURN nodes,edges`, map[string]any{"scope": kid}})
	if e != nil {
		return out, e
	}
	if len(rows) == 0 {
		return out, nil
	}
	if len(rows) != 1 || len(rows[0]) != 2 {
		return out, graphUnavailable()
	}
	var nodes, edges []string
	if json.Unmarshal(rows[0][0], &nodes) != nil || json.Unmarshal(rows[0][1], &edges) != nil {
		return out, graphUnavailable()
	}
	for _, raw := range nodes {
		var v domain.GraphNode
		if json.Unmarshal([]byte(raw), &v) != nil {
			return out, graphUnavailable()
		}
		out.Nodes = append(out.Nodes, v)
	}
	for _, raw := range edges {
		var v domain.GraphEdge
		if json.Unmarshal([]byte(raw), &v) != nil {
			return out, graphUnavailable()
		}
		out.Edges = append(out.Edges, v)
	}
	return out, nil
}

func (n *Neo4jGraph) Paths(ctx context.Context, kid string, terms []string) ([]domain.KnowledgePath, error) {
	rows, e := n.query(ctx, cypherStatement{`MATCH (g:Exam408Namespace {scope:$scope}) SET g.readCounter=coalesce(g.readCounter,0)+1 WITH g MATCH (a:Exam408Entity {scope:$scope}) WHERE any(t IN $terms WHERE size(t)>=2 AND (toLower(a.name) CONTAINS toLower(t) OR toLower(t) CONTAINS toLower(a.name))) WITH a ORDER BY size(a.name) DESC LIMIT 8 MATCH p=(a)-[:EXAM408_REL*0..2]-(b:Exam408Entity) WHERE all(n IN nodes(p) WHERE n.scope=$scope) AND all(r IN relationships(p) WHERE r.scope=$scope) RETURN [n IN nodes(p)|n.payload],[r IN relationships(p)|r.payload] ORDER BY length(p) DESC,a.name LIMIT 24`, map[string]any{"scope": kid, "terms": terms}})
	if e != nil {
		return nil, e
	}
	out := []domain.KnowledgePath{}
	for _, row := range rows {
		if len(row) != 2 {
			return nil, fmt.Errorf("invalid graph path response")
		}
		var nodes, edges []string
		if json.Unmarshal(row[0], &nodes) != nil || json.Unmarshal(row[1], &edges) != nil {
			return nil, graphUnavailable()
		}
		path := domain.KnowledgePath{}
		for _, raw := range nodes {
			var n domain.GraphNode
			if json.Unmarshal([]byte(raw), &n) != nil {
				return nil, graphUnavailable()
			}
			path.Nodes = append(path.Nodes, n)
		}
		for _, raw := range edges {
			var r domain.GraphEdge
			if json.Unmarshal([]byte(raw), &r) != nil {
				return nil, graphUnavailable()
			}
			path.Edges = append(path.Edges, r)
		}
		out = append(out, path)
	}
	return out, nil
}

func (n *Neo4jGraph) Version(ctx context.Context, kid string) (int64, error) {
	rows, e := n.query(ctx, cypherStatement{`MATCH (g:Exam408Namespace {scope:$scope}) RETURN g.version`, map[string]any{"scope": kid}})
	if e != nil {
		return 0, e
	}
	if len(rows) == 0 {
		return -1, nil
	}
	var version int64
	if len(rows[0]) != 1 || json.Unmarshal(rows[0][0], &version) != nil {
		return 0, graphUnavailable()
	}
	return version, nil
}
func (p *Postgres) CheckGraph(ctx context.Context) error {
	if p.Neo4j == nil {
		return graphUnavailable()
	}
	_, e := p.Neo4j.query(ctx, cypherStatement{Statement: "RETURN 1"})
	return e
}
