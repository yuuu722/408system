package domain

import (
	"context"
	"time"
)

type GraphNode struct {
	ID, Name, EntityType, Description         string
	KnowledgePointID                          string
	EvidenceChunkID, EvidenceExcerpt          string
	EvidenceDocumentID, EvidenceDocumentTitle string
	Source                                    string
	Confidence                                float64
	CreatedAt                                 time.Time
}

type GraphEdge struct {
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
