package httpapi

import (
	"context"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/service"
	"net/http"
	"strconv"
)

func (s *Server) adminRoutes(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/admin/assistant/check", s.requireAdmin(http.HandlerFunc(s.checkAssistant)))
	mux.Handle("POST /api/v1/admin/documents/retry-failed", s.requireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct{ KnowledgeBaseID string }
		if !s.input(w, r, &in) {
			return
		}
		n, e := s.Platform.RetryFailedPublicDocuments(r.Context(), uid(r), in.KnowledgeBaseID)
		s.reply(w, map[string]int64{"queued": n}, e)
	})))
	mux.Handle("PATCH /api/v1/admin/knowledge-bases/{id}", s.requireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Name, Description string }
		if !s.input(w, r, &in) {
			return
		}
		kb, e := s.Platform.UpdatePublicKnowledgeBase(r.Context(), uid(r), r.PathValue("id"), in.Name, in.Description)
		s.reply(w, kb, e)
	})))
	mux.Handle("GET /api/v1/admin/users", s.requireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		v, e := s.Platform.ManageUsers(r.Context(), uid(r), r.URL.Query().Get("q"), offset)
		s.reply(w, v, e)
	})))
	mux.Handle("PATCH /api/v1/admin/users/{id}", s.requireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Role     string
			Disabled bool
		}
		if !s.input(w, r, &in) {
			return
		}
		e := s.Platform.UpdateManagedUser(r.Context(), uid(r), r.PathValue("id"), in.Role, in.Disabled)
		s.reply(w, map[string]bool{"ok": e == nil}, e)
	})))
	mux.Handle("PUT /api/v1/documents/{id}/file", s.requireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d, e := s.Store.Document(r.Context(), uid(r), r.PathValue("id"))
		if e != nil {
			s.fail(w, e)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, s.Documents.MaxBytes+(1<<20))
		defer func() {
			if r.MultipartForm != nil {
				r.MultipartForm.RemoveAll()
			}
		}()
		h, e := fileHeader(r)
		if e != nil {
			s.fail(w, e)
			return
		}
		d, e = s.Documents.Upload(r.Context(), uid(r), d.KnowledgeBaseID, r.FormValue("resource_type"), h, d.ID)
		if e != nil {
			s.fail(w, e)
			return
		}
		write(w, 202, d)
	})))
	mux.Handle("GET /api/v1/system/capabilities", s.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		max := s.Config.MaxUploadBytes
		if max <= 0 {
			max = 200 << 20
		}
		mode := "remote"
		if s.Config.UsesLocalIndex() {
			mode = "lexical"
		}
		if s.Config.DemoMode {
			mode = "demo"
		}
		write(w, 200, map[string]any{"max_upload_bytes": max, "index_mode": mode, "personal_kb_enabled": s.Config.PersonalKBEnabled, "native_ocr": service.NativeOCRAvailable()})
	})))

	save := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var point domain.KnowledgePoint
		if !s.input(w, r, &point) {
			return
		}
		point.ID = r.PathValue("id")
		store, ok := s.Store.(interface {
			SaveKnowledgePoint(context.Context, string, domain.KnowledgePoint) (domain.KnowledgePoint, error)
		})
		if !ok {
			write(w, 503, map[string]string{"error": "目录维护暂不可用"})
			return
		}
		v, e := store.SaveKnowledgePoint(r.Context(), uid(r), point)
		s.reply(w, v, e)
	})
	mux.Handle("POST /api/v1/admin/knowledge-points", s.requireAdmin(save))
	mux.Handle("PUT /api/v1/admin/knowledge-points/{id}", s.requireAdmin(save))
}
