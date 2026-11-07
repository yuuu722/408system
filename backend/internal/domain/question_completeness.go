package domain

import (
	"regexp"
	"strings"
	"unicode"
)

var workedEquation = regexp.MustCompile(`[0-9].*[=＝].*[0-9]`)
var explanationWords = regexp.MustCompile(`[A-Za-z]{2,}`)

// HasQuestionExplanation checks for substantive text, not semantic correctness.
// A compact answer list is not an explanation. Human source review is still needed.
func HasQuestionExplanation(text string) bool {
	text = strings.TrimSpace(text)
	for _, marker := range []string{"【解析】", "解析：", "解析:"} {
		if i := strings.Index(text, marker); i >= 0 {
			text = text[i+len(marker):]
			break
		}
	}
	n := 0
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			n++
		}
	}
	return n >= 8 || workedEquation.MatchString(text) || len(explanationWords.FindAllString(text, -1)) >= 8
}

// KeepSourceAnswer prevents batch pairing from replacing an already complete
// record or silently changing an existing reliable answer to a conflicting one.
func KeepSourceAnswer(a Asset, proposed int) bool {
	p := a.Payload
	reliable := p.AnswerOrigin == "source" && p.Answer >= 0 && p.Answer < len(p.Options)
	return reliable && (HasQuestionExplanation(p.Explanation) || (proposed >= 0 && proposed != p.Answer))
}
