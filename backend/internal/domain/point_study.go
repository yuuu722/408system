package domain

import "time"

type PointMistake struct {
	AssessmentID string
	Question     Asset
	Result       QuestionResult
	SubmittedAt  time.Time
}
type PointStudy struct {
	PreferredKnowledgeBaseID          string
	PointNames                        map[string]string
	Point                             KnowledgePoint
	Ancestors, Children               []KnowledgePoint
	Knowledge                         []Citation
	Example                           *Asset
	Exercises                         []Asset
	Mistakes                          []PointMistake
	DirectQuestions, SubtreeQuestions int
	Missing                           []string
}

// Directory ancestry is navigation, not an asserted prerequisite or causal edge.
func PointScope(points []KnowledgePoint, root string) map[string]bool {
	scope := map[string]bool{root: true}
	for changed := true; changed; {
		changed = false
		for _, p := range points {
			if !scope[p.ID] && scope[p.ParentID] {
				scope[p.ID] = true
				changed = true
			}
		}
	}
	return scope
}
func SubtreeQuestionCounts(points []KnowledgePoint, direct map[string]int) map[string]int {
	parents := map[string]string{}
	out := map[string]int{}
	for _, p := range points {
		parents[p.ID] = p.ParentID
		out[p.ID] = direct[p.ID]
	}
	for id, n := range direct {
		seen := map[string]bool{id: true}
		for parent := parents[id]; parent != "" && !seen[parent]; parent = parents[parent] {
			out[parent] += n
			seen[parent] = true
		}
	}
	return out
}
