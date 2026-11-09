package httpapi

import "net/http"

func (s *Server) learningUnitRoutes(mux *http.ServeMux) {
	register := func(path string, fn http.HandlerFunc) { mux.Handle(path, s.auth(fn)) }
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
		if (step != "knowledge" && step != "example") || len(in.ResourceKey) != 64 {
			write(w, 400, map[string]string{"error": "仅支持记录教材阅读与例题学习；作答进度以实际提交为准"})
			return
		}
		s.reply(w, map[string]bool{"ok": true}, s.Platform.MarkUnitStep(r.Context(), uid(r), r.PathValue("point"), step, in.ResourceKey, in.Completed))
	})
	register("POST /api/v1/exam408/learning-units/{point}/check", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Platform.CreateUnitCheck(r.Context(), uid(r), r.PathValue("point"))
		s.reply(w, v, e)
	})
}
