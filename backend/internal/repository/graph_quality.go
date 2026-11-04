package repository

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/sourcequality"
)

// Read-time quarantine also protects users while an old Neo4j snapshot awaits
// replay. Source records remain available for audit and future reprocessing.
func usableGraphNode(n domain.GraphNode) bool {
	if n.KnowledgePointID != "" || n.Source == "canonical_directory" || n.Source == "canonical" {
		return true
	}
	if n.Source == "rule_candidate" {
		return sourcequality.TechnicalNoun(n.Name) && sourcequality.Definition(n.Name, n.Description)
	}
	return sourcequality.TechnicalNoun(n.Name)
}
func qualityGraph(g domain.KnowledgeGraph) domain.KnowledgeGraph {
	keep := map[string]bool{}
	nodes := []domain.GraphNode{}
	edges := []domain.GraphEdge{}
	for _, n := range g.Nodes {
		if usableGraphNode(n) {
			keep[n.ID] = true
			nodes = append(nodes, n)
		}
	}
	for _, e := range g.Edges {
		if keep[e.FromEntityID] && keep[e.ToEntityID] {
			edges = append(edges, e)
		}
	}
	g.Nodes = nodes
	g.Edges = edges
	return g
}
