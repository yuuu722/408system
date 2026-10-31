package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCatalogSuggestionsDistinguishSyllabusExercisesAndTextbooks(t *testing.T) {
	for _, c := range []struct{ name, kind, subject, purpose string }{
		{"2024年408考研大纲.pdf", "outline", "", "knowledge"},
		{"【横版】26王道操作系统选择题做题本.pdf", "exercise", "os", "questions"},
		{"26版竟成数据结构考研复习全书.pdf", "textbook", "ds", "knowledge"},
		{"计算机网络释疑与习题解答.pdf", "solution", "cn", "both"},
		{"2026操作系统.pdf", "textbook", "os", "knowledge"},
		{"408模拟试卷.pdf", "mock_exam", "", "questions"},
		{"没有分类信息.pdf", "other", "", "knowledge"},
		{"数据结构和操作系统对照.pdf", "other", "", "knowledge"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := SuggestDocumentCatalog(domain.Document{OriginalFilename: c.name, ResourceType: "other"})
			require.Equal(t, c.kind, got.ResourceType)
			require.Equal(t, c.subject, got.SubjectCode)
			require.Equal(t, c.purpose, got.Purpose)
			require.Equal(t, "pending", got.ReviewState)
		})
	}
}
