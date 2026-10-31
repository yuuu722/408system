package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestReportedDuplicateAndTruncatedTextDoesNotBecomeAnswer(t *testing.T) {
	first := "答案如下。\n\n1. 为什么要引入虚拟内存?\n\n上一节提到过，多道程序并发执行共享了主存。\n但是，若同时运行的进程太多，则需要很多的内存，当一个程序没有内存空间可用时，那么它甚至无法运行。所以，"
	guide := "000H | 代码页面1\n3.2 虚拟内存管理\n在学习本节时，请读者思考以下问题：\n1. 为什么要引入虚拟内存?\n读者要掌握虚拟内存解决问题的思想。\n3.2.1 虚拟内存的基本概念\n1) 一次性。作业必须一次性全部装入内存后，才能开始运行。这会导致内存不足时大作业无法运行。\n2) 驻留性。作业被装入内存后，就一直驻留在内存中。\n由以上分析可知，许多暂时不用的程序占据了大量内存空间，"
	contents := []string{first, first, guide}
	refs := []domain.Citation{{DocumentID: "book", Page: 237, ChunkID: "one"}, {DocumentID: "book", Page: 237, ChunkID: "duplicate"}, {DocumentID: "book", Page: 224, ChunkID: "three"}}
	answer, sources, _ := sentenceAnswer("invalid model format", "为什么要引入虚拟内存？", contents, refs, nil)
	require.NotContains(t, answer, "答案如下")
	require.NotContains(t, answer, "000H")
	require.NotContains(t, answer, "思考以下")
	require.NotContains(t, answer, "所以，")
	require.NotContains(t, answer, "为什么要引入")
	require.Contains(t, answer, "内存")
	require.False(t, danglingSentence(answer))
	require.LessOrEqual(t, len([]rune(answer)), 1100)
	seen := map[string]bool{}
	for _, part := range strings.Split(answer, "\n\n") {
		require.False(t, seen[part])
		seen[part] = true
	}
	sourceKeys := map[string]bool{}
	for _, s := range sources {
		key := s.DocumentID + s.Location + string(rune(s.Page))
		require.False(t, sourceKeys[key])
		sourceKeys[key] = true
	}
}
func TestModelCanSelectIDsButCannotInsertFacts(t *testing.T) {
	contents := []string{"虚拟内存通过按需调入提高内存利用率。"}
	refs := []domain.Citation{{DocumentID: "b", Page: 1}}
	answer, sources, model := sentenceAnswer(`{"selected":[1],"answer":"磁盘上的页框"}`, "为什么需要虚拟内存？", contents, refs, nil)
	require.True(t, model)
	require.Equal(t, contents[0], answer)
	require.Len(t, sources, 1)
	require.NotContains(t, answer, "磁盘")
	answer, _, model = sentenceAnswer(`{"selected":[999]}`, "为什么需要虚拟内存？", contents, refs, nil)
	require.False(t, model)
	require.Equal(t, contents[0], answer)
}

func TestWhitespaceCleanupDoesNotEraseMathOrWindowsPathSeparators(t *testing.T) {
	require.Equal(t, "内存空间", sourceWhitespace("内存\\\n空间"))
	require.Contains(t, sourceWhitespace("路径 C:\\\nWindows"), "C:\\")
	require.Contains(t, sourceWhitespace("公式 \\\\\n x"), "\\\\")
}

func TestInlineQuestionDoesNotHideFollowingExplanation(t *testing.T) {
	parts := completeSourceSentences("为什么需要虚拟内存？虚拟内存通过按需调入提高内存利用率。")
	require.Contains(t, parts, "虚拟内存通过按需调入提高内存利用率。")
}

func TestWhyQuestionDoesNotSubstituteImplementationPrerequisite(t *testing.T) {
	contents := []string{"虚拟内存技术允许将一个作业分多次调入内存。采用连续分配方式时，会使相当一部分内存空间处于空闲状态。因此，虚拟内存的实现需要建立在离散分配的内存管理方式的基础上。", "传统存储方式要求一次性全部装入，当内存不足时作业无法运行。虚拟内存利用程序的局部性，允许程序分次调入内存。"}
	refs := []domain.Citation{{DocumentID: "book", Page: 1}, {DocumentID: "book", Page: 2}}
	answer, _, _ := sentenceAnswer("invalid", "为什么需要虚拟内存？", contents, refs, nil)
	require.NotContains(t, answer, "采用连续分配")
	require.NotContains(t, answer, "实现需要建立")
	require.Contains(t, answer, "内存不足")
	require.Contains(t, answer, "局部性")
}
