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
	require.False(t, hasQuestionExtractionWarning([]string{"此页使用本地OCR识别，请核对文字和公式。"}))
}

func TestChapterAnswersDoNotStopFollowingQuestionExtraction(t *testing.T) {
	pages := []TextPage{{Number: 1, Text: "第一章 内存\n1. 下列内存描述哪个正确？\nA. 甲 B. 乙\n参考答案\n1. A\n第二章 文件\n1. 下列文件描述哪个正确？\nA. 丙 B. 丁\n参考答案\n1. B"}}
	assets := ExtractQuestions(domain.Document{ID: "d", OriginalFilename: "教材"}, pages, nil)
	require.Len(t, assets, 2)
	require.Equal(t, 0, assets[0].Payload.Answer)
	require.Equal(t, 1, assets[1].Payload.Answer)
}
func TestGuideQuestionsAreNotInventedAsSingleChoiceAssets(t *testing.T) {
	assets := ExtractQuestions(domain.Document{ID: "d"}, []TextPage{{Number: 1, Text: "3.2 虚拟内存管理\n1. 为什么需要虚拟内存？\n2. 虚拟内存是什么？"}}, nil)
	require.Empty(t, assets)
}
func TestOCRNoticeIsInformationalButOCRFailureRemainsBlocking(t *testing.T) {
	require.False(t, hasQuestionExtractionWarning([]string{"此页使用本地OCR识别，请核对文字和公式。"}))
	require.True(t, hasQuestionExtractionWarning([]string{"此页使用本地OCR识别，请核对文字和公式。", "OCR failed"}))
}
