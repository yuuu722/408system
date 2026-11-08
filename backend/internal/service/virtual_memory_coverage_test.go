package service

import "testing"

func TestVirtualMemoryEvidenceCoverage(t *testing.T) {
	q := "为什么需要虚拟内存？"
	evidence := []string{"一次性和驻留性使暂时不用的数据占据内存，程序超过物理内存时无法装入。", "基于局部性原理，仅须将当前要运行的部分装入内存，其余部分需要时请求调入。"}
	cases := []struct{ name, answer, issue string }{
		{"user short answer", "传统存储管理的一次性和驻留性导致浪费，通过对换区提高利用率。", "virtual_memory_coverage"},
		{"corrupted term", "传统存储管理的单一性及驻留性导致浪费。", "virtual_memory_term"},
		{"absolute", "虚拟内存避免了内存资源的浪费。", "virtual_memory_overclaim"},
		{"complete", "传统管理的一次性和驻留性使暂时不用的数据占用内存，程序超过物理内存时无法装入。基于局部性，只装入当前要运行的部分，其余部分请求调入。", ""},
		{"negation", "不能完全避免内存浪费。一次性和驻留性导致程序无法装入，基于局部性按需装入。", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := virtualMemoryAnswerIssue(q, tc.answer, evidence); got != tc.issue {
				t.Fatalf("got %q want %q", got, tc.issue)
			}
		})
	}
	if got := virtualMemoryAnswerIssue(q, "通过对换提高内存利用率。", []string{"通过对换提高内存利用率。"}); got != "" {
		t.Fatalf("required absent evidence: %s", got)
	}
	if got := virtualMemoryAnswerIssue("PPP有什么用？", "单一性", evidence); got != "" {
		t.Fatal("affected another topic")
	}
}

func TestBalanceVirtualMemoryBeforeBudget(t *testing.T) {
	var sentences []answerSentence
	for i := 0; i < 20; i++ {
		sentences = append(sentences, answerSentence{Text: "一次性和驻留性导致浪费。"})
	}
	sentences = append(sentences, answerSentence{Text: "程序超过物理内存时无法装入。"}, answerSentence{Text: "基于局部性原理只装入当前要运行的部分。"})
	got := balanceVirtualMemorySentences("为什么需要虚拟内存？", sentences)
	var mask uint8
	for _, sentence := range got[:3] {
		mask |= virtualMemoryAspects(sentence.Text)
	}
	if mask != 15 {
		t.Fatalf("complementary evidence crowded out: %04b", mask)
	}
	if len(got) != len(sentences) {
		t.Fatal("lost source sentences")
	}
}
