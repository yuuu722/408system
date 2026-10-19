package domain

import "time"

type User struct {
	ID, Username, Email, PasswordHash string
	Role                              string
	Disabled                          bool
	CreatedAt                         time.Time
}
type KnowledgeBase struct {
	ID, UserID, Name, Description string
	IsPublic                      bool
	CreatedAt, UpdatedAt          time.Time
}
type Document struct {
	ActiveRevisionID                                                                                                   string
	Execution                                                                                                          int64 `json:"-"`
	Progress, ProcessedChunks, TotalChunks                                                                             int
	ProcessingStage                                                                                                    string
	ID, KnowledgeBaseID, UserID, Filename, OriginalFilename, MimeType, StoragePath, ResourceType, Status, ErrorMessage string
	Size                                                                                                               int64
	CreatedAt, UpdatedAt                                                                                               time.Time
}
type Chunk struct {
	ID, DocumentID, KnowledgeBaseID, UserID, Content, SourceLocation string
	ChunkIndex                                                       int
	Metadata                                                         map[string]any
	Embedding                                                        []float32
	DocumentTitle                                                    string
	Score                                                            float64
	CreatedAt                                                        time.Time
}
type Subject struct {
	ID, Code, Name string
	SortOrder      int
}
type KnowledgePoint struct {
	ID, SubjectID, ParentID, Code, Name, Description, SubjectName string
	Keywords                                                      []string
	Level, SortOrder                                              int
	Children                                                      []KnowledgePoint `json:"children,omitempty"`
}
type PointRelation struct{ ID, FromPointID, ToPointID, RelationType string }
type LearningEvent struct {
	ID, UserID, SubjectID, KnowledgePointID, ResourceID, EventType string
	Metadata                                                       map[string]any
	CreatedAt                                                      time.Time
}
type Mastery struct {
	UserID, KnowledgePointID, KnowledgePointName, SubjectName string
	MasteryScore, WeaknessScore, Confidence                   float64
	InteractionCount                                          int
	LastInteractionAt                                         *time.Time
	UpdatedAt                                                 time.Time
}
type Recommendation struct {
	ID, UserID, KnowledgePointID, KnowledgePointName, SubjectName, Reason, RecommendedAction, Status string
	Score                                                                                            float64
	GeneratedAt, ExpiresAt                                                                           time.Time
}
type Citation struct {
	RevisionID                                            string
	Regions                                               []SourceRegion
	LocationQuality                                       string
	Page                                                  int
	DocumentID, DocumentTitle, ChunkID, Location, Excerpt string
	Score                                                 float64
}
type ChatResult struct {
	Answer, ConversationID string
	Sources                []Citation
	KnowledgePoints        []KnowledgePoint
}
type Dashboard struct {
	Subjects                               []SubjectProgress
	Weaknesses                             []Mastery
	Recommendations                        []Recommendation
	RecentDocuments                        []Document
	RecentEvents                           []LearningEvent
	KnowledgeBaseCount, ReadyDocumentCount int
}
type SubjectProgress struct {
	ID, Code, Name                    string
	Progress                          float64
	KnowledgePointCount, TouchedCount int
}

type KnowledgePointOverview struct {
	Resources    []Document
	Mastery      *Mastery
	RecentEvents []LearningEvent
}

var AllowedEvents = map[string]bool{"resource_view": true, "resource_complete": true, "rag_question": true, "knowledge_point_view": true, "quiz_correct": true, "quiz_wrong": true, "recommendation_view": true, "recommendation_click": true}

// UserRecordableEvents contains passive UI interactions that may safely be
// accepted from an authenticated browser. Outcome-bearing events such as quiz
// results and resource completion must be produced by trusted server workflows.
var UserRecordableEvents = map[string]bool{
	"resource_view":        true,
	"knowledge_point_view": true,
	"recommendation_view":  true,
	"recommendation_click": true,
}
