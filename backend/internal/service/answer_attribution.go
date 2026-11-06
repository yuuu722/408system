package service

import (
	"github.com/local/exam408-assistant/backend/internal/sourcequality"
	"regexp"
	"strings"
)

// These narrow guards catch known role reversals even when the same small
// generation/review model agrees with itself. They are not a general proof.
var invertedMultiple = regexp.MustCompile(`多次性[^。；！？]{0,80}(?:必须|要求|需要)[^。；！？]{0,12}一次性[^。；！？]{0,12}全部装入`)
var falseVirtualFeature = regexp.MustCompile(`虚拟(?:存储器|内存|存储)[^。；！？，]{0,12}(?:的(?:驻留性|一次性)(?:特征|特点)|(?:具有|具备|拥有)(?:了)?(?:驻留性|一次性))`)
var programResidency = regexp.MustCompile(`(?:程序|作业|应用程序)[^。；！？]{0,24}(?:具有|具备|拥有|存在)[^。；！？]{0,12}(?:一次性|驻留性)`)

func unsafeAttribution(text string) bool { return attributionIssue(text) != "" }

func attributionIssue(text string) string {
	if sourcequality.SuspectFormula(text) {
		return "suspect_formula"
	}
	for _, sentence := range strings.FieldsFunc(text, func(r rune) bool { return strings.ContainsRune("。；！？", r) }) {
		negated := false
		for _, n := range []string{"不必", "无需", "无须", "不需要", "不要求", "不再要求", "不是必须", "并非必须"} {
			negated = negated || strings.Contains(sentence, n)
		}
		if invertedMultiple.MatchString(sentence) && !negated {
			return "virtual_memory_definition"
		}
		if falseVirtualFeature.MatchString(sentence) && !strings.Contains(sentence, "不具有") && !strings.Contains(sentence, "不具备") && !strings.Contains(sentence, "不再具有") {
			return "virtual_memory_definition"
		}
		if programResidency.MatchString(sentence) && !strings.Contains(sentence, "不具有") && !strings.Contains(sentence, "不具备") {
			return "subject_mismatch"
		}
	}
	return ""
}
