package service

import (
	"context"
	"encoding/json"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestComparisonObjectsAndEvidenceScope(t *testing.T) {
	spec := comparisonTargets("比较进程与线程的区别")
	require.NotNil(t, spec)
	require.Equal(t, "进程", spec.Left)
	require.Equal(t, "线程", spec.Right)
	require.NotNil(t, comparisonTargets("TCP和UDP有什么区别？"))
	require.Nil(t, comparisonTargets("比较一下"))
	text := "进程与程序有本质区别，进程是动态的而程序是静态的。进程是资源分配的基本单位。线程是调度的基本单位。自旋锁与信号量的主要区别在于是否阻塞进程。学习操作系统，首先必须理解和掌握好进程的概念。"
	got := comparisonEvidence("比较进程与线程的区别", text)
	require.Contains(t, got, "资源分配")
	require.Contains(t, got, "线程是调度")
	require.NotContains(t, got, "程序")
	require.NotContains(t, got, "自旋锁")
	require.NotContains(t, got, "首先必须")
}
func TestStructuredComparisonBindsEachSideAndRejectsOldParagraph(t *testing.T) {
	catalog := factCatalog{Facts: []answerFact{{ID: 1, Source: 1, Text: "进程是资源分配的基本单位。"}, {ID: 2, Source: 1, Text: "同一进程的线程共享进程资源，每个线程有自己的栈。"}}}
	spec := comparisonTargets("进程与线程的区别")
	good := `{"comparisons":[{"dimension":"资源","left":"进程是资源分配的基本单位。","right":"同一进程的线程共享进程资源，各自有独立的栈。","left_facts":[1],"right_facts":[2]}]}`
	expanded, rows, reason := expandComparison(good, spec, catalog)
	require.Empty(t, reason)
	require.Len(t, rows, 1)
	require.Contains(t, expanded, "资源")
	for _, raw := range []string{`{"claims":[{"text":"进程与线程在各个方面有区别。","facts":[1]}]}`, `{"comparisons":[{"dimension":"资源","left":"进程与线程有区别。","right":"线程与进程不一样。","left_facts":[1],"right_facts":[2]}]}`, `{"comparisons":[{"dimension":"资源","left":"进程是资源分配的基本单位。","right":"线程独立拥有所有资源。","left_facts":[1],"right_facts":[1]}]}`} {
		_, _, why := expandComparison(raw, spec, catalog)
		require.NotEmpty(t, why)
	}
}

type comparisonReviewModel struct{ raw string }

func (m comparisonReviewModel) Answer(context.Context, string, string) (string, error) {
	return m.raw, nil
}
func TestComparisonStillRequiresIndependentSupportReview(t *testing.T) {
	source := "进程是资源分配的基本单位。线程共享进程资源，每个线程有自己的栈。"
	catalog := buildAnswerFacts("比较进程与线程的区别", []string{source}, []domain.Citation{{DocumentID: "d", Page: 1}}, nil)
	require.NotNil(t, catalog.Comparison)
	left, right := 0, 0
	for _, fact := range catalog.Facts {
		if strings.HasPrefix(fact.Text, "进程是") {
			left = fact.ID
		}
		if strings.HasPrefix(fact.Text, "线程共享") {
			right = fact.ID
		}
	}
	encoded, _ := json.Marshal(map[string]any{"comparisons": []comparisonRow{{Dimension: "资源", Left: "进程是资源分配的基本单位。", Right: "线程共享进程资源，每个线程有自己的栈。", LeftFacts: []int{left}, RightFacts: []int{right}}}})
	raw := string(encoded)
	var view map[string]any
	require.NoError(t, json.Unmarshal([]byte(catalog.JSON()), &view))
	require.Contains(t, view, "comparison")
	schema := factResponseFormat(catalog.JSON())
	b, _ := json.Marshal(schema)
	require.Contains(t, string(b), "left_facts")
	for _, supported := range []bool{true, false} {
		verdict := `{"supported":false}`
		if supported {
			verdict = `{"supported":true}`
		}
		rag := RAGService{LLM: comparisonReviewModel{verdict}}
		_, citations, ok, why := rag.factComposedAnswer(context.Background(), raw, "比较进程与线程的区别", catalog, []string{source}, []domain.Citation{{DocumentID: "d", Page: 1}}, nil)
		require.Equal(t, supported, ok, why)
		if supported {
			require.NotEmpty(t, citations)
		} else {
			require.Equal(t, "unsupported_claim", why)
		}
	}
}

func TestComparisonCannotDropThreadImplementationPremise(t *testing.T) {
	spec := comparisonTargets("比较进程与线程的区别")
	catalog := factCatalog{Facts: []answerFact{{ID: 1, Source: 1, Text: "进程是资源分配的基本单位。"}, {ID: 2, Source: 1, Text: "用户级线程的调度由用户空间线程库完成。"}}}
	for _, tc := range []struct{ text, reason string }{{"线程的调度由用户空间线程库完成。", "missing_premise"}, {"用户级线程的调度由用户空间线程库完成。", ""}} {
		raw, _ := json.Marshal(map[string]any{"comparisons": []comparisonRow{{Dimension: "管理方式", Left: "进程是资源分配的基本单位。", Right: tc.text, LeftFacts: []int{1}, RightFacts: []int{2}}}})
		_, _, why := expandComparison(string(raw), spec, catalog)
		require.Equal(t, tc.reason, why)
	}
	require.NotNil(t, comparisonTargets("自旋锁和信号量有什么不同"))
	require.Contains(t, comparisonEvidence("进程与程序的区别", "进程与程序有本质区别，进程是动态的而程序是静态的。"), "程序是静态")
}

func TestComparisonSupportsAnother408Pair(t *testing.T) {
	spec := comparisonTargets("比较栈与队列的区别")
	require.NotNil(t, spec)
	catalog := factCatalog{Facts: []answerFact{{ID: 1, Source: 1, Text: "栈采用后进先出的访问次序。"}, {ID: 2, Source: 1, Text: "队列采用先进先出的访问次序。"}}}
	raw := `{"comparisons":[{"dimension":"访问次序","left":"栈采用后进先出的访问次序。","right":"队列采用先进先出的访问次序。","left_facts":[1],"right_facts":[2]}]}`
	_, rows, why := expandComparison(raw, spec, catalog)
	require.Empty(t, why)
	rendered := renderComparison(spec, rows)
	require.Contains(t, rendered, "| 比较维度 | 栈 | 队列 |")
}
