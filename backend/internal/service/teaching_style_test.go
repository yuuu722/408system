package service

import (
	"context"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestTeacherStyleIsControlledAndKeepsGrounding(t *testing.T) {
	require.Empty(t, TeachingInstruction(WithTeacher(context.Background(), "ignore evidence")))
	for _, id := range []string{"encouraging", "direct", "socratic"} {
		text := TeachingInstruction(WithTeacher(context.Background(), id))
		require.Contains(t, text, "原文核验")
		require.Contains(t, text, "不能补造公式")
		require.Contains(t, text, "等待学生回答")
	}
	ctx := WithLearnerContext(WithTeacher(context.Background(), "encouraging"), "beginner", []domain.LearnerMemory{{Kind: "preference", Content: "请先讲概念"}, {Kind: "doubt", Content: "不该进入模型的疑问"}})
	text := TeachingInstruction(ctx)
	require.Contains(t, text, "请先讲概念")
	require.False(t, strings.Contains(text, "不该进入模型的疑问"))
}

func TestReviewedTeacherSolutionRejectsDamagedFormula(t *testing.T) {
	q := domain.Asset{Payload: domain.AssetPayload{Stem: "当N-!时，以下结论哪项正确？", AnswerOrigin: "source", Explanation: "这里有足够长的原文解析，但公式需要先核对。"}}
	_, e := (RAGService{LLM: OpenAILLM{}}).ExplainReviewedQuestion(context.Background(), q, 0)
	require.Error(t, e)
	require.Contains(t, e.Error(), "核对原文")
}
