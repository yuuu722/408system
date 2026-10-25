package service

import (
	"strings"
	"unicode"
)

// A conservative lexical guard in addition to vector similarity. It deliberately
// prefers abstention for synonyms with no textual overlap; this is not a semantic reranker.
func evidenceRelated(question, evidence string) bool {
	return lexicalEvidenceScore(question, evidence) >= .5
}

func lexicalEvidenceScore(question, evidence string) float64 {
	q := strings.ToLower(question)
	for _, stop := range []string{"请根据知识库", "根据知识库", "请根据教材", "根据教材", "请根据资料", "根据资料", "说明一下", "请说明", "说明", "检索概念", "为什么", "是什么", "如何", "怎么", "什么", "请问", "请解释", "解释一下", "需要", "介绍一下", "请", "的", "吗", "呢"} {
		q = strings.ReplaceAll(q, stop, " ")
	}
	parts := strings.FieldsFunc(q, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	e := strings.ToLower(evidence)
	tokens := map[string]bool{}
	for _, part := range parts {
		runes := []rune(part)
		if len(runes) < 2 {
			continue
		}
		if !strings.ContainsFunc(part, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
			tokens[part] = true
			continue
		}
		for i := 0; i+1 < len(runes); i++ {
			tokens[string(runes[i:i+2])] = true
		}
	}
	matched := 0
	for token := range tokens {
		if strings.Contains(e, token) {
			matched++
		}
	}
	if len(tokens) == 0 {
		return 0
	}
	return float64(matched) / float64(len(tokens))
}
