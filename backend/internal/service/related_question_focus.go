package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/sourcequality"
	"strings"
)

func focusedRelatedQuestions(question string, assets []domain.Asset) []domain.Asset {
	out := []domain.Asset{}
	seen := map[string]bool{}
	spec := comparisonTargets(question)
	for _, a := range assets {
		body := sourcequality.Normalize(a.Payload.Stem + "\n" + strings.Join(a.Payload.Options, "\n"))
		if a.Kind != "question" || a.Status != "ready" || sourcequality.CorruptQuestionText(body) || sourcequality.SuspectFormula(body) {
			continue
		}
		if spec != nil {
			if !mentions(body, spec.Left) || !mentions(body, spec.Right) {
				continue
			}
		} else if !evidenceRelated(question, body) {
			continue
		}
		family := domain.Family(a)
		if seen[family] {
			continue
		}
		seen[family] = true
		out = append(out, a)
		if len(out) == 3 {
			break
		}
	}
	return out
}
