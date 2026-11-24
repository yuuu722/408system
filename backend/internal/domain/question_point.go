package domain

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type questionPointTerm struct {
	text   string
	weight int
}
type questionPointTerms struct {
	id    string
	terms []questionPointTerm
}
type QuestionPointMatcher []questionPointTerms

func NewQuestionPointMatcher(points []KnowledgePoint) QuestionPointMatcher {
	matcher := make(QuestionPointMatcher, 0, len(points))
	for _, p := range points {
		item := questionPointTerms{id: p.ID}
		for _, term := range append(append([]string{}, p.Keywords...), p.Name) {
			n := utf8.RuneCountInString(term)
			if n < 2 {
				continue
			}
			weight := n * 2
			if term == p.Name {
				weight++
			}
			item.terms = append(item.terms, questionPointTerm{text: questionPointText(term), weight: weight})
		}
		matcher = append(matcher, item)
	}
	return matcher
}

func MatchQuestionPoint(text string, points []KnowledgePoint) string {
	return NewQuestionPointMatcher(points).Match(text)
}

func (matcher QuestionPointMatcher) Match(text string) string {
	best := 0
	bestStart, bestEnd := -1, -1
	id := ""
	ambiguous := false
	lower := questionPointText(text)
	for _, p := range matcher {
		score := 0
		start, end := -1, -1
		for _, term := range p.terms {
			if term.weight <= score {
				continue
			}
			if offset := questionTermIndex(lower, term.text); offset >= 0 {
				score, start, end = term.weight, offset, offset+len(term.text)
			}
		}
		// For two equally specific overlapping names, prefer the one
		// beginning the phrase (e.g. 虚拟内存 within 虚拟内存管理).
		overlap := start >= 0 && bestStart >= 0 && start < bestEnd && bestStart < end
		if score > best || score == best && overlap && start < bestStart {
			best = score
			bestStart, bestEnd = start, end
			id = p.id
			ambiguous = false
		} else if score > 0 && score == best && p.id != id && (!overlap || start == bestStart) {
			ambiguous = true
		}
	}
	if ambiguous {
		return ""
	}
	return id
}

// Ignore horizontal PDF layout spaces between adjacent Chinese characters.
func questionPointText(text string) string {
	runes := []rune(strings.ToLower(text))
	var out strings.Builder
	for i := 0; i < len(runes); i++ {
		if i > 0 && unicode.Is(unicode.Han, runes[i-1]) && (runes[i] == ' ' || runes[i] == '\t' || runes[i] == '\u3000') {
			j := i + 1
			for j < len(runes) && (runes[j] == ' ' || runes[j] == '\t' || runes[j] == '\u3000') {
				j++
			}
			if j < len(runes) && unicode.Is(unicode.Han, runes[j]) {
				i = j
			}
		}
		out.WriteRune(runes[i])
	}
	return out.String()
}

func questionTermIndex(text, term string) int {
	if term == "" {
		return -1
	}
	word := func(b byte) bool { return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '_' }
	for from := 0; from < len(text); {
		offset := strings.Index(text[from:], term)
		if offset < 0 {
			break
		}
		start := from + offset
		end := start + len(term)
		if (start == 0 || !word(term[0]) || !word(text[start-1])) &&
			(end == len(text) || !word(term[len(term)-1]) || !word(text[end])) {
			return start
		}
		from = start + 1
	}
	return -1
}

// QuestionMatchesPoint keeps curated assignments and requires an extracted
// assignment to be recoverable from the stem, never from distractors alone.
func (matcher QuestionPointMatcher) MatchesAsset(a Asset) bool {
	if a.Kind != "question" || a.PointID == "" || QuestionTextIssue(a) != "" {
		return false
	}
	if a.Payload.QuestionOrigin != "extracted" {
		return true
	}
	match := matcher.Match(a.Payload.Stem)
	if match == "" && a.Payload.AnswerOrigin == "source" && HasQuestionExplanation(a.Payload.Explanation) {
		match = matcher.Match(a.Payload.Explanation)
	}
	return match == a.PointID
}
