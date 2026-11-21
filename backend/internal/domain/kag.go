package domain

type KnowledgePath struct {
	Nodes []GraphNode
	Edges []GraphEdge
}
type RetrievalTrace struct {
	Mode         string
	Subquestions []string
	Paths        []KnowledgePath
	Notice       string
	WebStatus    string `json:",omitempty"`
}
type GraphRetrieval struct {
	Paths  []KnowledgePath
	Chunks []Chunk
	Terms  []string
}
