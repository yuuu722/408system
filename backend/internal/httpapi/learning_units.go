package httpapi

import "net/http"

func (s *Server) learningUnitRoutes(mux *http.ServeMux) {
	register := func(path string, fn http.HandlerFunc) { mux.Handle(path, s.auth(fn)) }
	register("GET /api/v1/exam408/knowledge-points/{point}/study", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Platform.PointStudy(r.Context(), uid(r), r.PathValue("point"))
		s.reply(w, v, e)
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
