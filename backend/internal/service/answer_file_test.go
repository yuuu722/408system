package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSeparateAnswerFileUsesSectionsAndPreservesContinuation(t *testing.T) {
	pages := []TextPage{{Number: 1, Text: "第一章 答案\n1. A\n解析：这是第一题的完整依据。"}, {Number: 2, Text: "这是解析在下一页的后续说明。\n2. B\n第二章 答案\n1. C"}}
	answers := ParseAnswerFile(pages)
	require.Len(t, answers, 3)
	require.Equal(t, 2, answers[0].Extra[0].Page)
	require.Contains(t, AnswerFileText(answers[0]), "后续说明")
	assets := []domain.Asset{{ID: "q1", Kind: "question", Payload: domain.AssetPayload{QuestionNumber: "1", SectionKey: "第一章"}}, {ID: "q2", Kind: "question", Payload: domain.AssetPayload{QuestionNumber: "1", SectionKey: "第二章"}}}
	pairs := MatchAnswerFile(assets, answers, "", "")
	require.Len(t, pairs, 2)
	require.Equal(t, 0, pairs[0].Answer.Answer)
	require.Equal(t, 2, pairs[1].Answer.Answer)
	answers = append(answers, answers[0])
	require.Len(t, MatchAnswerFile(assets, answers, "", ""), 1)
}
func TestAnswerFileRejectsUncertainAndPublishedMatches(t *testing.T) {
	assets := []domain.Asset{{Kind: "question", Payload: domain.AssetPayload{QuestionNumber: "1", SectionKey: "3.1", ReviewStatus: "published"}}, {Kind: "question", Payload: domain.AssetPayload{QuestionNumber: "2", SectionKey: "3.1"}}}
	answers := []FileAnswer{{Number: "1", Section: "第一节", Answer: 0}, {Number: "2", Section: "第一节", Answer: 1, Blocked: true}}
	require.Empty(t, MatchAnswerFile(assets, answers, "3.1", "第一节"))
}
func TestCompactAnswerTableDoesNotLoseEveryOtherEntry(t *testing.T) {
	entries := ParseAnswerFile([]TextPage{{Number: 4, Text: "1. A 2. B 3. D"}})
	require.Len(t, entries, 3)
	require.Equal(t, 1, entries[1].Answer)
}
func TestWhyAnswerKeepsProblemAndRemedy(t *testing.T) {
	contents := []string{"一次性及驻留性使不用的程序占据大量空间，内存不足时作业无法运行。", "虚拟内存利用程序的局部性，只需调入当前需要的部分即可运行。"}
	refs := []domain.Citation{{DocumentID: "book", Page: 1}, {DocumentID: "book", Page: 2}}
	answer, sources, _ := sentenceAnswer("invalid", "为什么需要虚拟内存？", contents, refs, nil)
	require.Contains(t, answer, "无法运行")
	require.Contains(t, answer, "局部性")
	require.Len(t, sources, 2)
	require.False(t, adequateCoverage("为什么需要虚拟内存？", contents[0], contents))
}

func TestQuestionOptionsAreNotAWorkedAnswer(t *testing.T) {
	entries := ParseAnswerFile([]TextPage{{Number: 1, Text: "1. 下列有关内存管理的陈述正确的是\nA. 第一项 B. 第二项"}})
	require.Len(t, entries, 1)
	require.True(t, entries[0].Blocked)
}

func TestAnswerFileFullWidthNumbersAndWhitespaceKeepOriginalQuote(t *testing.T) {
	pages := []TextPage{{Number: 8, Text: "参考答案\n１ Ａ\n２ Ｂ"}}
	answers := ParseAnswerFile(pages)
	require.Len(t, answers, 2)
	require.Equal(t, "1", answers[0].Number)
	require.Equal(t, 0, answers[0].Answer)
	require.Equal(t, "１ Ａ", answers[0].Quote)
	require.Equal(t, "2", answers[1].Number)
	require.Equal(t, 1, answers[1].Answer)
}
