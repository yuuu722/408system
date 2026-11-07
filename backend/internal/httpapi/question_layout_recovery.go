package httpapi

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/service"
	"net/http"
	"strings"
)

func (s *Server) repairQuestionLayout(w http.ResponseWriter, r *http.Request) {
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
	if !kb.IsPublic || d.Status != "ready" {
		s.fail(w, domain.ErrForbidden)
		return
	}
	if d.MimeType != "application/pdf" && !strings.HasSuffix(strings.ToLower(d.OriginalFilename), ".pdf") {
		s.reply(w, map[string]any{"Attempted": 0, "Remaining": 0, "Skipped": "非PDF，不改变现有解析"}, nil)
		return
	}
	base, e := s.Platform.DocumentStructure(r.Context(), uid(r), d.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	view, e := s.Platform.QuestionView(r.Context(), uid(r), d, base)
	if e != nil {
		s.fail(w, e)
		return
	}
	attempted := map[int]bool{}
	for _, n := range view.Attempted {
		attempted[n] = true
	}
	todo := []int{}
	for _, n := range service.MissingQuestionLayoutPages(base.Pages) {
		if !attempted[n] {
			todo = append(todo, n)
		}
	}
	if len(todo) == 0 {
		s.reply(w, map[string]any{"Attempted": 0, "Remaining": 0, "CachedPages": len(view.Pages), "CachedFailed": view.Failed}, nil)
		return
	}
	path, hash, e := service.StoredQuestionFile(s.Documents.StorageRoot, d.StoragePath)
	if e != nil {
		s.fail(w, &domain.RequestError{Status: 409, Message: e.Error()})
		return
	}
	if view.SourceHash != "" && view.SourceHash != hash {
		s.fail(w, &domain.RequestError{Status: 409, Message: "原文件已变化，停止版面修复"})
		return
	}
	view.SourceHash = hash
	limit := len(todo)
	if limit > 2 {
		limit = 2
	}
	failed := []map[string]any{}
	repaired := 0
	for _, n := range todo[:limit] {
		p, err := service.RecoverQuestionPageLayout(r.Context(), path, n)
		view.Attempted = append(view.Attempted, n)
		if err != nil {
			failure := map[string]any{"Page": n, "Reason": err.Error()}
			failed = append(failed, failure)
			view.Failed = append(view.Failed, failure)
			continue
		}
		for _, original := range base.Pages {
			if original.Number == n {
				p.Segment = original.Segment
				break
			}
		}
		view.Pages = append(view.Pages, p)
		repaired++
	}
	if e = s.Platform.SaveQuestionView(r.Context(), uid(r), d, base, view); e != nil {
		s.fail(w, e)
		return
	}
	s.reply(w, map[string]any{"Attempted": limit, "Repaired": repaired, "Remaining": len(todo) - limit, "Failed": failed, "Method": "original_pdf_bbox_or_page_local_ocr"}, nil)
}
