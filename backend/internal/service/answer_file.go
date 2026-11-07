package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/sourcequality"
	"regexp"
	"strconv"
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

var compactAnswers = regexp.MustCompile(`([0-9０-９]{1,4})(?:\s*[.．、):：]\s*|\s+)([A-FＡ-Ｆ])(?:[.。．、)）]|$|\s)`)
var answerParagraph = regexp.MustCompile(`^\s*[（(]?\s*([0-9０-９]{1,4})(?:\s*[.．、)）:：]\s*|\s+)(.+)$`)
var tableAnswerNumbers = regexp.MustCompile(`[0-9０-９]{1,4}`)
var tableAnswerLetters = regexp.MustCompile(`[A-FＡ-Ｆ]`)

var answerLetter = regexp.MustCompile(`^(?:[（(【]?\s*(?:答案|人参考答案|参考答案|正确答案|解答)\s*[】）)]?\s*(?:[:：]|为|是)?\s*)?[（(]?([A-FＡ-Ｆ])(?:[.。．、)）【\s]|$)`)

// Parses explicit source answers, retaining page and chapter. Options and bare
// question numbers are not answers. Non-letter worked solutions stay ungraded.
func ParseAnswerFile(pages []TextPage) []FileAnswer {
	pages = questionReadingPages(pages)
	out := []FileAnswer{}
	section := ""
	var current *FileAnswer
	flush := func() {
		if current != nil {
			if current.Answer < 0 && !multipleInlineAnswer.MatchString(AnswerFileText(*current)) {
				if m := inlineAnswer.FindStringSubmatch(AnswerFileText(*current)); m != nil {
					current.Answer = int(m[1][0] - 'A')
				}
			}
			if current.Answer < 0 && choiceOption.MatchString(current.Quote) && !strings.Contains(current.Quote, "解析") && !strings.Contains(current.Quote, "解答") {
				current.Blocked = true
			}
			if sourcequality.SuspectFormula(AnswerFileText(*current)) {
				current.Blocked = true
			}
			out = append(out, *current)
			current = nil
		}
	}
	appendLine := func(page TextPage, line string, from, to int) {
		if current == nil {
			return
		}
		if len([]rune(AnswerFileText(*current)))+len([]rune(line)) > 2400 {
			current.Blocked = true
			return
		}
		current.Blocked = current.Blocked || questionSpanWarning(page, from, to)
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
	for pageIndex, page := range pages {
		tableHead := ""
		tableNumbers := []string{}
		tableStart := 0
		lineOffset := 0
		pageLines := strings.Split(page.Text, "\n")
		probeLines := append([]string(nil), pageLines...)
		if pageIndex+1 < len(pages) {
			probeLines = append(probeLines, strings.Split(pages[pageIndex+1].Text, "\n")...)
		}
		for lineIndex, line := range pageLines {
			from, to := lineOffset, lineOffset+len([]rune(line))
			lineOffset = to + 1
			originalLine := line
			line = strings.TrimSpace(line)
			if line == "" || questionBoilerplate(line) || runningQuestionHeader(line, section) {
				continue
			}
			if tableHead != "" {
				letters := tableAnswerLetters.FindAllString(line, -1)
				if strings.HasPrefix(strings.Trim(line, "| "), "答案") && len(letters) == len(tableNumbers) {
					flush()
					for i, n := range tableNumbers {
						letter := strings.NewReplacer("Ａ", "A", "Ｂ", "B", "Ｃ", "C", "Ｄ", "D", "Ｅ", "E", "Ｆ", "F").Replace(letters[i])
						out = append(out, FileAnswer{Number: canonicalAnswerNumber(n), Section: section, Answer: int(letter[0] - 'A'), Page: page.Number, Quote: tableHead + "\n" + originalLine, Blocked: questionSpanWarning(page, tableStart, to)})
					}
					tableHead = ""
					tableNumbers = nil
					continue
				}
				tableHead = ""
				tableNumbers = nil
			}
			if strings.HasPrefix(strings.Trim(line, "| "), "题号") {
				flush()
				tableNumbers = tableAnswerNumbers.FindAllString(line, -1)
				if len(tableNumbers) > 0 {
					tableHead = originalLine
					tableStart = from
				}
				continue
			}
			if h, _, ok := questionHeadingContext(line, section); ok {
				flush()
				section = h
				continue
			}

			if answerHeading.MatchString(line) {
				flush()
				continue
			}
			if multi := multipleNumberedAnswer.FindAllStringSubmatch(line, -1); len(multi) > 0 {
				flush()
				for _, m := range multi {
					out = append(out, FileAnswer{Number: canonicalAnswerNumber(m[1]), Section: section, Answer: -1, Page: page.Number, Quote: line, Blocked: true})
				}
				continue
			}
			// Compact answer tables: each entry is cited separately, no explanation
			// from a neighboring answer is silently attached.
			entries := compactAnswers.FindAllStringSubmatch(line, -1)
			residue := compactAnswers.ReplaceAllString(line, "")
			residue = strings.NewReplacer("参考答案", "", "答案", "").Replace(residue)
			compactOnly := strings.Trim(residue, " \t|,，;；:：[]【】()（）.。") == ""
			if len(entries) > 1 && compactOnly {
				flush()
				for _, e := range entries {
					out = append(out, FileAnswer{Number: canonicalAnswerNumber(e[1]), Section: section, Answer: strings.Index("ABCDEF", strings.NewReplacer("Ａ", "A", "Ｂ", "B", "Ｃ", "C", "Ｄ", "D", "Ｅ", "E", "Ｆ", "F").Replace(e[2])), Page: page.Number, Quote: strings.TrimSpace(e[0]), Blocked: questionSpanWarning(page, from, to)})
				}
				continue
			}
			if m := answerParagraph.FindStringSubmatch(line); m != nil {
				answer := -1
				body := strings.NewReplacer("Ａ", "A", "Ｂ", "B", "Ｃ", "C", "Ｄ", "D", "Ｅ", "E", "Ｆ", "F").Replace(m[2])
				if multipleInlineAnswer.MatchString(body) {
					flush()
					current = &FileAnswer{Number: canonicalAnswerNumber(m[1]), Section: section, Answer: -1, Page: page.Number, Quote: line, Blocked: true}
					continue
				}
				if a := answerLetter.FindStringSubmatch(body); a != nil {
					answer = int(a[1][0] - 'A')
				}
				// Numbered calculation rows/substeps belong to the current
				// explanation, not to new answer keys for questions 1,2,3...
				labelled := hasWorkedAnswerLabel(body)
				if current != nil && answer >= 0 && !labelled && strings.ContainsAny(body, "+*/|#=") && len(strings.Fields(body)) >= 3 {
					han := 0
					for _, r := range body {
						if r >= '\u4e00' && r <= '\u9fff' {
							han++
						}
					}
					if han < 4 {
						appendLine(page, line, from, to)
						continue
					}
				}
				question := questionStemCue(body)
				if !question {
					probe := body
					for j := lineIndex + 1; j < len(probeLines) && j < lineIndex+20; j++ {
						next := questionLine(probeLines[j])
						if answerParagraph.MatchString(next) || isolatedQuestionNumber.MatchString(next) || inlineAnswer.MatchString(next) || hasWorkedAnswerLabel(next) {
							break
						}
						probe += "\n" + next
					}
					question = len(questionOptionMarks(questionLine(probe))) >= 2
				}
				if answer < 0 && !labelled && !question {
					appendLine(page, line, from, to)
					continue
				}
				flush()
				if answer < 0 && ((len([]rune(body)) < 8 && !hasWorkedAnswerLabel(body)) || (choiceOption.MatchString(body) && !inlineAnswer.MatchString(body))) {
					continue
				}
				current = &FileAnswer{Number: canonicalAnswerNumber(m[1]), Section: section, Answer: answer, Page: page.Number, Quote: line, Blocked: questionSpanWarning(page, from, to)}
			} else if current != nil {
				appendLine(page, line, from, to)
			}
		}
	}
	flush()
	return consolidateAnswerEntries(out)
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
	blocked := map[string]bool{}
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
		if a.Blocked {
			blocked[key(a.Section, a.Number)] = true
			continue
		}
		if asection != "" && a.Section != asection {
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
		if !blocked[k] && qs[k] == 1 && len(as[k]) == 1 {
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

func canonicalAnswerNumber(number string) string {
	number = fullwidthQuestionText.Replace(number)
	if n, e := strconv.Atoi(number); e == nil && n > 0 {
		return strconv.Itoa(n)
	}
	return number
}
