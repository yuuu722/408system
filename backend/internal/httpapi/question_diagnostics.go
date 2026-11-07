package httpapi

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/service"
	"net/http"
	"regexp"
	"strings"
)

var diagnosticChoiceMark = regexp.MustCompile(`(?m)(?:^|\s)[（(]?[AＡ](?:[.．、)）:：]|[ \t]*$)`)

// A bounded diagnostic of already stored public parses, never a reparse or an
// upload. Images, credentials, private libraries and full books are not exported.
func (s *Server) questionDiagnostics(w http.ResponseWriter, r *http.Request) {
	d, e := s.Store.Document(r.Context(), uid(r), r.PathValue("id"))
	if e != nil {
		s.fail(w, e)
		return
	}
	kb, e := s.Platform.KnowledgeBase(r.Context(), uid(r), d.KnowledgeBaseID)
	if e != nil {
		s.fail(w, e)
		return
	}
	if !kb.IsPublic {
		s.fail(w, domain.ErrForbidden)
		return
	}
	parsed, e := s.Platform.QuestionStructure(r.Context(), uid(r), d.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	points, e := s.Store.ListKnowledgePoints(r.Context())
	if e != nil {
		s.fail(w, e)
		return
	}
	candidates := service.ExtractQuestions(d, parsed.Pages, points)
	entries := service.ParseDocumentAnswers(d.OriginalFilename, parsed.Pages)
	warnings := map[string]int{}
	issues := map[string]int{}
	for _, p := range parsed.Pages {
		for _, v := range p.Warnings {
			warnings[v]++
		}
	}
	for _, a := range candidates {
		if a.Payload.Issue != "" {
			issues[a.Payload.Issue]++
		}
	}
	type sample struct {
		Reason        string
		Page, Segment int
		Text          string
		TextTruncated bool
		Warnings      []string
	}
	samples := []sample{}
	seen := map[int]bool{}
	add := func(index int, reason string) {
		if index < 0 || seen[index] || len(samples) >= 4 {
			return
		}
		seen[index] = true
		p := parsed.Pages[index]
		text := []rune(p.Text)
		truncated := len(text) > 6000
		if truncated {
			text = text[:6000]
		}
		samples = append(samples, sample{reason, p.Number, p.Segment, string(text), truncated, p.Warnings})
	}
	for i, p := range parsed.Pages {
		if diagnosticChoiceMark.MatchString(p.Text) && strings.ContainsAny(p.Text, "BＢ") {
			add(i, "首次出现A/B选项的保存文本")
			break
		}
	}
	for i, p := range parsed.Pages {
		if strings.Contains(p.Text, "答案") || strings.Contains(p.Text, "解析") || strings.Contains(p.Text, "解答") {
			add(i, "首次出现答案或解析的保存文本")
			break
		}
	}
	for i, p := range parsed.Pages {
		if len(p.Warnings) > 0 {
			add(i, "解析警告样本")
			break
		}
	}
	if len(candidates) > 0 {
		for i, p := range parsed.Pages {
			if p.Number == candidates[0].Payload.Page {
				add(i, "首次抽出候选题的页面")
				break
			}
		}
	}
	if len(parsed.Pages) > 0 {
		add(len(parsed.Pages)/2, "文档中部格式样本")
	}
	for i, p := range parsed.Pages {
		if strings.TrimSpace(p.Text) != "" {
			add(i, "非空页面补充样本")
			if len(samples) >= 4 {
				break
			}
		}
	}
	grades := 0
	ordered := []service.FileAnswer{}
	for _, a := range entries {
		if a.Answer >= 0 && !a.Blocked {
			grades++
			ordered = append(ordered, a)
		}
	}
	for _, a := range entries {
		if a.Answer < 0 || a.Blocked {
			ordered = append(ordered, a)
		}
	}
	answers := []map[string]any{}
	for _, a := range ordered {
		if len(answers) >= 5 {
			break
		}
		q := []rune(service.AnswerFileText(a))
		if len(q) > 1500 {
			q = q[:1500]
		}
		answers = append(answers, map[string]any{"Number": a.Number, "Section": a.Section, "Answer": a.Answer, "Blocked": a.Blocked, "Page": a.Page, "Excerpt": string(q)})
	}
	s.reply(w, map[string]any{"DocumentID": d.ID, "Filename": d.OriginalFilename, "Pages": len(parsed.Pages), "FreshCandidates": len(candidates), "AnswerEntries": grades, "AllParsedEntries": len(entries), "BookEvidence": service.StudyBookEvidence(d.OriginalFilename, parsed.Pages), "LayoutStats": service.QuestionReadingOrderStats(parsed.Pages), "WarningCounts": warnings, "IssueCounts": issues, "Samples": samples, "AnswerSamples": answers, "Notice": "保存的解析文本样本，不等于PDF页面图片或完整文档；仅用于排查抽取格式、OCR及答案定位。"}, nil)
}
