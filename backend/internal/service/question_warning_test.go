package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestWordPaginationNoticeDoesNotInvalidateQuestionsOrAnswers(t *testing.T) {
	notice := "Word转换页码：来自服务端排版，不保证与Word/WPS原始页码一致；请下载原文件核对。"
	for _, warnings := range [][]string{{notice}, {notice, "文字区域解析失败"}} {
		pages := []TextPage{{Number: 1, Text: "1. 下列哪项正确？\nA. 按需装入\nB. 全部装入\nC. 不用内存\nD. 不用外存", Warnings: warnings}, {Number: 2, Text: "参考答案\n1. A", Warnings: warnings}}
		assets := ExtractQuestions(domain.Document{ID: "doc", OriginalFilename: "教材.docx"}, pages, []domain.KnowledgePoint{{ID: "p", Name: "按需装入"}})
		require.Len(t, assets, 1)
		if len(warnings) == 1 {
			require.Empty(t, assets[0].Payload.Issue)
			require.Equal(t, "ready", assets[0].Status)
			require.Equal(t, "source", assets[0].Payload.AnswerOrigin)
			require.Equal(t, 0, assets[0].Payload.Answer)
		} else {
			require.Contains(t, assets[0].Payload.Issue, "解析异常")
			require.NotEqual(t, "ready", assets[0].Status)
			require.NotEqual(t, "source", assets[0].Payload.AnswerOrigin)
		}
	}
	require.True(t, hasQuestionExtractionWarning([]string{"此页使用本地OCR识别，请核对文字和公式。"}))
}
