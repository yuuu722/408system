package domain

import (
	"context"
	"time"
)

type GraphNode struct {
	Importance                                float64 `json:",omitempty"`
	ExamYears, CorpusYears                    int     `json:",omitempty"`
	ImportanceBasis                           string  `json:",omitempty"`
	ID, Name, EntityType, Description         string
	KnowledgePointID                          string
	SubjectCode                               string
	SubjectCodes                              []string `json:",omitempty"`
	SubjectState                              string   `json:",omitempty"`
	EvidenceChunkID, EvidenceExcerpt          string
	EvidenceDocumentID, EvidenceDocumentTitle string
	Source                                    string
	Confidence                                float64
	CreatedAt                                 time.Time
}

type GraphEdge struct {
	EvidenceCount                              int
	ID, FromEntityID, ToEntityID, RelationType string
	EvidenceChunkID, EvidenceExcerpt           string
	EvidenceDocumentID, EvidenceDocumentTitle  string
	Source                                     string
	Confidence                                 float64
	CreatedAt                                  time.Time
}

type KnowledgeGraph struct {
	Warning         string `json:",omitempty"`
	Storage         string
	KnowledgeBaseID string
	IsPublic        bool
	Nodes           []GraphNode
	Edges           []GraphEdge
}

type KnowledgeGraphStore interface {
	KnowledgeGraph(context.Context, string, string) (KnowledgeGraph, error)
}

// GraphRelationCandidate is an untrusted model proposal. The repository must
// independently validate endpoints and evidence before persisting it.
type GraphRelationCandidate struct {
	FromPointCode, ToPointCode, FromName, ToName, RelationType, Evidence string
	Confidence                                                           float64
}

type GraphEntityCandidate struct {
	Name, EntityType, Description, Evidence string
	Confidence                              float64
}

type GraphCandidateStore interface {
	AddValidatedGraphCandidates(context.Context, string, string, string, []GraphEntityCandidate, []GraphRelationCandidate) error
}

// FormalGraphNode reports whether the node belongs to the controlled directory.
func FormalGraphNode(node GraphNode) bool {
	return node.KnowledgePointID != "" && (node.Source == "canonical" || node.Source == "canonical_directory")
}

func FormalGraphEdge(edge GraphEdge) bool {
	return edge.Source == "canonical" || edge.Source == "canonical_directory" || edge.Source == "manual"
}

// GraphView returns the formal graph or the candidate review graph. The review
// graph includes formal endpoints only where they give a candidate edge context.
func DirectoryNames(points []KnowledgePoint) map[string]string {
	names := make(map[string]string, len(points))
	for _, point := range points {
		names[point.ID] = point.Name
	}
	return names
}

func DirectoryGraphNode(node GraphNode, names map[string]string) bool {
	name, known := names[node.KnowledgePointID]
	return known && FormalGraphNode(node) && node.Name == name
}

func GraphView(graph KnowledgeGraph, candidates bool, names map[string]string) KnowledgeGraph {
	nodes := make(map[string]bool, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodes[node.ID] = DirectoryGraphNode(node, names)
	}
	incident := map[string]bool{}
	edges := make([]GraphEdge, 0, len(graph.Edges))
	for _, edge := range graph.Edges {
		from, hasFrom := nodes[edge.FromEntityID]
		to, hasTo := nodes[edge.ToEntityID]
		formal := from && to && FormalGraphEdge(edge)
		if hasFrom && hasTo && formal != candidates {
			edges = append(edges, edge)
			incident[edge.FromEntityID], incident[edge.ToEntityID] = true, true
		}
	}
	filtered := make([]GraphNode, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		if nodes[node.ID] != candidates || incident[node.ID] {
			filtered = append(filtered, node)
		}
	}
	if !candidates {
		best := map[string]GraphNode{}
		for _, node := range filtered {
			current, ok := best[node.KnowledgePointID]
			if !ok || current.EvidenceDocumentID == "" && node.EvidenceDocumentID != "" {
				best[node.KnowledgePointID] = node
			}
		}
		ids := map[string]string{}
		unique := make([]GraphNode, 0, len(best))
		for _, node := range filtered {
			selected := best[node.KnowledgePointID]
			ids[node.ID] = selected.ID
			if node.ID == selected.ID {
				unique = append(unique, node)
			}
		}
		type edgeKey struct{ from, to, kind string }
		seen := map[edgeKey]int{}
		links := make([]GraphEdge, 0, len(edges))
		for _, edge := range edges {
			edge.FromEntityID, edge.ToEntityID = ids[edge.FromEntityID], ids[edge.ToEntityID]
			if edge.FromEntityID == edge.ToEntityID {
				continue
			}
			key := edgeKey{edge.FromEntityID, edge.ToEntityID, edge.RelationType}
			if index, ok := seen[key]; ok {
				if links[index].EvidenceDocumentID == "" && edge.EvidenceDocumentID != "" {
					links[index] = edge
				}
				continue
			}
			seen[key] = len(links)
			links = append(links, edge)
		}
		filtered, edges = unique, links
	}
	graph.Nodes, graph.Edges = filtered, edges
	return graph
}
