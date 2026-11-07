package service

import (
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/sourcequality"
	"regexp"
	"strings"
)

var questionHead = regexp.MustCompile(`^\s*[（(]?\s*(\d{1,4})\s*[.．、)）]\s*(.*)$`)
var optionMark = regexp.MustCompile(`(?m)(?:^|\s)[（(]?([Aa][Aa]?|[Bb][Bb]?|[Cc][Cc]?|[Dd][Dd]?|[Ee][Ee]?|[Ff][Ff]?)(?:[.．、，,)）:：]\s*|[ \t]+|[ \t]*$)`)
var multipleInlineAnswer = regexp.MustCompile(`(?:[【（(]\s*)?(?:人参考答案|参考答案|正确答案|答案)[】）)]?\s*(?:[:：]|为|是)?\s*[（(]?\s*[A-F](?:[、,，和及与 \t]*[A-F])+(?:\s|$|[.。，；;【)）])`)
var multipleNumberedAnswer = regexp.MustCompile(`(?:^|\s)([0-9０-９]{1,4})\s*[.．、):：]\s*[A-FＡ-Ｆ](?:[、,，和及与 \t]*[A-FＡ-Ｆ])+(?:\s|$|[.。，；;【)）])`)
var inlineAnswer = regexp.MustCompile(`(?:[【（(]\s*)?(?:人参考答案|参考答案|正确答案|答案)[】）)]?\s*(?:[:：]|为|是)?\s*[（(]?\s*([A-F])(?:\s|$|[.。，；;【)）])`)
var questionSection = regexp.MustCompile(`^(第[一二三四五六七八九十百0-9]+[章节]|[0-9]+\.[0-9]+(?:\.[0-9]+)?)`)

var answerHeading = regexp.MustCompile(`^(?:[一二三四五六七八九十0-9]+[、.． ]*)?(?:答案与参考要点|答案与参考解析|单项选择题参考答案|(?:参考)?答案(?:[与及和]?解析)?|习题(?:参考)?解答|习题答案与解析|试题解析|答案详解)[:：]?$`)

var answerEntry = regexp.MustCompile(`(?:^|\s)(\d{1,4})\s*[.．、):：]\s*([A-F])(?:\s|$|[，,；;])`)

