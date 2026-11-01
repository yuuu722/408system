package repository

import (
	"context"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"sort"
)

// Adds only ancestors/curated relations for points already represented in this
// corpus. No corpus-independent sample graph or inferred causal edge is created.
func (p *Postgres) attachDirectory(ctx context.Context, g *domain.KnowledgeGraph) error {
	points, e := p.ListKnowledgePoints(ctx)
	if e != nil {
		return e
	}
	byID := map[string]domain.KnowledgePoint{}
	for _, pt := range points {
		byID[pt.ID] = pt
	}
	represented := map[string]string{}
	for _, n := range g.Nodes {
		if n.KnowledgePointID != "" {
			if _, ok := represented[n.KnowledgePointID]; !ok {
				represented[n.KnowledgePointID] = n.ID
			}
		}
	}
	seeds := []string{}
	for id := range represented {
		seeds = append(seeds, id)
	}
	sort.Strings(seeds)
	visiting := map[string]bool{}
	var ensure func(string) string
	ensure = func(id string) string {
		if n := represented[id]; n != "" {
			return n
		}
		pt, ok := byID[id]
		if !ok || visiting[id] {
			return ""
		}
		visiting[id] = true
		n := "directory:" + pt.ID
		represented[id] = n
		g.Nodes = append(g.Nodes, domain.GraphNode{ID: n, Name: pt.Name, Description: pt.Description, KnowledgePointID: pt.ID, Source: "canonical_directory", Confidence: 1, EntityType: "knowledge_point", SubjectCode: subjectCode(pt.SubjectName)})
		return n
	}
	edgeKeys := map[string]bool{}
	for _, r := range g.Edges {
		edgeKeys[r.FromEntityID+":"+r.ToEntityID+":"+r.RelationType] = true
	}
	add := func(from, to, kind string) {
		key := from + ":" + to + ":" + kind
		if from == "" || to == "" || from == to || edgeKeys[key] {
			return
		}
		edgeKeys[key] = true
		g.Edges = append(g.Edges, domain.GraphEdge{ID: "directory:" + key, FromEntityID: from, ToEntityID: to, RelationType: kind, Source: "canonical_directory", Confidence: 1, EvidenceCount: 1, EvidenceExcerpt: "来自管理员维护的知识点目录与关系；不是上传文档中的推导证据。"})
	}
	for _, id := range seeds {
		seen := map[string]bool{}
		for id != "" && !seen[id] {
			seen[id] = true
			pt, ok := byID[id]
			if !ok || pt.ParentID == "" {
				break
			}
			child := ensure(id)
			parent := ensure(pt.ParentID)
			add(child, parent, "part_of")
			id = pt.ParentID
		}
	}
	rows, e := p.DB.Query(ctx, `SELECT from_point_id::text,to_point_id::text,relation_type FROM exam408_knowledge_point_relations ORDER BY from_point_id,to_point_id`)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var a, b, k string
		if e = rows.Scan(&a, &b, &k); e != nil {
			return e
		}
		if represented[a] != "" && represented[b] != "" {
			add(represented[a], represented[b], k)
		}
	}
	return rows.Err()
}
func subjectCode(name string) string {
	switch name {
	case "数据结构":
		return "ds"
	case "计算机组成原理":
		return "co"
	case "操作系统":
		return "os"
	case "计算机网络":
		return "cn"
	}
	return ""
}
