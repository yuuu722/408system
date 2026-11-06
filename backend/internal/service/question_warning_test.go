package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"strings"
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

func TestSplitNumberAndBareOptionLinesBecomeReviewableQuestions(t *testing.T) {
	pages := []TextPage{{Number: 1, Text: "（１）\n虚拟内存的特点是什么？\nＡ\n按需装入\nＢ\n全部常驻\nＣ\n不使用外存\nＤ\n不支持调入\n答案：A\n解析：虚拟内存允许按需装入。"}}
	assets := ExtractQuestions(domain.Document{ID: "d", OriginalFilename: "习题.pdf"}, pages, []domain.KnowledgePoint{{ID: "p", Name: "虚拟内存"}})
	require.Len(t, assets, 1)
	require.Len(t, assets[0].Payload.Options, 4)
	require.Equal(t, "按需装入", assets[0].Payload.Options[0])
	require.Equal(t, 0, assets[0].Payload.Answer)
	require.Empty(t, assets[0].Payload.Issue)
}
func TestLowConfidenceOtherQuestionDoesNotPoisonWholePage(t *testing.T) {
	confidence := .4
	page := TextPage{Number: 1, Text: "1. 虚拟内存是什么？\nA. 按需装入 B. 全部装入\n2. 虚拟内存是什么？\nA. 损坏文字 B. 按需装入", Warnings: []string{"第1页第4行OCR置信度较低，请核对原页数字与公式。"}}
	start := strings.Index(page.Text, "A. 损坏")
	page.Blocks = []domain.DocumentBlock{{ID: "bad", Text: "A. 损坏文字 B. 按需装入", Start: len([]rune(page.Text[:start])), End: len([]rune(page.Text)), Confidence: &confidence}}
	assets := ExtractQuestions(domain.Document{ID: "d"}, []TextPage{page}, []domain.KnowledgePoint{{ID: "p", Name: "虚拟内存"}})
	require.Len(t, assets, 2)
	require.Empty(t, assets[0].Payload.Issue)
	require.Contains(t, assets[1].Payload.Issue, "解析异常")
	page.Warnings = append(page.Warnings, "文字区域解析失败")
	assets = ExtractQuestions(domain.Document{ID: "d"}, []TextPage{page}, []domain.KnowledgePoint{{ID: "p", Name: "虚拟内存"}})
	require.Contains(t, assets[0].Payload.Issue, "解析异常")
}

func TestMultipleAnswersAreNeverSilentlySingleChoice(t *testing.T) {
	for _, answer := range []string{"答案：A、B", "答案：AB", "【答案】A、B"} {
		assets := ExtractQuestions(domain.Document{ID: "d"}, []TextPage{{Number: 1, Text: "1. 虚拟内存有什么特征？\nA. 甲 B. 乙 C. 丙 D. 丁\n" + answer}}, []domain.KnowledgePoint{{ID: "p", Name: "虚拟内存"}})
		require.Len(t, assets, 1)
		require.NotEqual(t, "ready", assets[0].Status)
		require.NotEqual(t, "source", assets[0].Payload.AnswerOrigin)
	}
	answers := ParseAnswerFile([]TextPage{{Number: 1, Text: "参考答案\n1. A、B"}})
	require.Len(t, answers, 1)
	require.True(t, answers[0].Blocked)
}

func TestBracketedExplicitSourceAnswer(t *testing.T) {
	assets := ExtractQuestions(domain.Document{ID: "d"}, []TextPage{{Number: 1, Text: "1. 虚拟内存有什么特征？\nA. 甲 B. 乙\n【答案】A【解析】采用按需装入。"}}, []domain.KnowledgePoint{{ID: "p", Name: "虚拟内存"}})
	require.Len(t, assets, 1)
	require.Equal(t, 0, assets[0].Payload.Answer)
	require.Equal(t, "source", assets[0].Payload.AnswerOrigin)
	require.Contains(t, assets[0].Payload.Explanation, "【解析】")
	require.Equal(t, "乙", assets[0].Payload.Options[1])
	entries := ParseAnswerFile([]TextPage{{Number: 1, Text: "1. 【答案】A、B"}})
	require.Len(t, entries, 1)
	require.True(t, entries[0].Blocked)
}

func TestQuestionImagesFollowTheirOwnTextOffsets(t *testing.T) {
	page := TextPage{Number: 1, Blocks: []domain.DocumentBlock{
		{ID: "q1", Text: "1. 虚拟内存如图是什么？\nA. 甲 B. 乙", Kind: "text"},
		{ID: "i1", Image: "data:image/png;base64,first", Kind: "image"},
		{ID: "q2", Text: "2. 虚拟内存如图是什么？\nA. 丙 B. 丁", Kind: "text"},
		{ID: "i2", Image: "data:image/png;base64,second", Kind: "image"},
	}}
	CanonicalPage(&page)
	assets := ExtractQuestions(domain.Document{ID: "d"}, []TextPage{page}, []domain.KnowledgePoint{{ID: "p", Name: "虚拟内存"}})
	require.Len(t, assets, 2)
	require.Equal(t, []string{"data:image/png;base64,first"}, assets[0].Payload.Images)
	require.Equal(t, []string{"data:image/png;base64,second"}, assets[1].Payload.Images)
}
