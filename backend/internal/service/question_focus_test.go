package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestReportedTCPResultCannotUseLinkLayerEvidence(t *testing.T) {
	q := "TCP 如何实现可靠传输？"
	noisy := "在通信线路质量较差的年代，在数据链路层使用可靠传输协议曾经是一种好办法。\n因此，能实现可靠传输的高级数据链路控制HDLC就成为当时比较流行的数据链路层协议。\nTCP在不可靠的IP层之上提供可靠传输服务，确保接收方从缓存中读取的字节流与发送方发出的字节流完全一致。\n对于通信质量较差的无线传输链路，数据链路层协议使用确认和重传机制，数据链路层向上提供可靠传输的服务。"
	result := answerEvidence(q, noisy, "传输层")
	require.Contains(t, result, "TCP")
	require.NotContains(t, result, "HDLC")
	require.NotContains(t, result, "无线传输")
	require.NotContains(t, result, "年代")
	require.False(t, evidenceRelated(q, "HDLC使用确认和重传实现可靠传输。"))
	require.False(t, adequateCoverage(q, result, []string{result}), "a service definition does not explain mechanisms")
	candidates := sentenceCandidates(q, []string{noisy})
	for _, c := range candidates {
		require.NotContains(t, c.Text, "HDLC")
		require.NotContains(t, c.Text, "无线")
	}
	require.Empty(t, answerEvidence(q, "HDLC使用确认和重传机制实现可靠传输。", "传输层 TCP"))
}
func TestTCPMechanismKeepsAnchoredContinuationAndCompleteQuotes(t *testing.T) {
	q := "TCP 如何实现可靠传输？"
	source := "TCP采用序号、确认和重传机制实现可靠传输。发送方对字节流编号，接收方通过确认号指示接下来期望收到的数据。若等待确认超时，发送方重传尚未确认的数据。\n\nHDLC通过确认和重传提供链路层可靠传输。"
	focused := focusedProtocolEvidence(q, source)
	require.Contains(t, focused, "等待确认超时")
	require.NotContains(t, focused, "HDLC")
	require.True(t, adequateCoverage(q, focused, []string{source}))
	require.True(t, evidenceRelated(q, focused))
	for _, part := range strings.Split(focused, "\n") {
		require.Contains(t, sourceWhitespace(source), part)
	}
	require.Equal(t, []string{"TCP", "UDP"}, protocolFocus("比较TCP与UDP的区别"))
	require.Empty(t, protocolFocus("description与script不是协议名"))
	require.NotEmpty(t, focusedProtocolEvidence("TCP与UDP有何区别？", "TCP提供可靠传输。UDP不保证可靠交付。"))
}
func TestProtocolGraphPathsDoNotDriftAndAreDeduplicated(t *testing.T) {
	tcp := domain.GraphNode{ID: "t", Name: "TCP 可靠传输"}
	ack := domain.GraphNode{ID: "ack", Name: "TCP 累计确认"}
	congestion := domain.GraphNode{ID: "cwnd", Name: "TCP 拥塞控制"}
	layer := domain.GraphNode{ID: "l", Name: "传输层"}
	udp := domain.GraphNode{ID: "u", Name: "UDP与传输层端口"}
	ip := domain.GraphNode{ID: "i", Name: "IPv4与子网划分"}
	p := domain.KnowledgePath{Nodes: []domain.GraphNode{tcp, ack}, Edges: []domain.GraphEdge{{RelationType: "part_of"}}}
	paths := []domain.KnowledgePath{p, p, {Nodes: []domain.GraphNode{ack, tcp}, Edges: p.Edges}, {Nodes: []domain.GraphNode{udp, layer, tcp}}, {Nodes: []domain.GraphNode{layer, ip}}, {Nodes: []domain.GraphNode{tcp, layer, ip}}, {Nodes: []domain.GraphNode{tcp, congestion}}}
	result := focusedPaths("TCP 如何实现可靠传输？", paths)
	require.Len(t, result, 1)
	require.Equal(t, p.Nodes, result[0].Nodes)
}

func TestTCPReliabilityRejectsQuotedCongestionExercise(t *testing.T) {
	q := "TCP 如何实现可靠传输？"
	mechanism := "TCP 用字节序号保证按序交付。累计确认号表示下一个期望收到的字节序号。超时未收到确认时，发送方重传未确认数据。"
	shallow := "TCP 用序号、确认和重传实现可靠传输，序号用于按序交付。"
	require.False(t, adequateCoverage(q, shallow, nil))
	require.False(t, completeAnswerText(mechanism+"结果。"))
	exercise := "TCP 的拥塞窗口初始值为 8，当拥塞窗口上升到 12 时网络发生了超时，TCP 使用慢开始和拥塞避免。TCP 选用了窗口扩大选项。"
	require.True(t, adequateCoverage(q, mechanism, nil))
	require.False(t, adequateCoverage(q, mechanism+exercise, nil))
	facts := buildAnswerFacts(q, []string{mechanism + exercise}, []domain.Citation{{}}, nil)
	require.NotEmpty(t, facts.Facts)
	for _, fact := range facts.Facts {
		require.False(t, tcpReliabilityOffTopic(fact.Text))
	}
}
