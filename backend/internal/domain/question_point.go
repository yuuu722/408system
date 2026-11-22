package domain

import "strings"

func MatchQuestionPoint(text string, points []KnowledgePoint) string {
	best := 0
	bestStart, bestEnd := -1, -1
	id := ""
	ambiguous := false
	lower := strings.ToLower(text)
	for _, p := range points {
		score := 0
		start, end := -1, -1
		for _, term := range append(append([]string{}, p.Keywords...), p.Name) {
			n := len([]rune(term))
			normalized := strings.ToLower(term)
			offset := strings.Index(lower, normalized)
			if n < 2 || offset < 0 {
				continue
			}
			// A canonical name wins a same-length collision with an alias.
			weight := n * 2
			if term == p.Name {
				weight++
			}
			if weight > score {
				score, start, end = weight, offset, offset+len(normalized)
			}
		}
		// For two equally specific overlapping names, prefer the one
		// beginning the phrase (e.g. 虚拟内存 within 虚拟内存管理).
		overlap := start >= 0 && bestStart >= 0 && start < bestEnd && bestStart < end
		if score > best || score == best && overlap && start < bestStart {
			best = score
			bestStart, bestEnd = start, end
			id = p.ID
			ambiguous = false
		} else if score > 0 && score == best && p.ID != id && (!overlap || start == bestStart) {
			ambiguous = true
		}
	}
	if ambiguous {
		return ""
	}
	return id
}

// QuestionMatchesPoint keeps curated assignments and requires an extracted
// assignment to be recoverable from the stem, never from distractors alone.
func QuestionMatchesPoint(a Asset, points []KnowledgePoint) bool {
	if a.Kind != "question" || a.PointID == "" || QuestionTextIssue(a) != "" {
		return false
	}
	if a.Payload.QuestionOrigin != "extracted" {
		return true
	}
	match := MatchQuestionPoint(a.Payload.Stem, points)
	if match == "" && a.Payload.AnswerOrigin == "source" && HasQuestionExplanation(a.Payload.Explanation) {
		match = MatchQuestionPoint(a.Payload.Explanation, points)
	}
	return match == a.PointID
}
