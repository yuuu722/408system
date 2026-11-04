package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTraditionalMemoryCauseIsAnExplanationEvenWithoutImplementationVerbs(t *testing.T) {
	q := "为什么需要虚拟内存？"
	source := "传统存储管理要求作业一次性全部装入并驻留内存。作业过大时无法运行，暂时不用的部分也占用内存，导致并发度下降。"
	require.NotEmpty(t, answerEvidence(q, source, "虚拟内存的基本概念"))
	causes := domain.Chunk{Content: source, Score: .6}
	mechanism := domain.Chunk{Content: "虚拟内存基于局部性按需调入程序，内存满时进行页面置换。", Score: .9}
	require.Greater(t, questionFocusScore(q, causes), questionFocusScore(q, mechanism))
	topics, needs := textbookIntentTerms(q)
	require.Contains(t, topics, "虚拟内存")
	require.Contains(t, needs, "一次性")
	require.Contains(t, needs, "驻留性")
}
func TestMemoryGraphPathsMustReachMemoryTopicAndDoNotRepeatDisplayConcepts(t *testing.T) {
	vm := domain.GraphNode{ID: "vm1", Name: "虚拟内存与页面置换"}
	memory := domain.GraphNode{ID: "m1", Name: "内存管理"}
	first := domain.KnowledgePath{Nodes: []domain.GraphNode{vm, memory}}
	duplicate := domain.KnowledgePath{Nodes: []domain.GraphNode{{ID: "vm2", Name: vm.Name}, {ID: "m2", Name: memory.Name}}}
	unrelated := domain.KnowledgePath{Nodes: []domain.GraphNode{memory, {ID: "cpu", Name: "浮点数与算术逻辑单元"}}}
	result := focusedPaths("为什么需要虚拟内存？", []domain.KnowledgePath{first, duplicate, unrelated})
	require.Len(t, result, 1)
}
func TestValidationCategoriesDistinguishModelOutputAndEvidenceRejection(t *testing.T) {
	for _, tc := range []struct {
		raw              string
		valid, supported bool
	}{{`{"supported":true}`, true, true}, {`{"supported":false}`, true, false}, {`{"supported":"false"}`, true, false}, {`{"answer":"yes"}`, false, false}} {
		supported, valid := reviewDecision(tc.raw)
		require.Equal(t, tc.valid, valid)
		require.Equal(t, tc.supported, supported)
	}
}
