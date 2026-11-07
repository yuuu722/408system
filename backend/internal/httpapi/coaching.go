package httpapi

import (
	"github.com/google/uuid"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/service"
	"net/http"
	"sort"
	"strings"
)

func (s *Server) coachingRoutes(mux *http.ServeMux) {
	register := func(path string, fn http.HandlerFunc) { mux.Handle(path, s.auth(fn)) }
	register("GET /api/v1/exam408/learning/overview", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Platform.LearningOverview(r.Context(), uid(r))
		s.reply(w, v, e)
	})
	register("PUT /api/v1/exam408/learning/memories/{key}", func(w http.ResponseWriter, r *http.Request) {
		var m domain.LearnerMemory
		if !s.input(w, r, &m) {
			return
		}
		m.Key = r.PathValue("key")
		s.reply(w, map[string]bool{"ok": true}, s.Platform.SaveMemory(r.Context(), uid(r), m))
	})
	register("DELETE /api/v1/exam408/learning/memories/{key}", func(w http.ResponseWriter, r *http.Request) {
		s.reply(w, map[string]bool{"ok": true}, s.Platform.DeleteMemory(r.Context(), uid(r), r.PathValue("key")))
	})
	register("GET /api/v1/exam408/coaching", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Platform.CoachSessions(r.Context(), uid(r))
		s.reply(w, v, e)
	})
	register("POST /api/v1/exam408/coaching", s.startCoach)
	register("POST /api/v1/exam408/coaching/{id}/explain", s.explainCoach)
	mux.Handle("POST /api/v1/admin/frequency-sources", s.requireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v domain.FrequencySource
		if !s.input(w, r, &v) {
			return
		}
		s.reply(w, map[string]bool{"ok": true}, s.Platform.SaveFrequencySource(r.Context(), uid(r), v))
	})))
}
func (s *Server) startCoach(w http.ResponseWriter, r *http.Request) {
	var in struct{ PointID, KnowledgeBaseID, Teacher string }
	if !s.input(w, r, &in) {
		return
	}
	if _, e := uuid.Parse(in.PointID); e != nil {
		s.fail(w, &domain.RequestError{Status: 400, Message: "请选择知识点"})
		return
	}
	if !validTeacher(in.Teacher) {
		s.fail(w, &domain.RequestError{Status: 400, Message: "请选择有效老师风格"})
		return
	}
	kb, e := s.Platform.KnowledgeBase(r.Context(), uid(r), in.KnowledgeBaseID)
	if e != nil {
		s.fail(w, e)
		return
	}
	if !kb.IsPublic && !s.Config.PersonalKBEnabled {
		s.fail(w, domain.ErrForbidden)
		return
	}
	if !s.Config.AssistantStatus().Enabled {
		s.fail(w, &domain.RequestError{Status: 503, Message: s.Config.AssistantStatus().Message})
		return
	}
	points, e := s.Store.ListKnowledgePoints(r.Context())
	if e != nil {
		s.fail(w, e)
		return
	}
	name := ""
	for _, p := range points {
		if p.ID == in.PointID {
			name = p.Name
			break
		}
	}
	if name == "" {
		s.fail(w, &domain.RequestError{Status: 404, Message: "知识点不存在"})
		return
	}
	sessions, e := s.Platform.CoachSessions(r.Context(), uid(r))
	if e != nil {
		s.fail(w, e)
		return
	}
	for _, v := range sessions {
		if v.PointID == in.PointID && v.KnowledgeBaseID == in.KnowledgeBaseID && v.Assessment.Result == nil {
			s.reply(w, v, nil)
			return
		}
	}
	assets, e := s.Platform.Assets(r.Context(), uid(r))
	if e != nil {
		s.fail(w, e)
		return
	}
	attempted := map[string]bool{}
	history, e := s.Platform.Assessments(r.Context(), uid(r))
	if e != nil {
		s.fail(w, e)
		return
	}
	for _, a := range history {
		if a.Result != nil {
			for _, q := range a.Questions {
				attempted[domain.Family(q)] = true
			}
		}
	}
	asset := ""
	// Prefer published exam-like sources with substantial explanations. This is
	// a transparent selection policy, not a claim of calibrated typicality.
	rank := func(a domain.Asset) int {
		score := 0
		text := a.Title + " " + a.Payload.Source
		if !strings.Contains(text, "模拟") && !strings.Contains(text, "风格") && (strings.Contains(text, "统考") || strings.Contains(text, "真题")) {
			score += 10
		}
		if domain.HasQuestionExplanation(a.Payload.Explanation) {
			score += 3
		}
		return score
	}
	sort.SliceStable(assets, func(i, j int) bool { return rank(assets[i]) > rank(assets[j]) })
	for _, a := range assets {
		if kb.IsPublic && !a.IsPublic {
			continue
		}
		if !a.IsPublic && a.Payload.DocumentID != "" {
			d, de := s.Store.Document(r.Context(), uid(r), a.Payload.DocumentID)
			if de != nil || d.KnowledgeBaseID != in.KnowledgeBaseID {
				continue
			}
		}
		if a.Kind == "question" && a.Status == "ready" && a.PointID == in.PointID && a.Payload.AnswerOrigin == "source" && !attempted[domain.Family(a)] {
			asset = a.ID
			break
		}
	}
	if asset == "" {
		s.fail(w, &domain.RequestError{Status: 409, Message: "该知识点暂无未作答的可靠检查题，可先通过智能答疑学习，或选择其他知识点。"})
		return
	}
	teachingCtx := service.WithTeacher(r.Context(), in.Teacher)
	profile, pe := s.Platform.Profile(r.Context(), uid(r))
	memories, me := s.Platform.Memories(r.Context(), uid(r))
	if pe == nil && me == nil {
		stage := "beginner"
		if profile != nil {
			stage = profile.Stage
		}
		teachingCtx = service.WithLearnerContext(teachingCtx, stage, memories)
	}
	lesson, e := s.RAG.Chat(teachingCtx, uid(r), in.KnowledgeBaseID, "", name+"是什么？")
	if e != nil {
		s.fail(w, e)
		return
	}
	if lesson.Generation == nil || !lesson.Generation.Verified || len(lesson.Sources) == 0 {
		s.fail(w, &domain.RequestError{Status: 422, Message: "当前教材证据或模型讲解未通过核验，请在智能答疑核对资料后重试；未生成新的检查题。"})
		return
	}
	a, e := s.Platform.CreateAssessment(r.Context(), uid(r), "practice", in.PointID, asset)
	if e != nil {
		s.fail(w, e)
		return
	}
	v, e := s.Platform.SaveCoachSession(r.Context(), uid(r), in.KnowledgeBaseID, in.PointID, in.Teacher, a, lesson)
	s.reply(w, v, e)
}
func validTeacher(id string) bool { return id == "encouraging" || id == "direct" || id == "socratic" }

