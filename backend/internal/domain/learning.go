package domain

import "time"

type AssetPayload struct {
	ReviewStatus                               string
	ParentAssetID                              string
	GeneratorModel, GeneratedAt                string
	ContentHash                                string
	DifficultyOrigin                           string
	EstimatedMinutes                           int
	Evidence                                   []Citation
	RevisionID, FamilyID, QuestionOrigin       string
	AnswerOrigin, Issue, ExtractionKey         string
	Images                                     []string
	Stem                                       string
	Options                                    []string
	Answer                                     int
	Explanation, Source, URL, DocumentID       string
	Page, StartSeconds, EndSeconds, Difficulty int
}
type Asset struct {
	IsPublic                         bool
	ID, Kind, PointID, Title, Status string
	Revision                         int
	Payload                          AssetPayload
}
type Profile struct {
	GoalScore, DailyMinutes     int
	ExamDate, Stage, Preference string
}
type Assessment struct {
	ID, Kind  string
	Questions []Asset
	Result    *AssessmentResult
	CreatedAt time.Time
}
type QuestionResult struct {
	FamilyID, RevisionID       string
	AnswerOrigin, Confirmation string
	Counted                    bool
	ID, PointID                string
	Selected, Answer           int
	Correct                    bool
	Explanation                string
}
type AssessmentResult struct {
	Counted        int
	Total, Correct int
	Items          []QuestionResult
}
type RadarSubject struct {
	ID, Name                                    string
	Tested, Correct, TotalPoints, CoveredPoints int
	Accuracy                                    *float64
}
type StudyTask struct {
	PlanDate, DurationSource                                   string
	EstimatedMinutes                                           int
	ID, AssetID, Title, Kind, PointID, Reason, URL, DocumentID string
	Page, StartSeconds, EndSeconds                             int
	Completed                                                  bool
}