// Deliberately bounded to numbered single-choice questions. It never invents missing options or source answers.
func ExtractQuestions(d domain.Document, pages []TextPage, points []domain.KnowledgePoint) []domain.Asset {
	pages = questionReadingPages(pages)
	type candidate struct {
		number, body, section string
		page                  int
		images                []string
		issue                 string
	}
	var candidates []candidate
	var current *candidate
	answerMode := false
	section := ""
	key := func(scope, number string) string { return scope + "#" + number }
	answers := map[string][]int{}
	answerPages := map[string]int{}
	answerTextPages := []TextPage{}
	flush := func() {
		if current != nil {
			candidates = append(candidates, *current)
			current = nil
		}
	}
	attachPage := func(page TextPage, from, to int) {
		if current == nil {
			return
		}
		if questionSpanWarning(page, from, to) {
			current.issue = "页面存在解析异常，请查看原文（Incomplete extraction）"
		}
		for _, b := range page.Blocks {
			if b.Image == "" {
				continue
			}
			if b.End > b.Start {
				if b.Start >= to || b.End <= from {
					continue
				}
			} else if b.Start < from || b.Start > to {
				continue
			}
			exists := false
			for _, img := range current.images {
				if img == b.Image {
					exists = true
					break
				}
			}
			if !exists {
				current.images = append(current.images, b.Image)
			}
		}
	}
	for _, page := range pages {
		filtered := page
		filteredLines := []string{}
		lineOffset := 0
		// A continuation may contain only a figure or a failed OCR region.
		if strings.TrimSpace(page.Text) == "" && !answerMode {
			attachPage(page, 0, len([]rune(page.Text)))
		}
		for _, line := range strings.Split(page.Text, "\n") {
			originalLine := line
			filteredLines = append(filteredLines, strings.Repeat(" ", len([]rune(line))))
			setAnswerLine := func() { filteredLines[len(filteredLines)-1] = originalLine }
			from, to := lineOffset, lineOffset+len([]rune(line))
			lineOffset = to + 1
			line = questionLine(line)

			if line == "" || questionBoilerplate(line) || runningQuestionHeader(line, section) {
				continue
			}
			if heading, isAnswer, ok := questionHeadingContext(line, section); ok {
				flush()
				section = heading
				setAnswerLine()
				answerMode = isAnswer
				continue
			}

			if answerHeading.MatchString(line) {
				flush()
				answerMode = true
				setAnswerLine()
				continue
			}
			if answerMode {
				setAnswerLine()
				// A partial/failed answer page must not become a source of automatic grades.
				if hasQuestionExtractionWarning(page.Warnings) {
					continue
				}
				if multi := multipleNumberedAnswer.FindAllStringSubmatch(line, -1); len(multi) > 0 {
					for _, m := range multi {
						answers[key(section, canonicalAnswerNumber(m[1]))] = append(answers[key(section, canonicalAnswerNumber(m[1]))], -1, -1)
					}
					continue
				}
				for _, m := range answerEntry.FindAllStringSubmatch(line, -1) {
					answers[key(section, canonicalAnswerNumber(m[1]))] = append(answers[key(section, canonicalAnswerNumber(m[1]))], int(m[2][0]-'A'))
					answerPages[key(section, canonicalAnswerNumber(m[1]))] = page.Number
				}
				continue
			}
			if m := questionHead.FindStringSubmatch(line); m != nil {
				flush()
				current = &candidate{number: canonicalAnswerNumber(m[1]), body: m[2], page: page.Number, section: section}
			} else if current != nil {
				current.body += "\n" + line
			}
			attachPage(page, from, to)
		}
		filtered.Text = strings.Join(filteredLines, "\n")
		answerTextPages = append(answerTextPages, filtered)
	}
	flush()
	numberCount := map[string]int{}
	for _, c := range candidates {
		numberCount[key(c.section, c.number)]++
	}
	out := []domain.Asset{}
	seenContent := map[string]bool{}
	for _, c := range candidates {
		marks := questionAllOptionMarks(c.body)
		// Numbered prose alone is not a choice question; validate the complete option sequence below.
		if len(marks) < 2 {
			continue
		}
		a := domain.Asset{Kind: "question", Title: fmt.Sprintf("%s · 第%s题", d.OriginalFilename, c.number), Payload: domain.AssetPayload{QuestionNumber: c.number, SectionKey: c.section, Answer: -1, AnswerOrigin: "ai_pending", Difficulty: 1, DocumentID: d.ID, Page: c.page, Source: fmt.Sprintf("%s，第%d页，第%s题", d.OriginalFilename, c.page, c.number), Images: c.images, Issue: c.issue, QuestionOrigin: "extracted"}}
		body := c.body
		if multipleInlineAnswer.MatchString(body) {
			a.Payload.Issue = "原文含多个答案，当前仅支持单选计分"
		} else if m := inlineAnswer.FindStringSubmatchIndex(body); m != nil {
			a.Payload.Answer = int(body[m[2]] - 'A')
			a.Payload.AnswerOrigin = "source"
			a.Payload.Explanation = strings.TrimSpace(body[m[3]:])
			body = body[:m[0]]
			marks = questionOptionMarks(body)
		} else if numberCount[key(c.section, c.number)] == 1 && len(answers[key(c.section, c.number)]) == 1 {
			a.Payload.Answer = answers[key(c.section, c.number)][0]
			a.Payload.AnswerOrigin = "source"
			a.Payload.Source += fmt.Sprintf("；答案第%d页", answerPages[key(c.section, c.number)])
		}
		marks = questionOptionMarks(body)
		if len(marks) > 0 {
			a.Payload.Stem = strings.TrimSpace(body[:marks[0][0]])
			for j, m := range marks {
				end := len(body)
				if j+1 < len(marks) {
					end = marks[j+1][0]
				}
				a.Payload.Options = append(a.Payload.Options, strings.TrimSpace(body[m[1]:end]))
				if optionLetter(body, m) != byte('A'+j) {
					a.Payload.Issue = "选项序号不连续（Incomplete options）"
				}
			}
		} else {
			a.Payload.Stem = body
		}
		if (strings.Contains(body, "如图") || strings.Contains(body, "下图") || strings.Contains(body, "图示")) && len(c.images) == 0 {
			a.Payload.Issue = "题目引用的图片未提取，请查看原文（Missing figure）"
		}
		best := 0
		for _, p := range points {
			score := 0
			for _, key := range append(p.Keywords, p.Name) {
				if key != "" && strings.Contains(strings.ToLower(body), strings.ToLower(key)) {
					score++
				}
			}
			if score > best {
				best = score
				a.PointID = p.ID
			}
		}
		if sourcequality.SuspectFormula(body) {
			a.Payload.Issue = "公式或字符疑似OCR错误，保留原页，不自动猜改"
		}
		// Only actual choice questions become assets; numbered prose, variable
		// names and fragments with empty stems/options are not quiz candidates.
		if strings.TrimSpace(a.Payload.Stem) == "" || len(a.Payload.Options) < 2 {
			continue
		}
		empty := false
		for _, option := range a.Payload.Options {
			if strings.TrimSpace(option) == "" {
				empty = true
			}
		}
		if empty {
			continue
		}
		domain.PrepareAsset(&a)
		a.Payload.DifficultyOrigin = "unknown"
		identity := digest(a.Payload.Stem + "\n" + strings.Join(a.Payload.Options, "\n") + "\n" + strings.Join(a.Payload.Images, "\n"))
		if seenContent[identity] {
			continue
		}
		seenContent[identity] = true
		a.Payload.ExtractionKey = identity
		a.Payload.FamilyID = d.ID + ":" + identity
		out = append(out, a)
	}
	// Answers at the back of the same book carry their worked explanation,
	// not just a grade letter. Repeated chapter+number identities remain blocked.
	entries := ParseAnswerFile(answerTextPages)
	for _, pair := range MatchAnswerFile(out, entries, "", "") {
		for i := range out {
			a := &out[i]
			if a.Payload.ExtractionKey != pair.Asset.Payload.ExtractionKey {
				continue
			}
			if pair.Answer.Answer < 0 || pair.Answer.Answer >= len(a.Payload.Options) {
				continue
			}
			if a.Payload.AnswerOrigin == "source" && a.Payload.Answer != pair.Answer.Answer {
				a.Payload.Issue = "同文件答案冲突"
				a.Status = "unavailable"
				continue
			}
			a.Payload.Answer = pair.Answer.Answer
			a.Payload.AnswerOrigin = "source"
			a.Payload.Explanation = AnswerFileText(pair.Answer)
			if a.PointID == "" {
				a.PointID = MatchQuestionPoint(a.Payload.Stem+"\n"+a.Payload.Explanation, points)
				if a.PointID != "" && strings.HasPrefix(a.Payload.Issue, "未匹配知识点") {
					a.Payload.Issue = ""
				}
			}
			a.Payload.Source += fmt.Sprintf("；同文件解析第%d页", pair.Answer.Page)
			a.Payload.Evidence = []domain.Citation{{DocumentID: d.ID, DocumentTitle: d.OriginalFilename, RevisionID: d.ActiveRevisionID, Page: pair.Answer.Page, Excerpt: pair.Answer.Quote, LocationQuality: "page"}}
			for _, extra := range pair.Answer.Extra {
				a.Payload.Evidence = append(a.Payload.Evidence, domain.Citation{DocumentID: d.ID, DocumentTitle: d.OriginalFilename, RevisionID: d.ActiveRevisionID, Page: extra.Page, Excerpt: extra.Quote, LocationQuality: "page"})
			}
			domain.PrepareAsset(a)
		}
	}
	return out
}

