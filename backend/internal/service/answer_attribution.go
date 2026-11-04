package service

import (
	"github.com/local/exam408-assistant/backend/internal/sourcequality"
	"regexp"
	"strings"
)

// These narrow guards catch known role reversals even when the same small
// generation/review model agrees with itself. They are not a general proof.
var programResidency = regexp.MustCompile(`(?:程序|作业|应用程序)[^。；！？]{0,24}(?:具有|具备|拥有|存在)[^。；！？]{0,12}(?:一次性|驻留性)`)

func unsafeAttribution(text string) bool {
	if sourcequality.SuspectFormula(text) {
		return true
	}
	for _, sentence := range strings.FieldsFunc(text, func(r rune) bool { return strings.ContainsRune("。；！？", r) }) {
		if programResidency.MatchString(sentence) && !strings.Contains(sentence, "不具有") && !strings.Contains(sentence, "不具备") {
			return true
		}
	}
	return false
}
