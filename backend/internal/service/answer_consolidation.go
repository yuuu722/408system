package service

import "github.com/local/exam408-assistant/backend/internal/domain"

// A table of grade letters and a separately numbered worked explanation may
// share the same chapter+number. Merge exactly one compact grade with exactly
// one labelled explanation; duplicates, conflicts and uncertain pages remain
// ambiguous. Keep both original excerpts in the resulting evidence.
func consolidateAnswerEntries(entries []FileAnswer) []FileAnswer {
	groups := map[string][]int{}
	for i, a := range entries {
		groups[a.Section+"#"+a.Number] = append(groups[a.Section+"#"+a.Number], i)
	}
	replacements := map[int]FileAnswer{}
	removed := map[int]bool{}
	for _, indices := range groups {
		if len(indices) != 2 {
			continue
		}
		first, second := entries[indices[0]], entries[indices[1]]
		if first.Blocked || second.Blocked {
			continue
		}
		grade, worked := first, second
		if domain.HasQuestionExplanation(AnswerFileText(first)) {
			grade, worked = second, first
		}
		if grade.Answer < 0 || domain.HasQuestionExplanation(AnswerFileText(grade)) || !domain.HasQuestionExplanation(AnswerFileText(worked)) || !hasWorkedAnswerLabel(AnswerFileText(worked)) {
			continue
		}
		if worked.Answer >= 0 && worked.Answer != grade.Answer {
			continue
		}
		worked.Answer = grade.Answer
		worked.Extra = append(worked.Extra, AnswerPassage{Page: grade.Page, Quote: grade.Quote})
		worked.Extra = append(worked.Extra, grade.Extra...)
		replacements[indices[0]] = worked
		removed[indices[1]] = true
	}
	out := []FileAnswer{}
	for i, a := range entries {
		if removed[i] {
			continue
		}
		if merged, ok := replacements[i]; ok {
			a = merged
		}
		out = append(out, a)
	}
	return out
}
