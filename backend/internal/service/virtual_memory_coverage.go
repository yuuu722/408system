package service

import "strings"

// Aspect hints supplement, rather than replace, quote and semantic verification.
// Require only aspects present in this round's evidence; never supply facts here.
func virtualMemoryAspects(text string) uint8 {
	var mask uint8
	groups := [][]string{
		{"超过物理内存", "超过了物理内存", "超过内存", "超过了内存", "大于物理内存", "装不下", "无法装入", "不能全部装入", "容量限制", "大于可用物理内存", "无法运行"},
		{"一次性", "驻留性", "暂时不用", "浪费", "并发度"},
		{"局部性"},
		{"按需", "请求调入", "分多次", "分次装入", "分批", "不必全部", "无须全部", "无需全部", "仅须", "当前要运行", "当前需要的部分", "当前使用的部分", "只需调入", "只装入"},
	}
	for i, terms := range groups {
		for _, term := range terms {
			if strings.Contains(text, term) {
				mask |= 1 << i
				break
			}
		}
	}
	return mask
}

func virtualMemoryAnswerIssue(q, answer string, contents []string) string {
	if !virtualMemoryWhy(q) {
		return ""
	}
	if strings.Contains(answer, "单一性") {
		return "virtual_memory_term"
	}
	for _, sentence := range strings.FieldsFunc(answer, func(r rune) bool { return strings.ContainsRune("。；！？", r) }) {
		for _, claim := range []string{"避免了内存资源的浪费", "完全避免内存浪费", "完全消除内存浪费", "彻底解决内存不足"} {
			if strings.Contains(sentence, claim) && !strings.Contains(sentence, "不能") && !strings.Contains(sentence, "无法") && !strings.Contains(sentence, "并非") && !strings.Contains(sentence, "并不") {
				return "virtual_memory_overclaim"
			}
		}
	}
	required := virtualMemoryAspects(strings.Join(contents, "\n"))
	missing := required &^ virtualMemoryAspects(answer)
	if missing != 0 {
		return "virtual_memory_coverage"
	}
	return ""
}

// Retain complementary explanations before the sentence/fact budgets are applied.
func balanceVirtualMemorySentences(q string, sentences []answerSentence) []answerSentence {
	if !virtualMemoryWhy(q) {
		return sentences
	}
	out := make([]answerSentence, 0, len(sentences))
	used := make([]bool, len(sentences))
	var covered uint8
	for bit := uint8(1); bit <= 8; bit <<= 1 {
		if covered&bit != 0 {
			continue
		}
		for i, sentence := range sentences {
			mask := virtualMemoryAspects(sentence.Text)
			if !used[i] && mask&bit != 0 {
				out = append(out, sentence)
				used[i] = true
				covered |= mask
				break
			}
		}
	}
	for i, sentence := range sentences {
		if !used[i] {
			out = append(out, sentence)
		}
	}
	return out
}
