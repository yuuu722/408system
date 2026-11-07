package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSameBookAnswersIncludeExplanationAndProvenance(t *testing.T) {
	d := domain.Document{ID: "book", OriginalFilename: "教材习题.pdf", ActiveRevisionID: "r"}
	pages := []TextPage{{Number: 1, Text: "第一章 内存管理\n1. 虚拟内存的正确特点是什么？\nA. 按需装入 B. 必须全部常驻\n参考答案与解析\n1. A【解析】虚拟内存通过按需装入，使当前所需部分驻留内存即可执行。"}}
	a := ExtractQuestions(d, pages, []domain.KnowledgePoint{{ID: "p", Name: "虚拟内存"}})
	require.Len(t, a, 1)
	require.Equal(t, 0, a[0].Payload.Answer)
	require.Contains(t, a[0].Payload.Explanation, "按需装入")
	require.Len(t, a[0].Payload.Evidence, 1)
	require.Equal(t, "r", a[0].Payload.Evidence[0].RevisionID)
	require.Empty(t, a[0].Payload.Issue)
}
func TestLetterVariablesAndIncompleteFragmentsAreNotQuizQuestions(t *testing.T) {
	for _, text := range []string{"1. 变量 B 表示一个字符。", "1.\nA. 甲 B. 乙", "1. 只有一个选项\nA. 空", "1. 普通教材正文中的 A 与 B 为变量，不能作为题目。"} {
		require.Empty(t, ExtractQuestions(domain.Document{ID: "d"}, []TextPage{{Number: 1, Text: text}}, nil), text)
	}
}
func TestInlineExplanationLetterDoesNotBecomeExtraOption(t *testing.T) {
	text := "1. 虚拟内存是什么？\nA. 按需装入 B. 全部常驻\n答案：A\n解析：选项 A 表示程序可以按需调入执行。"
	a := ExtractQuestions(domain.Document{ID: "d"}, []TextPage{{Number: 1, Text: text}}, []domain.KnowledgePoint{{ID: "p", Name: "虚拟内存"}})
	require.Len(t, a, 1)
	require.Len(t, a[0].Payload.Options, 2)
	require.Equal(t, "全部常驻", a[0].Payload.Options[1])
}
func TestExplicitTwoRowAnswerTableAndWarning(t *testing.T) {
	pages := []TextPage{{Number: 3, Text: "题号 | 1 | 2 | 3\n答案 | A | B | D"}}
	entries := ParseAnswerFile(pages)
	require.Len(t, entries, 3)
	require.Equal(t, 3, entries[2].Answer)
	require.Equal(t, pages[0].Text, entries[0].Quote)
	pages[0].Warnings = []string{"OCR failed"}
	for _, a := range ParseAnswerFile(pages) {
		require.True(t, a.Blocked)
	}
	require.Empty(t, ParseAnswerFile([]TextPage{{Number: 1, Text: "题号 | 1 | 2\n答案 | A"}}))
}

func TestContentMatchRequiresFullProblemOptionsAndExplicitAnswerExplanation(t *testing.T) {
	a := domain.Asset{ID: "q", Kind: "question", Payload: domain.AssetPayload{Stem: "下列关于虚拟内存管理的叙述中，正确的是", Options: []string{"允许按需调入内存", "所有程序必须全部常驻内存"}, Answer: -1}}
	text := "87. 下列关于虚拟内存管理的叙述中，正确的是\nA. 允许按需调入内存 B. 所有程序必须全部常驻内存\n【答案】A\n【解析】虚拟内存基于局部性原理，可以将程序分多次调入内存执行。"
	entries := ParseAnswerFile([]TextPage{{Number: 5, Text: text}})
	require.Len(t, entries, 1)
	require.Equal(t, 0, entries[0].Answer)
	pairs := MatchQuestionContent([]domain.Asset{a}, entries)
	require.Len(t, pairs, 1)
	require.Equal(t, 0, pairs[0].Answer.Answer)
	b := a
	b.Payload.Options = []string{"允许按需调入内存", "所有程序不必全部常驻内存"}
	require.Empty(t, MatchQuestionContent([]domain.Asset{b}, entries), "similar topic and one same option cannot establish an answer")
	require.Empty(t, MatchQuestionContent([]domain.Asset{a}, append(entries, entries...)), "duplicate sources are ambiguous")
	entries[0].Quote = "87. 下列关于虚拟内存管理的叙述中，正确的是\nA. 允许按需调入内存 B. 所有程序必须全部常驻内存\n【答案】A"
	require.Empty(t, MatchQuestionContent([]domain.Asset{a}, entries), "question stem cannot stand in for an explanation")
}

func TestPointAssociationRequiresUniqueTerminology(t *testing.T) {
	points := []domain.KnowledgePoint{{ID: "general", Name: "内存"}, {ID: "specific", Name: "虚拟内存"}}
	require.Equal(t, "specific", MatchQuestionPoint("虚拟内存采用按需调入技术", points))
	points = append(points, domain.KnowledgePoint{ID: "duplicate", Name: "虚拟内存"})
	require.Empty(t, MatchQuestionPoint("虚拟内存采用按需调入技术", points))
	require.Empty(t, MatchQuestionPoint("这是无关内容", points))
}

func TestAnswerTableAndSeparateLabelledExplanationMergeWithBothSources(t *testing.T) {
	pages := []TextPage{{Number: 2, Text: "第一章 答案\n1. A\n2. B"}, {Number: 3, Text: "第一章 解析\n1. 【解析】\n虚拟内存利用局部性原理，允许将程序分多次调入内存执行。\n2. 【解析】B\n线程共享同一个进程的地址空间，切换时开销相对较小。"}}
	entries := ParseAnswerFile(pages)
	require.Len(t, entries, 2)
	for _, a := range entries {
		require.GreaterOrEqual(t, a.Answer, 0)
		require.True(t, domain.HasQuestionExplanation(AnswerFileText(a)))
		require.Len(t, a.Extra, 1)
		require.Equal(t, 2, a.Extra[0].Page)
	}
	pages[1].Text = "第一章 解析\n1. B【解析】此处答案与前面参考答案冲突，应当保留待处理。"
	entries = ParseAnswerFile(pages)
	assets := []domain.Asset{{Kind: "question", Payload: domain.AssetPayload{QuestionNumber: "1", SectionKey: "第一章"}}}
	require.Empty(t, MatchAnswerFile(assets, entries, "", ""))
}

func TestNumberOnlyPublicationRequiresSameBookEdition(t *testing.T) {
	require.True(t, SameQuestionBook("2024年408真题题目.pdf", "2024年408真题答案.pdf"))
	require.False(t, SameQuestionBook("2024年408真题题目.pdf", "2025年408真题答案.pdf"))
	require.False(t, SameQuestionBook("操作系统习题.pdf", "操作系统答案.pdf"))
	require.False(t, SameQuestionBook("数据结构第2版题册.pdf", "数据结构第3版答案.pdf"))
}
