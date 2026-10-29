package service

import (
	"regexp"
	"strings"
)

var choiceOption = regexp.MustCompile(`(?m)(?:^|[\s。；;])(?:[A-DＡ-Ｄ])[.．、:：\s]`)
var questionBoundary = regexp.MustCompile(`(?m)^\s*(?:\([0-9]+\)|（[0-9]+）|[0-9]+[.．、])`)

// Conservative source filtering, not semantic entailment. Unresolved choices are
// questions, never asserted facts. Keep actual explanation sections for concept
// questions; exercise-solving requests may still retrieve the complete exercise.
func answerEvidence(question, content string) string {
	if questionIntent(question) != "概念与原理" && questionIntent(question) != "比较分析" {
		return strings.TrimSpace(content)
	}
	if choiceOption.MatchString(content) {
		explanation := -1
		for _, marker := range []string{"【解析】", "【解答】", "答案解析", "解析：", "解析:"} {
			if i := strings.Index(content, marker); i >= 0 && (explanation < 0 || i < explanation) {
				explanation = i
			}
		}
		if explanation < 0 {
			return ""
		}
		content = content[explanation:]
		// Do not accidentally retain the next unresolved exercise in the same chunk.
		if loc := questionBoundary.FindStringIndex(content); loc != nil {
			content = content[:loc[0]]
		}
		if choiceOption.MatchString(content) {
			return ""
		}
	}
	// A heading/outline with relevant words is not an explanation. Require a
	// related prose sentence with an explanatory predicate, even for outline PDFs.
	for _, sentence := range strings.FieldsFunc(content, func(r rune) bool { return strings.ContainsRune("。！？；\n", r) }) {
		if !evidenceRelated(question, sentence) {
			continue
		}
		for _, predicate := range []string{"是", "指", "用于", "用来", "使用", "利用", "通过", "由于", "因为", "因此", "使得", "能够", "可以", "提供", "避免", "解决", "实现", "允许", "需要", "保证", "基于", "负责", "不能", "并非", "称为"} {
			if strings.Contains(sentence, predicate) {
				return strings.TrimSpace(content)
			}
		}
	}
	return ""
}
