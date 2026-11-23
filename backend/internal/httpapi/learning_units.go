package httpapi

import (
	"net/http"

	"github.com/local/exam408-assistant/backend/internal/domain"
)

func (s *Server) learningUnitRoutes(mux *http.ServeMux) {
	register := func(path string, fn http.HandlerFunc) { mux.Handle(path, s.auth(fn)) }
	register("GET /api/v1/exam408/knowledge-points/{point}/study", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Platform.PointStudy(r.Context(), uid(r), r.PathValue("point"))
		s.reply(w, v, e)
	})
	register("POST /api/v1/exam408/knowledge-points/{point}/lesson", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ KnowledgeBaseID, Teacher string }
		if !s.input(w, r, &in) {
			return
		}
		point, e := s.Platform.KnowledgePoint(r.Context(), r.PathValue("point"))
		if e != nil {
			s.fail(w, e)
			return
		}
		if !s.Config.AssistantStatus().Enabled {
			s.fail(w, &domain.RequestError{Status: 503, Message: "讲解模型暂不可用，仍可查看本页教材和已核对例题"})
			return
		}
		lesson, e := s.RAG.PointLesson(r.Context(), uid(r), in.KnowledgeBaseID, point.Name, in.Teacher)
		s.reply(w, lesson, e)
	})
	register("GET /api/v1/exam408/learning-units", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Platform.LearningUnits(r.Context(), uid(r))
		s.reply(w, v, e)
	})
	register("PUT /api/v1/exam408/learning-units/{point}/steps/{step}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ResourceKey string
			Completed   bool
		}
		if !s.input(w, r, &in) {
			return
		}
		step := r.PathValue("step")
		if step != "knowledge" || len(in.ResourceKey) != 64 {
			write(w, 400, map[string]string{"error": "仅支持确认知识阅读；例题、练习和复测以实际作答为准"})
			return
		}
		s.reply(w, map[string]bool{"ok": true}, s.Platform.MarkUnitStep(r.Context(), uid(r), r.PathValue("point"), step, in.ResourceKey, in.Completed))
	})
	register("POST /api/v1/exam408/learning-units/{point}/check", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Platform.CreateUnitCheck(r.Context(), uid(r), r.PathValue("point"))
		s.reply(w, v, e)
	})
}
