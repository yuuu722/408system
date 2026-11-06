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
	for _, s := range []string{"不足", "无法", "一次性", "驻留性", "限制", "浪费", "有限", "约束", "装不下", "容纳不了", "全部装入", "长期占用"} {
		problem = problem || strings.Contains(text, s)
	}
	for _, s := range []string{"局部性", "按需", "当前需要", "分次", "分多次", "通过", "从而", "允许", "只将", "仅将", "需要的部分", "不必全部", "无需全部", "分批"} {
		mechanism = mechanism || strings.Contains(text, s)
	}
	return
}
func adequateCoverage(q, answer string, contents []string) bool {
	if tcpReliabilityQuestion(q) && !tcpMechanismCoverage(answer) {
		return false
	}

	if spec := comparisonTargets(q); spec != nil {
		return mentions(answer, spec.Left) && mentions(answer, spec.Right) && !vagueComparison(answer)
	}
	if protocolWhy(q) {
		return protocolPurpose(answer)
	}
	if !virtualMemoryWhy(q) {
		return true
	}
	p, m := answerAspects(strings.Join(contents, "\n"))
	ap, am := answerAspects(answer)
	return (!p || ap) && (!m || am)
}

func balanceWhyEvidence(q string, chunks []domain.Chunk, minScore float64) []domain.Chunk {
	if !virtualMemoryWhy(q) || len(chunks) < 2 {
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

// Purpose coverage is an intent hint, not a source-support proof. Semantic
// review and exact quote/condition checks still run after this check.
func protocolWhy(q string) bool { return whyQuestion(q) && len(protocolFocus(q)) > 0 }
func protocolPurpose(text string) bool {
	for _, sentence := range completeSourceSentences(text) {
		purpose := false
		for _, word := range []string{"用于", "用来", "目的", "解决", "提供", "支持", "适用", "满足", "便于", "作用", "功能", "为此", "封装", "协商", "身份认证"} {
			purpose = purpose || strings.Contains(sentence, word)
		}
		// Format facts and MTU limits alone do not explain why the protocol is used.
		detail := false
		for _, word := range []string{"首部", "尾部", "字段", "帧格式", "帧的格式", "MTU", "最大传送单元"} {
			detail = detail || strings.Contains(sentence, word)
		}
		if purpose && !detail {
			return true
		}
	}
	return false
}
