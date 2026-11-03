package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestReportedTCPKeepaliveDoesNotStandInForReliableDataTransfer(t *testing.T) {
	q := "TCP 如何实现可靠传输？"
	text := "TCP通过检验、序号、确认和重传等机制实现这一目的。\n除时间等待计时器外，TCP还使用一个保活计时器，以防止客户突发故障导致服务器长期无效等待。\nTCP在不可靠的IP层之上提供可靠传输服务，确保接收方从缓存中读取的字节流与发送方发出的字节流完全一致。"
	actual := answerEvidence(q, text)
	require.NotContains(t, actual, "保活")
	require.NotContains(t, actual, "时间等待")
	require.Contains(t, actual, "序号")
	require.Contains(t, focusedProtocolEvidence("TCP保活计时器如何工作？", text), "保活")
}
func TestConfirmedTCPHeadingPreservesMechanismParagraphWithoutRepeatedAcronym(t *testing.T) {
	q := "TCP 如何实现可靠传输？"
	body := "发送方对字节流中的数据编号，序号用于接收方恢复正确的顺序。接收方通过确认号说明期望接收的数据。若等待确认超时，发送方重传未被确认的数据。"
	require.Empty(t, answerEvidence(q, body, "传输层 TCP"), "broad retrieval hint is not a source heading")
	actual := answerEvidence(q, body, "", "5.6 TCP可靠传输的实现")
	require.NotEmpty(t, actual)
	require.Contains(t, actual, "等待确认超时")
	refs := []domain.Citation{{DocumentID: "book", Page: 121, Location: "第121页 · 5.6 TCP可靠传输的实现"}}
	answer, sources, _ := sentenceAnswer("", q, []string{actual}, refs, nil)
	require.NotEmpty(t, sources)
	require.True(t, adequateCoverage(q, answer, []string{actual}))
	for _, sentence := range strings.Split(actual, "\n") {
		require.Contains(t, sourceWhitespace(body), sentence)
	}
}

func TestImplicitProtocolScopeCannotAdmitAnotherLayer(t *testing.T) {
	q := "TCP 如何实现可靠传输？"
	body := "HDLC通过确认和重传机制保证数据链路层的可靠性。"
	require.Empty(t, answerEvidence(q, body, "", "5.6 TCP可靠传输的实现"))
}
