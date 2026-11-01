package service

import "strings"

// Punctuation is a necessary conservative guard, not a proof of factual truth.
// Never promote an unterminated page/chunk tail to a finished answer sentence.
func completeAnswerText(text string) bool {
	t := strings.TrimSpace(text)
	t = strings.TrimRight(t, "\"'”’）)]}*`$ ")
	if t == "" || !strings.ContainsRune("。！？.!?；;", []rune(t)[len([]rune(t))-1]) {
		return false
	}
	core := strings.TrimRight(t, "。！？.!?；; ")
	for _, suffix := range []string{"那部分", "这一部分", "这部分", "所需的", "当前要运行的", "以便", "从而", "以及", "使得", "因此", "通过将", "的一种"} {
		if strings.HasSuffix(core, suffix) {
			return false
		}
	}
	return !danglingSentence(core)
}
