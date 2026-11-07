package domain

import (
	"sort"
	"time"
)

type LearnerMemory struct {
	Key, PointID, Kind, Content string
	Resolved                    bool
	UpdatedAt                   time.Time
}
type FrequencySource struct {
	ID, PointID, Organization, Title, URL, Excerpt string
	StartYear, EndYear, Exams, Appearances         int
}
type PointInsight struct {
	ID, Name, Code, SubjectID, SubjectName, ParentID, Description, State, ImportanceBasis string
	Tested, Correct, AvailableQuestions, ExamYears, CorpusYears, Doubts                   int
	Accuracy                                                                              *float64
	Importance, Priority                                                                  float64
	Sources                                                                               []FrequencySource
}
type LearningOverview struct {
	Points       []PointInsight
	Memories     []LearnerMemory
	NeedsPretest bool
	Notice       string
}
type CoachSession struct {
	Feedback                              *ChatResult `json:",omitempty"`
	ID, PointID, Teacher, KnowledgeBaseID string
	CreatedAt                             time.Time
	Assessment                            Assessment
	Lesson                                ChatResult
}

// One family and one primary knowledge point per round, then rotate subjects.
// Coverage is limited by available validated questions, never by invented items.
func DiagnosticSelection(assets []Asset, pointSubjects map[string]string, priorities map[string]float64, limit int) []Asset {
	sort.SliceStable(assets, func(i, j int) bool {
		if priorities[assets[i].PointID] != priorities[assets[j].PointID] {
			return priorities[assets[i].PointID] > priorities[assets[j].PointID]
		}
		return assets[i].ID < assets[j].ID
	})
	groups := map[string][]Asset{}
	seen := map[string]bool{}
	for _, a := range assets {
		if a.Kind != "question" || a.Status != "ready" || a.Payload.AnswerOrigin != "source" || a.PointID == "" || seen[Family(a)] {
			continue
		}
		seen[Family(a)] = true
		groups[pointSubjects[a.PointID]] = append(groups[pointSubjects[a.PointID]], a)
	}
	subjects := []string{}
	for k := range groups {
		subjects = append(subjects, k)
	}
	sort.Strings(subjects)
	out := []Asset{}
	used := map[string]int{}
	for len(out) < limit {
		progress := false
		for _, subject := range subjects {
			g := groups[subject]
			least := int(^uint(0) >> 1)
			for _, a := range g {
				if used[a.PointID] < least {
					least = used[a.PointID]
				}
			}
			for i, a := range g {
				if used[a.PointID] > least {
					continue
				}
				out = append(out, a)
				used[a.PointID]++
				groups[subject] = append(g[:i], g[i+1:]...)
				progress = true
				break
			}
			if len(out) == limit {
				break
			}
		}
		if !progress {
			break
		}
	}
	return out
}
