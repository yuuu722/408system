package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"regexp"
	"strings"
)

type AnswerPassage struct {
	Page  int
	Quote string
}
type FileAnswer struct {
	Extra                  []AnswerPassage
	Number, Section, Quote string
	Answer, Page           int
	Blocked                bool
}

var compactAnswers = regexp.MustCompile(`(\d{1,4})\s*[.．、):：]\s*([A-FＡ-Ｆ])(?:[.。．、)）]|$|\s)`)
var answerParagraph = regexp.MustCompile(`^\s*[（(]?(\d{1,4})[.．、)）:：]\s*(.+)$`)
var answerLetter = regexp.MustCompile(`^(?:[（(【]?\s*(?:答案|参考答案|正确答案|解答)\s*[】）)]?\s*[:：]?\s*)?([A-FＡ-Ｆ])(?:[.。．、)）\s]|$)`)

// Parses explicit source answers, retaining page and chapter. Options and bare
// question numbers are not answers. Non-letter worked solutions stay ungraded.
func ParseAnswerFile(pages []TextPage) []FileAnswer {
	out := []FileAnswer{}
	section := ""
	var current *FileAnswer
	flush := func() {
		if current != nil {
			if current.Answer < 0 && choiceOption.MatchString(current.Quote) && !strings.Contains(current.Quote, "解析") && !strings.Contains(current.Quote, "解答") {
				current.Blocked = true
			}
			out = append(out, *current)
			current = nil
		}
	}
	for _, page := range pages {
		for _, line := range strings.Split(page.Text, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if h := questionSection.FindString(line); h != "" && len([]rune(line)) < 80 && !strings.ContainsAny(line, "。！？?") {
				flush()
				section = h
				continue
			}
			if line == "答案" || line == "参考答案" || line == "答案与解析" {
				continue
			}
			// Compact answer tables: each entry is cited separately, no explanation
			// from a neighboring answer is silently attached.
			entries := compactAnswers.FindAllStringSubmatch(line, -1)
			if len(entries) > 1 {
				flush()
				for _, e := range entries {
					out = append(out, FileAnswer{Number: e[1], Section: section, Answer: strings.Index("ABCDEF", strings.NewReplacer("Ａ", "A", "Ｂ", "B", "Ｃ", "C", "Ｄ", "D", "Ｅ", "E", "Ｆ", "F").Replace(e[2])), Page: page.Number, Quote: strings.TrimSpace(e[0]), Blocked: hasQuestionExtractionWarning(page.Warnings)})
				}
				continue
			}
			if m := answerParagraph.FindStringSubmatch(line); m != nil {
				flush()
				answer := -1
				body := strings.NewReplacer("Ａ", "A", "Ｂ", "B", "Ｃ", "C", "Ｄ", "D", "Ｅ", "E", "Ｆ", "F").Replace(m[2])
				if a := answerLetter.FindStringSubmatch(body); a != nil {
					answer = int(a[1][0] - 'A')
				}
				if answer < 0 && (len([]rune(body)) < 8 || choiceOption.MatchString(body) || strings.ContainsAny(body, "?？")) {
					continue
				}
				current = &FileAnswer{Number: m[1], Section: section, Answer: answer, Page: page.Number, Quote: line, Blocked: hasQuestionExtractionWarning(page.Warnings)}
			} else if current != nil {
				if len([]rune(AnswerFileText(*current)))+len([]rune(line)) > 2400 {
					current.Blocked = true
					continue
				}
				current.Blocked = current.Blocked || hasQuestionExtractionWarning(page.Warnings)
				if current.Page != page.Number {
					if len(current.Extra) > 0 && current.Extra[len(current.Extra)-1].Page == page.Number {
						current.Extra[len(current.Extra)-1].Quote += "\n" + line
					} else if len(current.Extra) < 3 {
						current.Extra = append(current.Extra, AnswerPassage{Page: page.Number, Quote: line})
					} else {
						current.Blocked = true
					}
				} else {
					current.Quote += "\n" + line
				}

			}
		}
	}
	flush()
	return out
}

type AnswerFileProposal struct {
	Asset  domain.Asset
	Answer FileAnswer
}

// Two explicitly chosen files define the book/paper scope. Within that scope,
// chapter+number must be unique. Explicit section mapping handles different
// printed heading conventions without silently guessing their equivalence.
func MatchAnswerFile(assets []domain.Asset, answers []FileAnswer, qsection, asection string) []AnswerFileProposal {
	qs := map[string]int{}
	as := map[string][]FileAnswer{}
	mapped := qsection != "" || asection != ""
	key := func(section, number string) string {
		if mapped {
			return number
		}
		return section + "#" + number
	}
	for _, a := range assets {
		if a.Kind != "question" || a.Payload.QuestionNumber == "" || a.Payload.ReviewStatus == "published" || (qsection != "" && a.Payload.SectionKey != qsection) {
			continue
		}
		qs[key(a.Payload.SectionKey, a.Payload.QuestionNumber)]++
	}
	for _, a := range answers {
		if a.Blocked || (asection != "" && a.Section != asection) {
			continue
		}
		k := key(a.Section, a.Number)
		as[k] = append(as[k], a)
	}
	out := []AnswerFileProposal{}
	for _, a := range assets {
		if a.Payload.QuestionNumber == "" || a.Payload.ReviewStatus == "published" || (qsection != "" && a.Payload.SectionKey != qsection) {
			continue
		}
		k := key(a.Payload.SectionKey, a.Payload.QuestionNumber)
		if qs[k] == 1 && len(as[k]) == 1 {
			out = append(out, AnswerFileProposal{a, as[k][0]})
		}
	}
	return out
}

func AnswerFileText(a FileAnswer) string {
	parts := []string{a.Quote}
	for _, p := range a.Extra {
		parts = append(parts, p.Quote)
	}
	return strings.Join(parts, "\n\n")
}
