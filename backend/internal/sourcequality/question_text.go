package sourcequality

import (
	"regexp"
	"strings"
	"unicode"
)

var questionLatinToken = regexp.MustCompile(`[A-Za-z]{2,}`)

// A narrow corruption signal, not English spellchecking. Keep real protocol
// names, code and formula variables; never guess the missing Chinese words.
func CorruptQuestionText(text string) bool {
	if strings.ContainsAny(text, "�\x00") {
		return true
	}
	for _, line := range strings.Split(text, "\n") {
		han := 0
		for _, r := range line {
			if unicode.Is(unicode.Han, r) {
				han++
			}
		}
		if han < 4 {
			continue
		}
		short, capitals, longCaps := 0, 0, 0
		for _, token := range questionLatinToken.FindAllString(line, -1) {
			if knownLatin[strings.ToLower(token)] {
				continue
			}
			if token == strings.ToUpper(token) {
				capitals++
				if len(token) >= 8 {
					longCaps++
				}
			}
			if len(token) >= 2 && len(token) <= 3 {
				short++
			}
		}
		if (longCaps > 0 && capitals >= 2 && !strings.Contains(line, "_")) || (short >= 4 && !strings.ContainsAny(line, "={}();")) {
			return true
		}
	}
	return false
}
