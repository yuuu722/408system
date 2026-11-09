package httpapi

import (
	"errors"
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/service"
	"net/http"
	"strconv"
)

func (s *Server) teachingUnit(r *http.Request) (domain.LearningUnit, error) {
	units, e := s.Platform.LearningUnits(r.Context(), uid(r))
	if e != nil {
		return domain.LearningUnit{}, e
	}
	for _, u := range units {
		if u.PointID == r.PathValue("point") {
			return u, nil
		}
	}
	return domain.LearningUnit{}, &domain.RequestError{Status: 404, Message: "该知识点不在当前学习计划中"}
}
func (s *Server) unitTeachingRoutes(mux *http.ServeMux) {
	register := func(path string, fn http.HandlerFunc) { mux.Handle(path, s.auth(fn)) }
	lesson := func(generate bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			u, e := s.teachingUnit(r)
			if e != nil {
				s.fail(w, e)
				return
			}
			key := u.Steps[0].ResourceKey + ":coherent-v1"
			if !generate {
				result, e := s.Platform.UnitLesson(r.Context(), uid(r), u.PointID, key)
				s.reply(w, map[string]any{"Lesson": result}, e)
				return
			}
			result, e := s.Platform.BuildUnitLesson(r.Context(), uid(r), u.PointID, key, func() (domain.ChatResult, error) {
				if !s.Config.AssistantStatus().Enabled {
					return domain.ChatResult{}, &domain.RequestError{Status: 503, Message: "讲解模型暂不可用，请检查模型服务后重试"}
				}
				if len(u.Knowledge) == 0 {
					return domain.ChatResult{}, &domain.RequestError{Status: 409, Message: "尚缺少可核对教材，不能生成完整知识讲解"}
				}
				doc, e := s.Platform.Document(r.Context(), uid(r), u.Knowledge[0].DocumentID)
				if e != nil {
					return domain.ChatResult{}, e
				}
				ctx := service.WithTeacher(r.Context(), "encouraging")
				if profile, err := s.Platform.Profile(ctx, uid(r)); err == nil && profile != nil {
					if memories, err := s.Platform.Memories(ctx, uid(r)); err == nil {
						ctx = service.WithLearnerContext(ctx, profile.Stage, memories)
					}
				}
				question := fmt.Sprintf("请为学习单元“%s”给出连贯完整的知识复习讲解，学习目标：%s。按概念与适用条件、原理与逻辑、易混点与小结组织成完整段落，不能只抄分块或罗列原句。仅讲有教材证据支持的内容，缺少依据的部分明确说明，不编造例子或公式；每段完整结束，来源统一放在正文后。", u.Name, u.Goal)
				return s.RAG.Chat(ctx, uid(r), doc.KnowledgeBaseID, "", question)
			})
			if e != nil {
				var requestError *domain.RequestError
				if errors.As(e, &requestError) {
					s.fail(w, e)
				} else {
					s.fail(w, &domain.RequestError{Status: 422, Message: e.Error()})
				}
				return
			}
			s.reply(w, map[string]any{"Lesson": result}, nil)
		}
	}
	register("GET /api/v1/exam408/learning-units/{point}/lesson", lesson(false))
	register("POST /api/v1/exam408/learning-units/{point}/lesson", lesson(true))
	register("GET /api/v1/exam408/knowledge-points/{point}/example-attempts", func(w http.ResponseWriter, r *http.Request) {
		revision, e := strconv.Atoi(r.URL.Query().Get("revision"))
		if e != nil {
			write(w, 400, map[string]string{"error": "例题版本无效"})
			return
		}
		_, key, e := s.Platform.ExampleResource(r.Context(), uid(r), r.PathValue("point"), r.URL.Query().Get("asset"), revision)
		if e != nil {
			s.fail(w, e)
			return
		}
		a, e := s.Platform.ExampleAttempt(r.Context(), uid(r), r.PathValue("point"), key)
		s.reply(w, map[string]any{"Assessment": a}, e)
	})
	register("POST /api/v1/exam408/knowledge-points/{point}/example-attempts", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			AssetID  string
			Revision int
			Retry    bool
		}
		if !s.input(w, r, &in) {
			return
		}
		a, e := s.Platform.CreateExampleAttempt(r.Context(), uid(r), r.PathValue("point"), in.AssetID, in.Revision, in.Retry)
		s.reply(w, a, e)
	})
	register("POST /api/v1/exam408/example-attempts/{id}/explain", func(w http.ResponseWriter, r *http.Request) {
		feedback, e := s.Platform.ExampleFeedback(r.Context(), uid(r), r.PathValue("id"))
		if e != nil {
			s.fail(w, e)
			return
		}
		if feedback != nil {
			s.reply(w, feedback, nil)
			return
		}
		a, e := s.Platform.Assessment(r.Context(), uid(r), r.PathValue("id"))
		if e != nil {
			s.fail(w, e)
			return
		}
		if a.Result == nil || len(a.Questions) != 1 || len(a.Result.Items) != 1 {
			s.fail(w, &domain.RequestError{Status: 409, Message: "请先提交例题作答，再查看讲解"})
			return
		}
		if !s.Config.AssistantStatus().Enabled {
			s.fail(w, &domain.RequestError{Status: 503, Message: "老师讲解暂不可用，已保留作答与原文解析"})
			return
		}
		result, e := s.RAG.ExplainReviewedQuestion(service.WithTeacher(r.Context(), "encouraging"), a.Questions[0], a.Result.Items[0].Selected)
		if e != nil {
			s.fail(w, &domain.RequestError{Status: 422, Message: e.Error()})
			return
		}
		s.reply(w, result, s.Platform.SaveExampleFeedback(r.Context(), uid(r), a.ID, result))
	})
}
