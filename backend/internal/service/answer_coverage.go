package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"strings"
)

// Transparent coverage hints, not a semantic correctness proof. When the
// retrieved text contains both aspects, do not return only the problem statement.
func whyQuestion(q string) bool {
	return strings.Contains(q, "为什么") || strings.Contains(q, "原因") || strings.Contains(q, "必要")
}
func answerAspects(text string) (problem, mechanism bool) {
	for _, s := range []string{"不足", "无法", "一次性", "驻留性", "限制", "浪费"} {
		problem = problem || strings.Contains(text, s)
	}
	for _, s := range []string{"局部性", "按需", "当前需要", "分次", "分多次", "通过", "从而", "允许"} {
		mechanism = mechanism || strings.Contains(text, s)
	}
	return
}
func adequateCoverage(q, answer string, contents []string) bool {
	if tcpReliabilityQuestion(q) && !tcpMechanismCoverage(answer) {
		return false
	}
	if !whyQuestion(q) {
		return true
	}
	p, m := answerAspects(strings.Join(contents, "\n"))
	ap, am := answerAspects(answer)
	return (!p || ap) && (!m || am)
}

func balanceWhyEvidence(q string, chunks []domain.Chunk, minScore float64) []domain.Chunk {
	if !whyQuestion(q) || len(chunks) < 2 {
		return chunks
	}
	p, m := answerAspects(chunks[0].Content)
	if p && m {
		return chunks
	}
	for i := 1; i < len(chunks); i++ {
		if chunks[i].Score < minScore || !evidenceRelated(q, chunkIndexText(chunks[i])) {
			continue
		}
		cp, cm := answerAspects(chunks[i].Content)
		if (!p && cp) || (!m && cm) {
			c := chunks[i]
			copy(chunks[2:i+1], chunks[1:i])
			chunks[1] = c
			break
		}
	}
	return chunks
}
