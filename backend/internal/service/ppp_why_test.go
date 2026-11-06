package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPPPWhyIsNotSubjectToVirtualMemoryCoverageKeywords(t *testing.T) {
	contents := []string{"IP数据报在PPP帧中是信息部分，其长度受最大传送单元MTU的限制。PPP允许多种网络层协议在点到点链路上传输。"}
	answer := "PPP用于点到点链路，为多种网络层协议的数据传输提供封装和链路配置服务。"
	require.True(t, adequateCoverage("为什么要用PPP", answer, contents), "MTU限制不是PPP使用目的必须重复的方面")
	require.False(t, adequateCoverage("为什么要用PPP", "PPP帧首部有四个字段，尾部有两个字段。", contents))
}
func TestProtocolWhyRetrievesAndRanksPurposeBeforeFrameLayout(t *testing.T) {
	q := "为什么要用PPP"
	topics, needs := textbookIntentTerms(q)
	require.Contains(t, topics, "PPP")
	require.Contains(t, needs, "用于")
	frame := domain.Chunk{Content: "PPP帧首部有四个字段，尾部有两个字段。", Score: .9}
	purpose := domain.Chunk{Content: "PPP用于点到点链路，支持多种网络层协议的数据传输，并提供建立、配置和测试链路的功能。", Score: .7}
	require.Greater(t, questionFocusScore(q, purpose), questionFocusScore(q, frame))
	sentences := sentenceCandidates(q, []string{frame.Content, purpose.Content})
	require.NotEmpty(t, sentences)
	require.Equal(t, 2, sentences[0].Source)
}
func TestProtocolWhyKeepsHeadingScopedPurposeAndRejectsWrongProtocol(t *testing.T) {
	text := "该协议用于点到点链路的数据封装。它提供链路配置功能。TCP通过序号实现可靠传输。"
	focused := focusedProtocolEvidence("为什么要用PPP", text, "数据链路层 / PPP协议")
	require.Contains(t, focused, "点到点链路")
	require.NotContains(t, focused, "TCP")
}

func TestChinesePromptDoesNotFuseWithProtocolToken(t *testing.T) {
	require.True(t, evidenceRelated("为什么要用PPP", "PPP用于点到点链路。"))
	require.False(t, evidenceRelated("为什么要用PPP", "TCP使用序号和确认。"))
	require.Equal(t, "第2层 PPP 协议 IPv6 地址", separateQueryScripts("第2层PPP协议IPv6地址"))
}
