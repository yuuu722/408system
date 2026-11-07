package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAnswerPageReferenceDoesNotHideWorkbookQuestions(t *testing.T) {
	pages := []TextPage{{Number: 1, Text: "公众号：排版标记\n第 1 章基础\n1.1 基础概念（答案见原书 P6）\n(01) 操作系统管理什么？\nA. 系统资源 B. 颜色 C. 天气 D. 日期\n·第 1 页，共646页·"}, {Number: 2, Text: "(02) 下列哪个属于系统资源？\nA. 内存 B. 颜色 C. 天气 D. 日期\n·第 2 页，共646页·"}}
	a := ExtractQuestions(domain.Document{ID: "q"}, pages, nil)
	require.Len(t, a, 2)
	require.Equal(t, "1.1", a[0].Payload.SectionKey)
	require.Equal(t, "1", a[0].Payload.QuestionNumber)
	require.Equal(t, "日期", a[0].Payload.Options[3])
	require.NotContains(t, a[0].Payload.Options[3], "页")
}
func TestExerciseAnswerSiblingSectionsAndLeadingZeros(t *testing.T) {
	pages := []TextPage{{Number: 1, Text: "1.1.4 本节习题精选\n01. 操作系统管理什么？\nA. 资源 B. 颜色\n1.1.5 答案与解析\n1. A\n解析：操作系统负责管理系统资源并为应用提供服务。"}}
	a := ExtractQuestions(domain.Document{ID: "q"}, pages, []domain.KnowledgePoint{{ID: "p", Name: "操作系统"}})
	require.Len(t, a, 1)
	require.Equal(t, 0, a[0].Payload.Answer)
	require.Contains(t, a[0].Payload.Explanation, "管理系统资源")
	require.Equal(t, "1.1", a[0].Payload.SectionKey)
}
func TestSavedGeometryRejoinsDetachedNumbersWithoutChangingContent(t *testing.T) {
	p := TextPage{Number: 1, Blocks: []domain.DocumentBlock{
		{ID: "n1", Kind: "text", Text: "01.", Box: []float64{.05, .1, .09, .12}},
		{ID: "n2", Kind: "text", Text: "02.", Box: []float64{.05, .3, .09, .32}},
		{ID: "q1", Kind: "text", Text: "Which value equals x^{2}?", Box: []float64{.11, .1, .8, .12}},
		{ID: "a1", Kind: "text", Text: "A. x*x", Box: []float64{.11, .14, .4, .16}},
		{ID: "b1", Kind: "text", Text: "B. x+2", Box: []float64{.5, .14, .8, .16}},
		{ID: "q2", Kind: "text", Text: "Which item is a resource?", Box: []float64{.11, .3, .8, .32}},
		{ID: "a2", Kind: "text", Text: "A. Memory", Box: []float64{.11, .34, .4, .36}},
		{ID: "b2", Kind: "text", Text: "B. Colour", Box: []float64{.5, .34, .8, .36}},
	}}
	CanonicalPage(&p)
	original := p.Text
	repaired, ok := repairQuestionReadingPage(p)
	require.True(t, ok)
	require.Contains(t, repaired.Text, "01. Which value equals x^{2}?")
	a := ExtractQuestions(domain.Document{ID: "q"}, []TextPage{p}, nil)
	require.Len(t, a, 2)
	require.Equal(t, "x*x", a[0].Payload.Options[0])
	require.Equal(t, original, p.Text)
	p.Blocks[1].Box = []float64{.55, .3, .59, .32}
	_, ok = repairQuestionReadingPage(p)
	require.False(t, ok, "independent columns cannot be silently interleaved")
	p.Blocks[1].Box = nil
	_, ok = repairQuestionReadingPage(p)
	require.False(t, ok, "missing geometry must not be guessed from order")
}
func TestCompanionBookRequiresOriginalBookReferencePublisherSubjectAndYear(t *testing.T) {
	qname := "【横版】26王道操作系统选择题做题本.pdf"
	qp := []TextPage{{Number: 1, Text: "1.1 基础概念（答案见原书 P6）"}}
	ap := []TextPage{{Number: 2, Text: "2026 年操作系统考研复习指导 / 王道论坛组编"}}
	require.True(t, MaybeCompanionStudyBook(qname, qp, "2026操作系统.pdf"))
	require.True(t, CompanionStudyBooks(qname, qp, "2026操作系统.pdf", ap))
	require.False(t, CompanionStudyBooks(qname, qp, "2027操作系统.pdf", ap))
	require.False(t, CompanionStudyBooks(qname, qp, "2026数据结构.pdf", ap))
	require.False(t, CompanionStudyBooks(qname, qp, "26竟成操作系统考研复习全书.pdf", ap))
	require.False(t, CompanionStudyBooks(qname, nil, "2026操作系统.pdf", ap))
	require.False(t, CompanionStudyBooks("26王道操作系统模拟题做题本.pdf", qp, "2026操作系统.pdf", ap))
}
func TestBookAnswerViewExcludesTheoryAndKeepsTopicAcrossRunningHeader(t *testing.T) {
	pages := []TextPage{{Number: 2, Text: "2026 年操作系统考研复习指导 / 王道论坛组编"},
		{Number: 3, Text: "1.1.1 基本概念\n1. A 是某个变量的说明。\n1.1.4 本节习题精选\n01. 操作系统管理什么？\nA. 资源 B. 日期\n1.1.5 答案与解析\n01. A\n解析：操作系统负责管理系统资源。"},
		{Number: 4, Text: "第 1 章 基础概念 7\n02. B\n解析：应用通过系统调用使用操作系统提供的服务。"}}
	entries := ParseDocumentAnswers("2026操作系统.pdf", pages)
	require.Len(t, entries, 2)
	require.Equal(t, "1.1", entries[0].Section)
	require.Equal(t, "1.1", entries[1].Section)
	require.Equal(t, "1", entries[0].Number)
	require.Equal(t, 1, entries[1].Answer)
	for _, e := range entries {
		require.NotContains(t, e.Quote, "某个变量")
	}
}
func TestSuspectFormulaNeverBecomesAutoGradedQuestion(t *testing.T) {
	p := []TextPage{{Number: 1, Text: "1. 计数值为 N-! 时哪个正确？\nA. 甲 B. 乙\n答案：A\n解析：这里有足够长的文字但公式仍然不可确认。"}}
	a := ExtractQuestions(domain.Document{ID: "q"}, p, []domain.KnowledgePoint{{ID: "p", Name: "计数"}})
	require.Len(t, a, 1)
	require.Contains(t, a[0].Payload.Issue, "OCR")
	require.NotEqual(t, "ready", a[0].Status)
}

