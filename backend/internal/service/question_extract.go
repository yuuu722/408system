package service

import (
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"regexp"
	"strings"
)

var questionHead = regexp.MustCompile(`^\s*[（(]?\s*(\d{1,4})\s*[.．、)）]\s*(.*)$`)
var optionMark = regexp.MustCompile(`(?m)(?:^|\s)([A-F])(?:[.．、)）:：]\s*|[ \t]+|[ \t]*$)`)
var multipleInlineAnswer = regexp.MustCompile(`(?:[【（(]\s*)?(?:参考答案|正确答案|答案)[】）)]?\s*(?:[:：]|为|是)?\s*[（(]?\s*[A-F](?:[、,，和及与 \t]*[A-F])+(?:\s|$|[.。，；;【)）])`)
var multipleNumberedAnswer = regexp.MustCompile(`(?:^|\s)([0-9０-９]{1,4})\s*[.．、):：]\s*[A-FＡ-Ｆ](?:[、,，和及与 \t]*[A-FＡ-Ｆ])+(?:\s|$|[.。，；;【)）])`)
var inlineAnswer = regexp.MustCompile(`(?:[【（(]\s*)?(?:参考答案|正确答案|答案)[】）)]?\s*(?:[:：]|为|是)?\s*[（(]?\s*([A-F])(?:\s|$|[.。，；;【)）])`)
var questionSection = regexp.MustCompile(`^(第[一二三四五六七八九十百0-9]+[章节]|[0-9]+\.[0-9]+(?:\.[0-9]+)?)`)

var answerHeading = regexp.MustCompile(`^(?:[一二三四五六七八九十0-9]+[、.． ]*)?(?:参考)?答案(?:[与及和]?解析)?[:：]?$`)

var answerEntry = regexp.MustCompile(`(?:^|\s)(\d{1,4})\s*[.．、):：]\s*([A-F])(?:\s|$|[，,；;])`)

// Deliberately bounded to numbered single-choice questions. It never invents missing options or source answers.
func ExtractQuestions(d domain.Document, pages []TextPage, points []domain.KnowledgePoint) []domain.Asset {
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
		lineOffset := 0
		// A continuation may contain only a figure or a failed OCR region.
		if strings.TrimSpace(page.Text) == "" && !answerMode {
			attachPage(page, 0, len([]rune(page.Text)))
		}
		for _, line := range strings.Split(page.Text, "\n") {
			from, to := lineOffset, lineOffset+len([]rune(line))
			lineOffset = to + 1
			line = strings.ReplaceAll(strings.TrimSpace(line), "　", " ")
			line = strings.NewReplacer("０", "0", "１", "1", "２", "2", "３", "3", "４", "4", "５", "5", "６", "6", "７", "7", "８", "8", "９", "9", "Ａ", "A", "Ｂ", "B", "Ｃ", "C", "Ｄ", "D", "Ｅ", "E", "Ｆ", "F").Replace(line)
			if line == "" {
				continue
			}
			if heading := questionSection.FindString(line); heading != "" && len([]rune(line)) < 80 && !strings.ContainsAny(line, "。！？?") {
				flush()
				section = heading
				answerMode = strings.Contains(line, "答案") || strings.Contains(line, "解析")
				continue
			}

			if answerHeading.MatchString(line) {
				flush()
				answerMode = true
				continue
			}
			if answerMode {
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
					answers[key(section, m[1])] = append(answers[key(section, m[1])], int(m[2][0]-'A'))
					answerPages[key(section, m[1])] = page.Number
				}
				continue
			}
			if m := questionHead.FindStringSubmatch(line); m != nil {
				flush()
				current = &candidate{number: m[1], body: m[2], page: page.Number, section: section}
			} else if current != nil {
				current.body += "\n" + line
			}
			attachPage(page, from, to)
		}
	}
	flush()
	numberCount := map[string]int{}
	for _, c := range candidates {
		numberCount[key(c.section, c.number)]++
	}
	out := []domain.Asset{}
	seenContent := map[string]bool{}
	for _, c := range candidates {
		marks := optionMark.FindAllStringSubmatchIndex(c.body, -1)
		// Numbered prose in textbooks is not a question; retain suspicious question-like incomplete items only.
		if len(marks) == 0 {
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
			marks = optionMark.FindAllStringSubmatchIndex(body, -1)
		} else if numberCount[key(c.section, c.number)] == 1 && len(answers[key(c.section, c.number)]) == 1 {
			a.Payload.Answer = answers[key(c.section, c.number)][0]
			a.Payload.AnswerOrigin = "source"
			a.Payload.Source += fmt.Sprintf("；答案第%d页", answerPages[key(c.section, c.number)])
		}
		if len(marks) > 0 {
			a.Payload.Stem = strings.TrimSpace(body[:marks[0][0]])
			for j, m := range marks {
				end := len(body)
				if j+1 < len(marks) {
					end = marks[j+1][0]
				}
				a.Payload.Options = append(a.Payload.Options, strings.TrimSpace(body[m[1]:end]))
				if body[m[2]] != byte('A'+j) {
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
	return out
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
