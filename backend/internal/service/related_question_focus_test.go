package service

import (
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestComparisonRelatedQuestionsUseStemNotWrongPointTagOrExplanation(t *testing.T) {
	a := func(id, stem string) domain.Asset {
		return domain.Asset{ID: id, Kind: "question", PointID: "same-wrong-tag", Status: "ready", Payload: domain.AssetPayload{Stem: stem, Options: []string{"甲", "乙"}, Explanation: "服务器进程和线程都可能在后台运行。"}}
	}
	candidates := []domain.Asset{a("ftp", "FTP服务器监听哪个端口？"), a("deadlock", "死锁避免与死锁检测有什么区别？"), a("noise", "进程与线程 TRERREAEED RED 如何区别？"), a("valid", "进程与线程在资源共享方面，哪个说法正确？")}
	got := focusedRelatedQuestions("比较进程与线程的区别", candidates)
	require.Len(t, got, 1)
	require.Equal(t, "valid", got[0].ID)
	require.Empty(t, focusedRelatedQuestions("TCP如何实现可靠传输？", []domain.Asset{a("ftp", "FTP服务器监听哪个端口？")}))
}
func TestComparisonFactsKeepBothSidesWhenOneSideHasManySentences(t *testing.T) {
	var left strings.Builder
	for i := 0; i < 24; i++ {
		fmt.Fprintf(&left, "进程具有独立的资源分配信息，第%d种资源由操作系统管理。", i)
	}
	source := []string{left.String(), "线程是调度的基本单位，并保存自己的寄存器状态。"}
	facts := buildAnswerFacts("比较进程与线程的区别", source, []domain.Citation{{DocumentID: "left"}, {DocumentID: "right"}}, nil)
	l, r := false, false
	for _, f := range facts.Facts {
		l = l || strings.Contains(f.Text, "进程具有")
		r = r || strings.Contains(f.Text, "线程是调度")
	}
	require.True(t, l)
	require.True(t, r, "one-sided lexical ranking must not consume all 10 facts")
}
