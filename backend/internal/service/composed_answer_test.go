package service

import (
	"context"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
)

type reviewLLM struct {
	answer string
	calls  int
}

func (m *reviewLLM) Answer(context.Context, string, string) (string, error) {
	m.calls++
	return m.answer, nil
}
func TestComposedAnswerRequiresActualEvidenceAndSupportReview(t *testing.T) {
	llm := &reviewLLM{answer: `{"supported":true}`}
	s := RAGService{LLM: llm}
	content := []string{"虚拟内存允许程序分多次调入内存，不要求一次性全部装入。"}
	refs := []domain.Citation{{DocumentID: "textbook", Page: 4}}
	raw := `{"claims":[{"text":"程序可以分批调入内存，无须一次全部装入。","kind":"explanation","evidence":[{"source":1,"quote":"虚拟内存允许程序分多次调入内存，不要求一次性全部装入。"}]}]}`
	answer, sources, ok := s.composedAnswer(context.Background(), raw, "为何使用虚拟内存", content, refs, nil)
	require.True(t, ok)
	require.Contains(t, answer, "分批")
	require.Len(t, sources, 1)
	llm.answer = `{"supported":false}`
	_, _, ok = s.composedAnswer(context.Background(), raw, "为何", content, refs, nil)
	require.False(t, ok)
	llm.calls = 0
	_, _, ok = s.composedAnswer(context.Background(), `{"claims":[{"text":"错误","kind":"explanation","evidence":[{"source":1,"quote":"不存在的引用原文内容"}]}]}`, "为何", content, refs, nil)
	require.False(t, ok)
	require.Zero(t, llm.calls)
}
