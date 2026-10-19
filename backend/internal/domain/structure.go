package domain

type DocumentBlock struct {
	ID, Kind, Text, Image string
	Start, End            int
	Box                   []float64
	Confidence            *float64
}
type StructuredPage struct {
	Segment           int
	CropBox, MediaBox []float64
	Width, Height     float64
	Rotation          int
	CoordinateSpace   string
	Number            int
	Text              string
	Blocks            []DocumentBlock
	Warnings          []string
	Image             string
}
type ParsedDocument struct {
	State      string
	RevisionID string
	Pages      []StructuredPage
	Engine     string
	Warnings   []string
}

// Offsets are Unicode code points in canonical page text; boxes use top-left normalized page coordinates.
type SourceRegion struct {
	Segment    int
	Page       int
	BlockID    string
	Start, End int
	Box        []float64
}

type RevisionSpec struct{ Fingerprint, FileHash, ParseKey, ChunkKey, EmbeddingSpace string }
type Revision struct {
	ID                  string
	ParsedDone, Chunked bool
	TotalPages          int
}
