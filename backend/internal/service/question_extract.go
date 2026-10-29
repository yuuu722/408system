package service

import (
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"regexp"
	"strings"
)

var questionHead = regexp.MustCompile(`^\s*(\d{1,4})[.．、)]\s*(.+)$`)
var optionMark = regexp.MustCompile(`(?:^|\s)([A-F])[.．、)）]\s*`)
var inlineAnswer = regexp.MustCompile(`(?:参考答案|正确答案|答案)\s*[:：]?\s*([A-F])(?:\s|$|[.。，；;])`)
var answerEntry = regexp.MustCompile(`(?:^|\s)(\d{1,4})\s*[.．、):：]\s*([A-F])(?:\s|$|[，,；;])`)

// Deliberately bounded to numbered single-choice questions. It never invents missing options or source answers.
func ExtractQuestions(d domain.Document, pages []TextPage, points []domain.KnowledgePoint) []domain.Asset {
	type candidate struct {
		number, body string
		page         int
		images       []string
		issue        string
	}
	var candidates []candidate
	var current *candidate
	answerMode := false
	answers := map[string][]int{}
	answerPages := map[string]int{}
	flush := func() {
		if current != nil {
			candidates = append(candidates, *current)
			current = nil
		}
	}
	attachPage := func(page TextPage) {
		if current == nil {
			return
		}
		if hasQuestionExtractionWarning(page.Warnings) {
			current.issue = "页面存在解析异常，请查看原文（Incomplete extraction）"
		}
		for _, b := range page.Blocks {
			if b.Image == "" {
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
		// A continuation may contain only a figure or a failed OCR region.
		if strings.TrimSpace(page.Text) == "" && !answerMode {
			attachPage(page)
		}
		for _, line := range strings.Split(page.Text, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if line == "参考答案" || line == "答案与解析" || line == "答案" {
				flush()
				answerMode = true
				continue
			}
			if answerMode {
				// A partial/failed answer page must not become a source of automatic grades.
				if hasQuestionExtractionWarning(page.Warnings) {
					continue
				}
				for _, m := range answerEntry.FindAllStringSubmatch(line, -1) {
					answers[m[1]] = append(answers[m[1]], int(m[2][0]-'A'))
					answerPages[m[1]] = page.Number
				}
				continue
			}
			if m := questionHead.FindStringSubmatch(line); m != nil {
				flush()
				current = &candidate{number: m[1], body: m[2], page: page.Number}
			} else if current != nil {
				current.body += "\n" + line
			}
			attachPage(page)
		}
	}
	flush()
	numberCount := map[string]int{}
	for _, c := range candidates {
		numberCount[c.number]++
	}
	out := []domain.Asset{}
	seenContent := map[string]bool{}
	for _, c := range candidates {
		marks := optionMark.FindAllStringSubmatchIndex(c.body, -1)
		// Numbered prose in textbooks is not a question; retain suspicious question-like incomplete items only.
		if len(marks) == 0 && !strings.ContainsAny(c.body, "?？") && !strings.Contains(c.body, "下列") {
			continue
		}
		a := domain.Asset{Kind: "question", Title: fmt.Sprintf("%s · 第%s题", d.OriginalFilename, c.number), Payload: domain.AssetPayload{Answer: -1, AnswerOrigin: "ai_pending", Difficulty: 1, DocumentID: d.ID, Page: c.page, Source: fmt.Sprintf("%s，第%d页，第%s题", d.OriginalFilename, c.page, c.number), Images: c.images, Issue: c.issue, QuestionOrigin: "extracted"}}
		body := c.body
		if m := inlineAnswer.FindStringSubmatchIndex(body); m != nil {
			a.Payload.Answer = int(body[m[2]] - 'A')
			a.Payload.AnswerOrigin = "source"
			a.Payload.Explanation = strings.TrimSpace(body[m[1]:])
			body = body[:m[0]]
			marks = optionMark.FindAllStringSubmatchIndex(body, -1)
		} else if numberCount[c.number] == 1 && len(answers[c.number]) == 1 {
			a.Payload.Answer = answers[c.number][0]
			a.Payload.AnswerOrigin = "source"
			a.Payload.Source += fmt.Sprintf("；答案第%d页", answerPages[c.number])
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
		if strings.HasPrefix(warning, "Word转换页码：来自服务端排版，不保证与Word/WPS原始页码一致；请下载原文件核对。") {
			continue
		}
		if strings.TrimSpace(warning) != "" {
			return true
		}
	}
	return false
}
