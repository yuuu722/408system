package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"regexp"
	"strings"
)

type studyBook struct{ Publisher, Subject, Year, Role string }

var editionYear = regexp.MustCompile(`(?:^|[^0-9])(20[2-3][0-9])(?:[^0-9]|$)`)
var shortEdition = regexp.MustCompile(`(?:^|[^0-9])([2-3][0-9])(?:版|王道|竟成|竞成|考研|数据结构|操作系统|计算机)`)

func bookYear(name string) string {
	if m := editionYear.FindStringSubmatch(name); m != nil {
		return m[1]
	}
	if m := shortEdition.FindStringSubmatch(name); m != nil {
		return "20" + m[1]
	}
	return ""
}
func bookPublisher(name string) string {
	wang := strings.Contains(name, "王道")
	jing := strings.Contains(name, "竟成") || strings.Contains(name, "竞成")
	if wang == jing {
		return ""
	}
	if wang {
		return "王道"
	}
	return "竟成"
}
func bookSubject(name string) string {
	out := ""
	for code, terms := range map[string][]string{"os": {"操作系统"}, "ds": {"数据结构"}, "cn": {"计算机网络", "计网"}, "co": {"计算机组成原理", "计组"}} {
		for _, term := range terms {
			if strings.Contains(name, term) {
				if out != "" && out != code {
					return ""
				}
				out = code
				break
			}
		}
	}
	return out
}
func describeStudyBook(name string, pages []TextPage) studyBook {
	name = strings.ReplaceAll(name, " ", "")
	d := studyBook{Publisher: bookPublisher(name), Subject: bookSubject(name), Year: bookYear(name)}
	for _, bad := range []string{"强化", "模拟", "真题", "考点分类", "综合题", "应用题"} {
		if strings.Contains(name, bad) {
			return studyBook{}
		}
	}
	reference := false
	for i, page := range pages {
		if i >= 8 {
			break
		}
		for _, raw := range strings.Split(page.Text, "\n") {
			line := strings.ReplaceAll(questionLine(raw), " ", "")
			reference = reference || strings.Contains(line, "答案见原书")
			if strings.Contains(line, "王道论坛组编") {
				if d.Publisher != "" && d.Publisher != "王道" {
					return studyBook{}
				}
				d.Publisher = "王道"
			}
			if !strings.Contains(line, "考研复习指导") && !strings.Contains(line, "考研复习全书") {
				continue
			}
			subject, publisher, year := bookSubject(line), bookPublisher(line), bookYear(line)
			if subject != "" && d.Subject != "" && subject != d.Subject {
				return studyBook{}
			}
			if publisher != "" && d.Publisher != "" && publisher != d.Publisher {
				return studyBook{}
			}
			if year != "" && d.Year != "" && year != d.Year {
				return studyBook{}
			}
			if d.Subject == "" {
				d.Subject = subject
			}
			if d.Publisher == "" {
				d.Publisher = publisher
			}
			if d.Year == "" {
				d.Year = year
			}
			if subject != "" {
				d.Role = "textbook"
			}
		}
	}
	if strings.Contains(name, "选择题做题本") || strings.Contains(name, "课后习题做题本") {
		if !reference {
			return studyBook{}
		}
		d.Role = "workbook"
	}
	return d
}

// Filename-only prefilter saves loading unrelated large books; final approval
// additionally requires the title/edition evidence in the saved front matter.
func MaybeCompanionStudyBook(qname string, qpages []TextPage, aname string) bool {
	q := describeStudyBook(qname, qpages)
	if q.Role == "" || q.Publisher == "" || q.Subject == "" || q.Year == "" {
		return false
	}
	if bookSubject(aname) != q.Subject || bookYear(aname) != q.Year {
		return false
	}
	if p := bookPublisher(aname); p != "" && p != q.Publisher {
		return false
	}
	for _, bad := range []string{"做题本", "模拟", "真题", "强化", "考点分类", "综合题", "应用题"} {
		if strings.Contains(aname, bad) {
			return false
		}
	}
	return true
}

// A workbook explicitly referring to the original book may use that same
// publisher+year+subject textbook's unique topic+number answer. Subject/year
// alone is insufficient, and mock papers/other series are never admitted.
func CompanionStudyBooks(qname string, qpages []TextPage, aname string, apages []TextPage) bool {
	q, a := describeStudyBook(qname, qpages), describeStudyBook(aname, apages)
	return (q.Role == "workbook" || q.Role == "textbook") && a.Role == "textbook" && q.Publisher != "" && q.Publisher == a.Publisher && q.Subject != "" && q.Subject == a.Subject && q.Year != "" && q.Year == a.Year
}

// Read only explicit answer sections in a recognized study textbook. This
// keeps numbered theory, contents and calculation substeps out of answer keys.
func ParseDocumentAnswers(name string, pages []TextPage) []FileAnswer {
	pages = questionReadingPages(pages)
	if describeStudyBook(name, pages).Role != "textbook" {
		return ParseAnswerFile(pages)
	}
	filtered := append([]TextPage(nil), pages...)
	mode := false
	scope := ""
	found := false
	for i, p := range filtered {
		lines := strings.Split(p.Text, "\n")
		for j, raw := range lines {
			line := questionLine(raw)
			if runningQuestionHeader(line, scope) {
				continue
			}
			if heading, answer, ok := questionHeadingContext(line, scope); ok {
				scope = heading
				mode = answer
				if answer {
					found = true
				}
				continue
			}
			if answerHeading.MatchString(line) {
				mode = true
				found = true
				continue
			}
			if !mode {
				lines[j] = strings.Repeat(" ", len([]rune(raw)))
			}
		}
		filtered[i].Text = strings.Join(lines, "\n")
		// Keep original boxes as confidence evidence, but do not reconstruct excluded
		// theory from the blocks while parsing this filtered answer view.
		blocks := append([]domain.DocumentBlock(nil), p.Blocks...)
		for j := range blocks {
			blocks[j].Text = ""
		}
		filtered[i].Blocks = blocks
	}
	if !found {
		return ParseAnswerFile(pages)
	}
	return ParseAnswerFile(filtered)
}

func StudyBookEvidence(name string, pages []TextPage) map[string]string {
	d := describeStudyBook(name, pages)
	return map[string]string{"Publisher": d.Publisher, "Subject": d.Subject, "Year": d.Year, "Role": d.Role}
}