// Pick a contiguous A,B,... option sequence. An A variable in the question
// stem or a lone B in numbered textbook prose is not an option list.
func questionOptionMarks(body string) [][]int {
	all := questionAllOptionMarks(body)
	for i, m := range all {
		if optionLetter(body, m) != 'A' || strings.TrimSpace(body[:m[0]]) == "" {
			continue
		}
		// Bare inline letters need an actual question cue. In prose, "A 与 B"
		// denotes variables; line-start labels and punctuated A. remain valid.
		prefix := body[:m[2]]
		lineStart := strings.TrimSpace(prefix[strings.LastIndex(prefix, "\n")+1:]) == ""
		punctuated := strings.ContainsAny(body[m[3]:m[1]], ".．、，,)）:：")
		cue := strings.ContainsAny(body[:m[0]], "?？")
		for _, term := range []string{"下列", "正确", "错误", "选择", "以下", "( )", "（ ）", "()", "（）"} {
			cue = cue || strings.Contains(body[:m[0]], term)
		}
		if !lineStart && !punctuated && !cue {
			continue
		}

		selected := [][]int{m}
		for _, n := range all[i+1:] {
			if optionLetter(body, n) != byte('A'+len(selected)) {
				break
			}
			selected = append(selected, n)
		}
		if len(selected) >= 2 && len(selected) == len(all)-i {
			return selected
		}
	}
	return nil
}

