package domain

import "testing"

func TestFormalGraphPrefersSourcedNodeAndPreservesCrossSubjectLink(t *testing.T) {
	graph := KnowledgeGraph{
		Nodes: []GraphNode{
			{ID: "a", Name: "快表", KnowledgePointID: "tlb", Source: "canonical_directory"},
			{ID: "a-source", Name: "快表", KnowledgePointID: "tlb", Source: "canonical", EvidenceDocumentID: "doc"},
			{ID: "b", Name: "地址转换", KnowledgePointID: "address", Source: "canonical_directory"},
			{ID: "noise", Name: "教材广告", Source: "rule_candidate"},
		},
		Edges: []GraphEdge{
			{FromEntityID: "a", ToEntityID: "b", RelationType: "related", Source: "canonical_directory"},
			{FromEntityID: "a-source", ToEntityID: "b", RelationType: "related", Source: "canonical_directory"},
		},
	}
	formal := GraphView(graph, false, map[string]string{"tlb": "快表", "address": "地址转换"})
	if len(formal.Nodes) != 2 || len(formal.Edges) != 1 || formal.Edges[0].FromEntityID != "a-source" {
		t.Fatalf("expected a single sourced node and its link: %+v", formal)
	}
}
