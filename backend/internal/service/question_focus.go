package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"regexp"
	"sort"
	"strings"
)

// Explicit protocol names constrain expansion; a shared word such as
// "reliable" cannot replace the protocol the student actually asked about.
var ackToken = regexp.MustCompile(`(?i)\bACK\b`)

var protocolName = regexp.MustCompile(`(?i)(?:TCP|UDP|HDLC|PPP|HTTP[S]?|FTP|SMTP|DNS|DHCP|ARP|ICMP|IPv[46]|OSPF|BGP|RIP|TLS|SSL)`)

func protocolFocus(text string) []string {
	out := []string{}
	seen := map[string]bool{}
	// ASCII boundaries permit Chinese text next to a protocol but not words such
	// as "transcript" containing RIP or UDP embedded in an unrelated identifier.
	for _, loc := range protocolName.FindAllStringIndex(text, -1) {
		asciiLetter := func(b byte) bool { return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' }
		if loc[0] > 0 && asciiLetter(text[loc[0]-1]) || loc[1] < len(text) && asciiLetter(text[loc[1]]) {
			continue
		}
		term := strings.ToUpper(text[loc[0]:loc[1]])
		if !seen[term] {
			seen[term] = true
			out = append(out, term)
		}
	}
	return out
}
func sharesProtocol(focus []string, text string) bool {
	names := protocolFocus(text)
	for _, f := range focus {
		for _, n := range names {
			if f == n {
				return true
			}
		}
	}
	return false
}
func competingProtocol(focus []string, text string) bool {
	for _, n := range protocolFocus(text) {
		found := false
		for _, f := range focus {
			if n == f {
				found = true
			}
		}
		if !found {
			return true
		}
	}
	return false
}

// Preserve complete source sentences and their conditions. Unnamed continuation
// is accepted only immediately after an anchored sentence, in the same paragraph,
// and never after a competing protocol. No generic heading grants relevance.
func focusedProtocolEvidence(q, content string, scope ...string) string {
	focus := protocolFocus(q)
	if len(focus) == 0 {
		return content
	}
	out := []string{}
	scoped := len(scope) > 0 && sharesProtocol(focus, scope[0]) && !competingProtocol(focus, scope[0])
	for _, paragraph := range strings.Split(content, "\n\n") {
		active := scoped
		for _, sentence := range completeSourceSentences(paragraph) {
			if tcpReliabilityOnly(q) && tcpReliabilityOffTopic(sentence) {
				active = false
				continue
			}
			anchored := sharesProtocol(focus, sentence)
			if competingProtocol(focus, sentence) && !anchored {
				active = false
				continue
			}
			continuation := false
			for _, prefix := range []string{"它", "该协议", "其", "当", "若", "如果", "因此", "为此", "发送方", "接收方", "发送者", "接收者", "发送端", "接收端", "超时", "确认", "序号", "累计确认", "快速重传", "重传", "滑动窗口"} {
				continuation = continuation || strings.HasPrefix(sentence, prefix)
			}
			if scoped && tcpReliabilityQuestion(q) {
				for _, word := range []string{"序号", "确认", "重传", "校验", "丢失", "重复", "失序", "滑动窗口", "sequence", "acknowledg", "retransmi", "checksum", "timeout"} {
					continuation = continuation || strings.Contains(strings.ToLower(sentence), word)
				}
			}
			if scoped && protocolWhy(q) && protocolPurpose(sentence) {
				continuation = true
			}
			if anchored || active && continuation {
				out = append(out, sentence)
				active = true
			} else {
				active = false
			}
		}
	}
	return strings.Join(out, "\n")
}
func focusedPaths(q string, paths []domain.KnowledgePath) []domain.KnowledgePath {
	focus := protocolFocus(q)
	out := []domain.KnowledgePath{}
	seen := map[string]bool{}
	for _, p := range paths {
		vm := virtualMemoryWhy(q)
		found := len(focus) == 0 && !vm
		invalid := false
		reliabilityPath := false
		ids := []string{}
		kinds := []string{}
		for _, n := range p.Nodes {
			found = found || sharesProtocol(focus, n.Name)
			for _, mechanism := range []string{"可靠传输", "序号", "确认", "重传", "校验和", "按序交付"} {
				reliabilityPath = reliabilityPath || strings.Contains(n.Name, mechanism)
			}
			if tcpReliabilityOnly(q) && (tcpReliabilityOffTopic(n.Name) || strings.Contains(n.Name, "拥塞控制") || strings.Contains(n.Name, "网络层") || strings.Contains(n.Name, "传输层")) {
				invalid = true
			}
			if vm {
				found = found || strings.Contains(n.Name, "虚拟内存") || strings.Contains(n.Name, "虚拟存储")
			}
			invalid = invalid || len(focus) > 0 && competingProtocol(focus, n.Name)
			key := n.KnowledgePointID
			if key == "" {
				key = strings.ToLower(strings.Join(strings.Fields(n.Name), ""))
			}
			if key == "" {
				key = n.ID
			}
			ids = append(ids, key)
		}
		if !found || invalid || tcpReliabilityOnly(q) && len(p.Nodes) > 1 && !reliabilityPath {
			continue
		}
		reverse := append([]string{}, ids...)
		for i, j := 0, len(reverse)-1; i < j; i, j = i+1, j-1 {
			reverse[i], reverse[j] = reverse[j], reverse[i]
		}
		key := strings.Join(ids, "/")
		if rev := strings.Join(reverse, "/"); rev < key {
			key = rev
		}
		for _, e := range p.Edges {
			kinds = append(kinds, e.RelationType)
		}
		sort.Strings(kinds)
		key += "/" + strings.Join(kinds, ",")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out
}

// A quoted exercise state or a congestion-control detail is not an
// explanation of TCP's reliable byte-stream mechanism.
func tcpReliabilityOffTopic(text string) bool {
	for _, term := range []string{"拥塞窗口", "慢开始", "慢启动", "拥塞避免", "窗口扩大选项", "窗口缩放", "保活", "时间等待"} {
		if strings.Contains(text, term) {
			return true
		}
	}
	lower := strings.ToLower(text)
	return strings.Contains(lower, "cwnd") || strings.Contains(lower, "window scaling") || strings.Contains(lower, "keepalive") || strings.Contains(lower, "time_wait")
}

func tcpReliabilityOnly(question string) bool {
	return tcpReliabilityQuestion(question) && !tcpReliabilityOffTopic(question) &&
		!strings.Contains(question, "拥塞控制")
}

func tcpReliabilityQuestion(q string) bool {
	return sharesProtocol([]string{"TCP"}, q) && strings.Contains(q, "可靠") && (strings.Contains(q, "如何") || strings.Contains(q, "怎么") || strings.Contains(q, "实现") || strings.Contains(q, "机制") || strings.HasPrefix(q, "请讲解"))
}

// This is a minimum coverage check, not a correctness proof or generated answer.
// Definitions alone cannot answer a request to explain the mechanism.
func tcpMechanismCoverage(text string) bool {
	sequence := strings.Contains(text, "序号") || strings.Contains(text, "序列号") || strings.Contains(text, "编号") || strings.Contains(strings.ToLower(text), "sequence")
	ack := strings.Contains(text, "确认") || strings.Contains(text, "应答") || ackToken.MatchString(text) || strings.Contains(strings.ToLower(text), "acknowledg")
	retransmit := strings.Contains(text, "重传") || strings.Contains(text, "重发") || strings.Contains(text, "重新发送") || strings.Contains(strings.ToLower(text), "retransmi")
	return sequence && ack && retransmit
}