func (s *Server) explainCoach(w http.ResponseWriter, r *http.Request) {
	if _, e := uuid.Parse(r.PathValue("id")); e != nil {
		s.fail(w, &domain.RequestError{Status: 400, Message: "课堂标识无效"})
		return
	}
	sessions, e := s.Platform.CoachSessions(r.Context(), uid(r))
	if e != nil {
		s.fail(w, e)
		return
	}
	for _, v := range sessions {
		if v.ID != r.PathValue("id") {
			continue
		}
		if v.Assessment.Result == nil {
			s.fail(w, &domain.RequestError{Status: 409, Message: "请先作答，老师会等待你的回答"})
			return
		}
		if v.Feedback != nil {
			s.reply(w, v.Feedback, nil)
			return
		}
		if !s.Config.AssistantStatus().Enabled {
			s.fail(w, &domain.RequestError{Status: 503, Message: "模型暂不可用，可先核对原文解析"})
			return
		}
		a, e := s.Platform.Assessment(r.Context(), uid(r), v.Assessment.ID)
		if e != nil {
			s.fail(w, e)
			return
		}
		if len(a.Questions) != 1 || a.Result == nil || len(a.Result.Items) != 1 {
			s.fail(w, &domain.RequestError{Status: 409, Message: "课堂检查题数量不正确"})
			return
		}
		feedback, e := s.RAG.ExplainReviewedQuestion(service.WithTeacher(r.Context(), v.Teacher), a.Questions[0], a.Result.Items[0].Selected)
		if e != nil {
			s.fail(w, &domain.RequestError{Status: 422, Message: e.Error()})
			return
		}
		if e = s.Platform.SaveCoachFeedback(r.Context(), uid(r), v.ID, feedback); e != nil {
			s.fail(w, e)
			return
		}
		s.reply(w, feedback, nil)
		return
	}
	s.fail(w, &domain.RequestError{Status: 404, Message: "课堂记录不存在"})
}
