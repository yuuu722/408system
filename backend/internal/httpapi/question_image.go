package httpapi

import (
	"io"
	"net/http"

	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/service"
)

func (s *Server) recognizeQuestion(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, service.MaxQuestionImageBytes+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		write(w, 400, map[string]string{"error": "问题图片请求无效或超过10 MiB"})
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	kb, err := s.Store.KnowledgeBase(r.Context(), uid(r), r.FormValue("KnowledgeBaseID"))
	if err != nil {
		s.fail(w, err)
		return
	}
	if !s.Config.PersonalKBEnabled && !kb.IsPublic {
		s.fail(w, domain.ErrForbidden)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		write(w, 400, map[string]string{"error": "请选择题目图片"})
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, service.MaxQuestionImageBytes+1))
	if err != nil {
		s.fail(w, err)
		return
	}
	vision := s.RAG.Vision
	if s.Config.DemoMode {
		vision = nil
	} // Explicit demo never invokes an external model.
	result, err := service.RecognizeQuestionImage(r.Context(), vision, data)
	if err != nil {
		s.fail(w, err)
		return
	}
	write(w, 200, result)
}
