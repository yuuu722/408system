package domain

import (
	"github.com/local/exam408-assistant/backend/internal/sourcequality"
	"strings"
)

func QuestionTextIssue(a Asset) string {
	if a.Kind == "question" && sourcequality.CorruptQuestionText(a.Payload.Stem+"\n"+strings.Join(a.Payload.Options, "\n")) {
		return "题干或选项存在明显OCR乱码，暂停练习，请核对原文修复"
	}
	return ""
}
func MarkQuestionTextQuality(a *Asset) {
	if issue := QuestionTextIssue(*a); issue != "" {
		a.Status = "unavailable"
		a.Payload.Issue = issue
	}
}
