package domain

import "testing"

func TestQuestionPointMatchesPDFSpacingAndProtocolBoundaries(t *testing.T) {
	points := []KnowledgePoint{{ID: "vm", Name: "虚拟内存"}, {ID: "ip", Name: "IP"}, {ID: "ipv6", Name: "IPv6"}}
	for text, want := range map[string]string{
		"虚 拟　内存按需调页":    "vm",
		"该路由器转发IPv6数据报": "ipv6",
		"CHIP的封装":       "",
		"IP的作用":         "ip",
	} {
		if got := MatchQuestionPoint(text, points); got != want {
			t.Errorf("%s: got %q, want %q", text, got, want)
		}
	}
}