// Pagination notices describe Word rendering, not lost question content.
// Unknown warnings remain blocking; do not silently waive OCR/parser failures.
func hasQuestionExtractionWarning(warnings []string) bool {
	for _, warning := range warnings {
		if warning == "此页使用本地OCR识别，请核对文字和公式。" {
			continue
		}
		if strings.HasPrefix(warning, "Word转换页码：来自服务端排版，不保证与Word/WPS原始页码一致；请下载原文件核对。") {
			continue
		}
		if strings.TrimSpace(warning) != "" {
			return true
		}
	}
	return false
}

// CanUseQuestionPage permits pagination notices, never lost-content warnings.
func CanUseQuestionPage(page TextPage) bool { return !hasQuestionExtractionWarning(page.Warnings) }

// Recognition warnings with block coordinates apply only to overlapping
// question lines. Unknown/structural page failures still block the whole page.
func questionSpanWarning(page TextPage, from, to int) bool {
	located := false
	for _, b := range page.Blocks {
		if b.Confidence != nil && b.End > b.Start {
			located = true
			break
		}
	}
	warnings := []string{}
	for _, w := range page.Warnings {
		if located && strings.HasPrefix(w, "第") && strings.Contains(w, "行OCR置信度较低") {
			continue
		}
		warnings = append(warnings, w)
	}
	if hasQuestionExtractionWarning(warnings) {
		return true
	}
	for _, b := range page.Blocks {
		if b.Confidence != nil && *b.Confidence < .8 && b.Start < to && b.End > from {
			return true
		}
	}
	return false
}

func optionLetter(body string, m []int) byte {
	c := body[m[2]]
	if c >= 'a' && c <= 'f' {
		c = c - 'a' + 'A'
	}
	return c
}
func questionAllOptionMarks(body string) [][]int {
	out := [][]int{}
	for _, m := range optionMark.FindAllStringSubmatchIndex(body, -1) {
		c := body[m[2]]
		if c >= 'a' && c <= 'f' && !strings.ContainsAny(body[m[3]:m[1]], ".．、，,)）:：") {
			continue
		}
		out = append(out, m)
	}
	return out
}
