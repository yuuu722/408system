package httpapi

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/service"
	"net/http"
)

func (s *Server) catalogRoutes(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/exam408/assets/{id}/link-solution", s.auth(http.HandlerFunc(s.linkSolution)))
	mux.Handle("POST /api/v1/knowledge-bases/{id}/catalog/batch", s.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			DocumentIDs                        []string
			ResourceType, SubjectCode, Purpose string
		}
		if !s.input(w, r, &in) {
			return
		}
		if len(in.DocumentIDs) == 0 || len(in.DocumentIDs) > 200 {
			s.fail(w, &domain.RequestError{Status: 400, Message: "每批选择1至200份资料"})
			return
		}
		kb, e := s.Store.KnowledgeBase(r.Context(), uid(r), r.PathValue("id"))
		if e != nil {
			s.fail(w, e)
			return
		}
		actor, e := s.Store.UserByID(r.Context(), uid(r))
		if e != nil {
			s.fail(w, e)
			return
		}
		if kb.IsPublic && actor.Role != "admin" {
			s.fail(w, domain.ErrForbidden)
			return
		}
		done := 0
		failures := map[string]string{}
		for _, id := range in.DocumentIDs {
			d, e := s.Store.Document(r.Context(), uid(r), id)
			if e != nil || d.KnowledgeBaseID != kb.ID {
				failures[id] = "无权操作此资料"
				continue
			}
			c := domain.DocumentCatalog{DocumentID: id, ResourceType: in.ResourceType, SubjectCode: in.SubjectCode, Purpose: in.Purpose, ReviewState: "confirmed", Reason: "维护者批量核对"}
			if e = s.Platform.SaveCatalog(r.Context(), uid(r), c); e != nil {
				failures[id] = e.Error()
			} else {
				done++
			}
		}
		s.reply(w, map[string]any{"updated": done, "failed": failures}, nil)
	})))
	mux.Handle("POST /api/v1/knowledge-bases/{id}/rebuild-index", s.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kid := r.PathValue("id")
		kb, e := s.Store.KnowledgeBase(r.Context(), uid(r), kid)
		if e != nil {
			s.fail(w, e)
			return
		}
		actor, e := s.Store.UserByID(r.Context(), uid(r))
		if e != nil {
			s.fail(w, e)
			return
		}
		if kb.IsPublic && actor.Role != "admin" {
			s.fail(w, domain.ErrForbidden)
			return
		}
		docs, e := s.Store.ListDocuments(r.Context(), uid(r), kid)
		if e != nil {
			s.fail(w, e)
			return
		}
		queued := 0
		failures := []string{}
		for _, d := range docs {
			if d.Status != "ready" && d.Status != "failed" {
				continue
			}
			if e = s.Documents.Reprocess(r.Context(), uid(r), d.ID); e != nil {
				failures = append(failures, d.OriginalFilename)
			} else {
				queued++
			}
		}
		s.reply(w, map[string]any{"queued": queued, "failed": failures}, nil)
	})))
	mux.Handle("GET /api/v1/knowledge-bases/{id}/catalog", s.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		docs, e := s.Store.ListDocuments(r.Context(), uid(r), r.PathValue("id"))
		if e != nil {
			s.fail(w, e)
			return
		}
		saved, e := s.Platform.Catalogs(r.Context(), uid(r), r.PathValue("id"))
		if e != nil {
			s.fail(w, e)
			return
		}
		byID := map[string]domain.DocumentCatalog{}
		for _, c := range saved {
			byID[c.DocumentID] = c
		}
		out := []domain.DocumentCatalog{}
		for _, d := range docs {
			c, ok := byID[d.ID]
			if !ok || c.ReviewState != "confirmed" {
				suggested := service.SuggestDocumentCatalog(d)
				if ok {
					suggested.Purpose = c.Purpose
				}
				c = suggested
			}
			out = append(out, c)
		}
		s.reply(w, out, nil)
	})))
	patch := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var c domain.DocumentCatalog
		if !s.input(w, r, &c) {
			return
		}
		c.DocumentID = r.PathValue("id")
		e := s.Platform.SaveCatalog(r.Context(), uid(r), c)
		s.reply(w, c, e)
	})
	mux.Handle("PATCH /api/v1/admin/documents/{id}/catalog", s.requireAdmin(patch))
	mux.Handle("PATCH /api/v1/documents/{id}/catalog", s.auth(patch))
	organize := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		d, e := s.Store.Document(r.Context(), uid(r), id)
		if e != nil {
			s.fail(w, e)
			return
		}
		existing, e := s.Platform.Catalogs(r.Context(), uid(r), d.KnowledgeBaseID)
		if e != nil {
			s.fail(w, e)
			return
		}
		c := service.SuggestDocumentCatalog(d)
		for _, old := range existing {
			if old.DocumentID == id {
				if old.ReviewState == "confirmed" {
					c = old
				} else {
					c.Purpose = old.Purpose
				}
				break
			}
		}
		if e = s.Platform.SaveCatalog(r.Context(), uid(r), c); e != nil {
			s.fail(w, e)
			return
		}
		if c.Purpose == "knowledge" {
			s.reply(w, map[string]any{"catalog": c, "candidates": 0}, nil)
			return
		}
		parsed, e := s.Platform.DocumentStructure(r.Context(), uid(r), id)
		if e != nil {
			s.fail(w, e)
			return
		}
		points, e := s.Store.ListKnowledgePoints(r.Context())
		if e != nil {
			s.fail(w, e)
			return
		}
		assets := service.ExtractQuestions(d, parsed.Pages, points)
		e = s.Platform.RefreshQuestionCandidates(r.Context(), uid(r), d, parsed, assets)
		s.reply(w, map[string]any{"catalog": c, "candidates": len(assets)}, e)
	})
	mux.Handle("POST /api/v1/admin/documents/{id}/organize", s.requireAdmin(organize))
	mux.Handle("POST /api/v1/documents/{id}/organize", s.auth(organize))

}
