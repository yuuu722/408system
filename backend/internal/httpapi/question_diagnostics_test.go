package httpapi

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDiagnosticAnswerHeadingDoesNotSelectCoverHint(t *testing.T) {
	for _, line := range []string{"使用建议 先独立完成试卷，再查看文末答案与参考要点", "本书答案与解析见原书P6", "1.1.5 答案与解析4"} {
		require.False(t, diagnosticAnswerHeading.MatchString(line), line)
	}
	for _, line := range []string{"参考答案与解析", "第 1 套：参考答案", "模拟卷一 参考答案与要点", "答案与参考要点"} {
		require.True(t, diagnosticAnswerHeading.MatchString(line), line)
	}
}
