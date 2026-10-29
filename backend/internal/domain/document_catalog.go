package domain

// A document is stored once; its use in retrieval and question extraction is explicit.
type DocumentCatalog struct {
	DocumentID, ResourceType, SubjectCode, Purpose, ReviewState, Reason string
}
