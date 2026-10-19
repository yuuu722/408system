package domain

import "context"

type Store interface {
	CreateUser(context.Context, User) (User, error)
	UserByLogin(context.Context, string) (User, error)
	UserByID(context.Context, string) (User, error)
	CreateKnowledgeBase(context.Context, string, string, string) (KnowledgeBase, error)
	ListKnowledgeBases(context.Context, string) ([]KnowledgeBase, error)
	KnowledgeBase(context.Context, string, string) (KnowledgeBase, error)
	DeleteKnowledgeBase(context.Context, string, string) error
	CreateDocument(context.Context, Document) (Document, error)
	ListDocuments(context.Context, string, string) ([]Document, error)
	Document(context.Context, string, string) (Document, error)
	UpdateDocumentStatus(context.Context, string, string, string) error
	UpdateDocumentProgress(context.Context, string, string, int, int, int) error
	DeleteDocument(context.Context, string, string) error
	ReplaceChunks(context.Context, string, string, []Chunk) error
	SearchChunks(context.Context, string, string, []float32, int) ([]Chunk, error)
	ListSubjects(context.Context) ([]Subject, error)
	ListKnowledgePoints(context.Context) ([]KnowledgePoint, error)
	KnowledgePoint(context.Context, string) (KnowledgePoint, error)
	PointRelations(context.Context, string) ([]PointRelation, error)
	MapDocumentToPoint(context.Context, string, string, string) error
	KnowledgePointOverview(context.Context, string, string) (KnowledgePointOverview, error)
	CreateConversation(context.Context, string, string, string) (string, error)
	SaveMessage(context.Context, string, string, string, string, []Citation) error
	CreateLearningEvent(context.Context, LearningEvent) (LearningEvent, error)
	ApplyMastery(context.Context, LearningEvent) error
	ListMastery(context.Context, string, bool) ([]Mastery, error)
	ReplaceRecommendations(context.Context, string, []Recommendation) error
	ListRecommendations(context.Context, string) ([]Recommendation, error)
	Dashboard(context.Context, string) (Dashboard, error)
}

type EmbeddingProvider interface {
	Embed(context.Context, []string) ([][]float32, error)
}
type LLMProvider interface {
	Answer(context.Context, string, string) (string, error)
}
type VectorStore interface {
	Search(context.Context, string, string, []float32, int) ([]Chunk, error)
}
type DocumentParser interface {
	Parse(context.Context, string, string) (string, error)
}
