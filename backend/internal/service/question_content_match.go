package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"strings"
	"unicode"
)

func questionIdentityText(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, text)
}

// Different file names and question numbers may describe the very same problem.
// Content matching requires the entire stem and every option, an explicit
// single source answer, and a substantive labelled explanation. It never uses
// subject similarity or a repeated number alone to connect unrelated files.
func MatchQuestionContent(assets []domain.Asset, entries []FileAnswer) []AnswerFileProposal {
	out := []AnswerFileProposal{}
	for _, a := range assets {
		if a.Kind != "question" || a.Payload.ReviewStatus == "published" || len(a.Payload.Options) < 2 {
			continue
		}
		stem := questionIdentityText(a.Payload.Stem)
		if len([]rune(stem)) < 12 {
			continue
		}
		matches := []FileAnswer{}
		for _, entry := range entries {
			if entry.Blocked || entry.Answer < 0 || entry.Answer >= len(a.Payload.Options) {
				continue
			}
			quote := AnswerFileText(entry)
			// Question text itself must not count as the explanation.
			explanation := ""
			for _, marker := range []string{"【解析】", "解析：", "解析:", "解答：", "理由："} {
				if i := strings.Index(quote, marker); i >= 0 {
					explanation = quote[i+len(marker):]
					break
				}
			}
			if !domain.HasQuestionExplanation(explanation) {
				continue
			}
			normalized := questionIdentityText(quote)
			start := strings.Index(normalized, stem)
			if start < 0 {
				continue
			}
			after := normalized[start+len(stem):]
			valid := true
			for _, option := range a.Payload.Options {
				option = questionIdentityText(option)
				if option == "" {
					valid = false
					break
				}
				i := strings.Index(after, option)
				if i < 0 {
					valid = false
					break
				}
				after = after[i+len(option):]
			}
			if valid {
				matches = append(matches, entry)
			}
		}
		if len(matches) == 1 {
			out = append(out, AnswerFileProposal{Asset: a, Answer: matches[0]})
		}
	}
	return out
}

// Fill an unassigned point only when the strongest literal terminology match
// is unique. This is a rule-based association, never a claimed semantic proof.
func MatchQuestionPoint(text string, points []domain.KnowledgePoint) string {
	best := 0
	id := ""
	ambiguous := false
	for _, p := range points {
		score := 0
		for _, term := range append(append([]string{}, p.Keywords...), p.Name) {
			n := len([]rune(term))
			if n < 2 || !strings.Contains(strings.ToLower(text), strings.ToLower(term)) {
				continue
			}
			if n > score {
				score = n
			}
		}
		if score > best {
			best = score
			id = p.ID
			ambiguous = false
		} else if score > 0 && score == best && p.ID != id {
			ambiguous = true
		}
	}
	if ambiguous {
		return ""
	}
	return id
}

func hasWorkedAnswerLabel(text string) bool {
	for _, marker := range []string{"【解析】", "解析：", "解析:", "解答：", "理由："} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
