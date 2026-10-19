package httpapi

import (
	"github.com/local/exam408-assistant/backend/internal/service"
	"net/http"
)

func (s *Server) personalRoutes(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/exam408/practice/generate", s.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in service.GenerationRequest
		if !s.input(w, r, &in) {
			return
		}
		assets, e := s.Generator.Generate(r.Context(), uid(r), in)
		s.reply(w, assets, e)
	})))
	mux.Handle("POST /api/v1/exam408/assessments/{id}/questions/{qid}/confirm", s.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Decision string }
		if !s.input(w, r, &in) {
			return
		}
		v, e := s.Platform.ConfirmAnswer(r.Context(), uid(r), r.PathValue("id"), r.PathValue("qid"), in.Decision)
		s.reply(w, v, e)
	})))
	mux.Handle("POST /api/v1/exam408/assessments/{id}/questions/{qid}/infer", s.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, e := s.Platform.Assessment(r.Context(), uid(r), r.PathValue("id"))
		if e != nil {
			s.fail(w, e)
			return
		}
		if a.Result == nil {
			write(w, 400, map[string]string{"error": "请先作答再获取参考答案（Incomplete assessment）"})
			return
		}
		for _, q := range a.Questions {
			if q.ID == r.PathValue("qid") {
				if q.Payload.AnswerOrigin == "source" || q.Payload.AnswerOrigin == "" {
					write(w, 400, map[string]string{"error": "此题已有资料答案（Invalid inference request）"})
					return
				}
				// A saved suggestion is reused; a repeated click never silently changes the reference.
				for _, v := range a.Result.Items {
					if v.ID == q.ID && v.AnswerOrigin == "ai" {
						s.reply(w, a.Result, nil)
						return
					}
				}
				answer, explanation, e := s.QuestionAI.Infer(r.Context(), q)
				if e != nil {
					s.fail(w, e)
					return
				}
				result, e := s.Platform.SaveAIAnswer(r.Context(), uid(r), a.ID, q.ID, answer, explanation)
				s.reply(w, result, e)
				return
			}
		}
		write(w, 404, map[string]string{"error": "题目不存在（Not found）"})
	})))
	mux.Handle("GET /api/v1/documents/{id}/structure", s.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Platform.RevisionStructure(r.Context(), uid(r), r.PathValue("id"), r.URL.Query().Get("revision"))
		s.reply(w, v, e)
	})))
}
