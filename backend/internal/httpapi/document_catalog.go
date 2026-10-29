package httpapi

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/service"
	"net/http"
)

func (s *Server) catalogRoutes(mux *http.ServeMux) {
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
	mux.Handle("PATCH /api/v1/admin/documents/{id}/catalog", s.requireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var c domain.DocumentCatalog
		if !s.input(w, r, &c) {
			return
		}
		c.DocumentID = r.PathValue("id")
		e := s.Platform.SaveCatalog(r.Context(), uid(r), c)
		s.reply(w, c, e)
	})))
	mux.Handle("POST /api/v1/admin/documents/{id}/organize", s.requireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	})))
}