func TestPrintedPageFooterDoesNotBecomeLastOption(t *testing.T) {
	for _, footer := range []string{"·第 1 页，共646页·", "第2页 (共8页)"} {
		a := ExtractQuestions(domain.Document{ID: "q"}, []TextPage{{Number: 1, Text: "1. 哪项正确？\nA. 正确 B. 错误\n" + footer}}, nil)
		require.Len(t, a, 1)
		require.Equal(t, "错误", a[0].Payload.Options[1])
	}
}

func TestReformattedQuestionKeepsPublishedSourceAnchor(t *testing.T) {
	a := domain.Asset{Kind: "question", Payload: domain.AssetPayload{DocumentID: "d", QuestionNumber: "01", SectionKey: "1.1.4", Page: 6}}
	b := a
	b.Payload.QuestionNumber = "1"
	b.Payload.SectionKey = "1.1"
	b.Payload.ExtractionKey = "new-derived-hash"
	require.True(t, SameQuestionSourceAnchor(a, b))
	b.Payload.Page = 7
	require.False(t, SameQuestionSourceAnchor(a, b))
	b.Payload.Page = 6
	b.Payload.DocumentID = "other"
	require.False(t, SameQuestionSourceAnchor(a, b))
}

func TestCalculationSubstepsDoNotCreateSpuriousAnswerKeys(t *testing.T) {
	p := []TextPage{{Number: 1, Text: "1. 【参考答案】B\n解析：需要依次执行下列栈操作来计算结果。\n1 读取表达式中的第一个元素并压栈\n2 再读取下一个元素并执行出栈操作\n3 最终得到正确的计算结果\n4 C x + y * z\n2. 【人参考答案】A\n解析：这一题使用队列先进先出的性质得到结果。"}}
	entries := ParseAnswerFile(p)
	require.Len(t, entries, 2)
	require.Equal(t, "1", entries[0].Number)
	require.Equal(t, 1, entries[0].Answer)
	require.Contains(t, entries[0].Quote, "3 最终得到")
	require.Equal(t, "2", entries[1].Number)
	require.Equal(t, 0, entries[1].Answer)
}

func TestObservedOptionCaseDuplicationAndCommaLabels(t *testing.T) {
	for _, text := range []string{
		"1. 下列哪个正确？\nA. 第一项 B. 第二项 c. 第三项 D. 第四项",
		"1. 下列哪个正确？\nA. 第一项 B. 第二项 Cc. 第三项 D. 第四项",
		"1. 下列哪个正确？\nA. 第一项 B. 第二项 C，第三项 D，第四项",
	} {
		a := ExtractQuestions(domain.Document{ID: "q"}, []TextPage{{Number: 1, Text: text}}, nil)
		require.Len(t, a, 1)
		require.Len(t, a[0].Payload.Options, 4)
		require.Equal(t, "第三项", a[0].Payload.Options[2])
		require.Equal(t, "第四项", a[0].Payload.Options[3])
	}
}

func TestObservedMissingSectionSeparatorNeedsExplicitPriorTopic(t *testing.T) {
	scope, answer, ok := questionHeadingContext("12.8 答案与解析", "1.2")
	require.True(t, ok)
	require.True(t, answer)
	require.Equal(t, "1.2", scope)
	scope, _, _ = questionHeadingContext("12.8 答案与解析", "")
	require.Equal(t, "12.8", scope)
	scope, _, _ = questionHeadingContext("3.12 答案与解析", "3.1")
	require.Equal(t, "3.12", scope)
	scope, _, _ = questionHeadingContext("3.12.5 答案与解析", "3.1")
	require.Equal(t, "3.12", scope)
}
