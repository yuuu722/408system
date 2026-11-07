package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/service"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
)

func (s *Server) foundationRoutes(mux *http.ServeMux) {
	s.personalRoutes(mux)
	s.coachingRoutes(mux)
	mux.Handle("POST /api/v1/knowledge-bases/{id}/deduplicate-questions", s.requireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Platform.DeduplicatePublicQuestions(r.Context(), uid(r), r.PathValue("id"))
		s.reply(w, v, e)
	})))
	mux.Handle("POST /api/v1/documents/{id}/repair-question-layout", s.requireAdmin(http.HandlerFunc(s.repairQuestionLayout)))
	mux.Handle("GET /api/v1/documents/{id}/question-diagnostics", s.requireAdmin(http.HandlerFunc(s.questionDiagnostics)))
	mux.Handle("POST /api/v1/documents/{id}/resolve-answers", s.requireAdmin(http.HandlerFunc(s.resolveQuestionAnswers)))
	mux.Handle("POST /api/v1/documents/{id}/publish-verified", s.requireAdmin(http.HandlerFunc(s.publishVerifiedQuestions)))
	mux.Handle("GET /api/v1/knowledge-bases/{id}/question-inventory", s.requireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Platform.PublicQuestionInventory(r.Context(), uid(r), r.PathValue("id"))
		s.reply(w, v, e)
	})))
	mux.Handle("PATCH /api/v1/knowledge-bases/{id}", s.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Name, Description string }
		if !s.input(w, r, &in) {
			return
		}
		k, e := s.Platform.UpdatePersonalKnowledgeBase(r.Context(), uid(r), r.PathValue("id"), in.Name, in.Description)
		s.reply(w, k, e)
	})))
	s.adminRoutes(mux)
	register := func(pattern string, handler http.HandlerFunc) { mux.Handle(pattern, s.auth(handler)) }
	register("GET /api/v1/conversations", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Platform.ListConversations(r.Context(), uid(r))
		s.reply(w, v, e)
	})
	register("GET /api/v1/conversations/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Platform.ConversationMessages(r.Context(), uid(r), r.PathValue("id"), "")
		s.reply(w, v, e)
	})
	register("PATCH /api/v1/conversations/{id}", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Title string }
		if !s.input(w, r, &in) {
			return
		}
		s.reply(w, map[string]bool{"ok": true}, s.Platform.RenameConversation(r.Context(), uid(r), r.PathValue("id"), in.Title))
	})
	register("DELETE /api/v1/conversations/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.reply(w, map[string]bool{"ok": true}, s.Platform.DeleteConversation(r.Context(), uid(r), r.PathValue("id")))
	})
	register("GET /api/v1/documents/{id}/file", s.documentFile)
	register("GET /api/v1/knowledge-bases/{id}/graph", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Platform.KnowledgeGraph(r.Context(), uid(r), r.PathValue("id"))
		if e == nil {
			overview, oe := s.Platform.LearningOverview(r.Context(), uid(r))
			if oe == nil {
				byID := map[string]domain.PointInsight{}
				for _, p := range overview.Points {
					byID[p.ID] = p
				}
				for i := range v.Nodes {
					if p, ok := byID[v.Nodes[i].KnowledgePointID]; ok {
						v.Nodes[i].Importance = p.Importance
						v.Nodes[i].ImportanceBasis = p.ImportanceBasis
						v.Nodes[i].ExamYears = p.ExamYears
						v.Nodes[i].CorpusYears = p.CorpusYears
					}
				}
			}
		}
		s.reply(w, v, e)
	})
	register("POST /api/v1/exam408/assistant/stream", s.chatStream)
	register("GET /api/v1/exam408/study-summary", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Platform.StudySummary(r.Context(), uid(r))
		s.reply(w, v, e)
	})
	register("GET /api/v1/exam408/profile", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Platform.Profile(r.Context(), uid(r))
		s.reply(w, v, e)
	})
	register("PUT /api/v1/exam408/profile", func(w http.ResponseWriter, r *http.Request) {
		var in domain.Profile
		if !s.input(w, r, &in) {
			return
		}
		in.DisplayName = strings.TrimSpace(in.DisplayName)
		in.Bio = strings.TrimSpace(in.Bio)
		if e := service.ValidateProfile(in); e != nil {
			write(w, 400, map[string]string{"error": e.Error()})
			return
		}
		s.reply(w, in, s.Platform.SaveProfile(r.Context(), uid(r), in))
	})
	register("GET /api/v1/exam408/assets", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Platform.Assets(r.Context(), uid(r))
		if !s.Config.PersonalKBEnabled {
			filtered := []domain.Asset{}
			for _, a := range v {
				if a.IsPublic {
					filtered = append(filtered, a)
				}
			}
			v = filtered
		}
		user, ue := s.Store.UserByID(r.Context(), uid(r))
		if ue != nil {
			s.fail(w, ue)
			return
		}
		if user.Role != "admin" {
			visible := []domain.Asset{}
			for _, a := range v {
				if a.Status == "ready" || !a.IsPublic {
					visible = append(visible, a)
				}
			}
			v = visible
		}
		s.reply(w, v, e)
	})
	register("POST /api/v1/exam408/assets", s.saveAsset)
	register("PUT /api/v1/exam408/assets/{id}", s.saveAsset)
	register("DELETE /api/v1/exam408/assets/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.reply(w, map[string]bool{"ok": true}, s.Platform.DeleteAsset(r.Context(), uid(r), r.PathValue("id")))
	})
	register("GET /api/v1/exam408/assessments", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Platform.Assessments(r.Context(), uid(r))
		s.reply(w, v, e)
	})
	register("POST /api/v1/exam408/assessments", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Kind, PointID, AssetID string }
		if !s.input(w, r, &in) {
			return
		}
		if in.Kind != "diagnostic" && in.Kind != "practice" && in.Kind != "retest" {
			write(w, 400, map[string]string{"error": "不支持的测评类型（Invalid assessment type）"})
			return
		}
		v, e := s.Platform.CreateAssessment(r.Context(), uid(r), in.Kind, in.PointID, in.AssetID)
		s.reply(w, v, e)
	})
	register("POST /api/v1/exam408/assessments/{id}/submit", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Answers map[string]int }
		if !s.input(w, r, &in) {
			return
		}
		v, e := s.Platform.SubmitAssessment(r.Context(), uid(r), r.PathValue("id"), in.Answers)
		if e == nil {
			if pe := s.Platform.GenerateTasks(r.Context(), uid(r)); pe != nil && s.Log != nil {
				s.Log.Warn("plan refresh failed; assessment preserved", "error", pe)
			}
		}
		s.reply(w, v, e)
	})
	register("GET /api/v1/exam408/radar", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Platform.Radar(r.Context(), uid(r))
		s.reply(w, v, e)
	})
	register("GET /api/v1/exam408/tasks", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Platform.Tasks(r.Context(), uid(r))
		s.reply(w, v, e)
	})
	register("GET /api/v1/exam408/tasks/plan", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Platform.DailyPlan(r.Context(), uid(r))
		s.reply(w, v, e)
	})
	register("POST /api/v1/exam408/tasks/generate", func(w http.ResponseWriter, r *http.Request) {
		s.reply(w, map[string]bool{"ok": true}, s.Platform.GenerateTasks(r.Context(), uid(r)))
	})
	register("POST /api/v1/exam408/tasks/{id}/complete", func(w http.ResponseWriter, r *http.Request) {
		s.reply(w, map[string]bool{"ok": true}, s.Platform.CompleteTask(r.Context(), uid(r), r.PathValue("id")))
	})
}
func (s *Server) reply(w http.ResponseWriter, v any, err error) {
	if err != nil {
		s.fail(w, err)
		return
	}
	write(w, 200, v)
}
func (s *Server) input(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := decode(r, v); err != nil {
		write(w, 400, map[string]string{"error": "请求格式不正确（Invalid request body）"})
		return false
	}
	return true
}
func (s *Server) saveAsset(w http.ResponseWriter, r *http.Request) {
	var a domain.Asset
	a.Payload.Answer = -1
	if !s.input(w, r, &a) {
		return
	}
	a.ID = r.PathValue("id")
	if !s.Config.PersonalKBEnabled {
		a.IsPublic = true
	}
	a.Payload.Issue = ""
	a.Payload.ExtractionKey = ""
	a.Payload.ContentHash = ""
	a.Payload.DifficultyOrigin = "user"
	a.Payload.AnswerOrigin = "ai_pending"
	if a.Payload.Answer >= 0 {
		a.Payload.AnswerOrigin = "source"
	}
	if a.ID != "" {
		old, e := s.Platform.Asset(r.Context(), uid(r), a.ID)
		if e != nil {
			s.fail(w, e)
			return
		}
		if old.Payload.ExtractionKey != "" && a.Payload.ReviewStatus != "published" {
			a.Payload.ReviewStatus = "pending"
		}
		a.Payload.ExtractionKey = old.Payload.ExtractionKey
		a.Payload.QuestionOrigin = old.Payload.QuestionOrigin
		a.Payload.FamilyID = old.Payload.FamilyID
		a.Payload.Evidence = old.Payload.Evidence
		a.Payload.GeneratorModel = old.Payload.GeneratorModel
		a.Payload.GeneratedAt = old.Payload.GeneratedAt
		a.Payload.ParentAssetID = old.Payload.ParentAssetID
		if old.Payload.QuestionOrigin == "ai_generated" {
			a.Payload.AnswerOrigin = "ai"
			a.Payload.DifficultyOrigin = "ai_estimate"
		}
	} else {
		a.Payload.QuestionOrigin = "user"
		a.Payload.GeneratorModel = ""
		a.Payload.GeneratedAt = ""
		a.Payload.Evidence = nil
		a.Payload.RevisionID = ""
		a.Payload.ContentHash = ""
	}
	if err := service.ValidateAsset(a); err != nil {
		write(w, 400, map[string]string{"error": err.Error()})
		return
	}
	v, e := s.Platform.SaveAsset(r.Context(), uid(r), a)
	if e == nil && v.IsPublic && v.Kind == "material" && strings.TrimSpace(v.Payload.Stem) != "" {
		e = s.Platform.SyncPublicMaterial(r.Context(), s.Documents.StorageRoot, v.ID)
		if e == nil {
			v, e = s.Platform.Asset(r.Context(), uid(r), v.ID)
		}
	}
	s.reply(w, v, e)
}
func (s *Server) documentFile(w http.ResponseWriter, r *http.Request) {
	d, err := s.Store.Document(r.Context(), uid(r), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	revision := r.URL.Query().Get("revision")
	explicit := revision != ""
	if revision == "" {
		revision = d.ActiveRevisionID
	}
	if revision != "" {
		version, e := s.Platform.RevisionSource(r.Context(), uid(r), d.ID, revision)
		if e != nil && explicit {
			s.fail(w, e)
			return
		}
		if e == nil {
			d = version
		}
	}
	root, err := filepath.Abs(s.Documents.StorageRoot)
	if err != nil {
		s.fail(w, err)
		return
	}
	path, err := filepath.Abs(d.StoragePath)
	if err != nil {
		s.fail(w, err)
		return
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		write(w, 403, map[string]string{"error": "禁止访问（Access denied）"})
		return
	}
	disposition := "attachment"
	if d.MimeType == "application/pdf" {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", d.MimeType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": d.OriginalFilename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeFile(w, r, path)
}
func (s *Server) chatStream(w http.ResponseWriter, r *http.Request) {
	if !s.Config.AssistantStatus().Enabled {
		write(w, 503, map[string]string{"error": s.Config.AssistantStatus().Message})
		return
	}
	var in struct {
		KnowledgeBaseID, ConversationID, Question string
		IncludePersonal                           bool
		Teacher                                   string
	}
	if !s.input(w, r, &in) {
		return
	}
	if !s.Config.PersonalKBEnabled {
		kb, e := s.Store.KnowledgeBase(r.Context(), uid(r), in.KnowledgeBaseID)
		if e != nil {
			s.fail(w, e)
			return
		}
		if !kb.IsPublic {
			s.fail(w, domain.ErrForbidden)
			return
		}
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		write(w, 500, map[string]string{"error": "Streaming unavailable"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	emit := func(event string, data any) error {
		b, e := json.Marshal(data)
		if e != nil {
			return e
		}
		if _, e = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b); e != nil {
			return e
		}
		flusher.Flush()
		return r.Context().Err()
	}
	if in.Teacher == "" {
		in.Teacher = "encouraging"
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
	result, err := s.RAG.ChatStream(teachingCtx, uid(r), in.KnowledgeBaseID, in.ConversationID, in.Question, emit, in.IncludePersonal)
	if err != nil {
		if s.Log != nil {
			s.Log.Error("assistant stream failed", "error", err)
		}
		message := "回答未完成，请重试（Answer incomplete）"
		var public *service.PublicError
		if errors.As(err, &public) {
			message = public.Message
		}
		_ = emit("error", map[string]string{"error": message})
		return
	}
	if result.Generation != nil && result.Generation.Verified {
		if e := s.Platform.RecordQuestionDoubts(r.Context(), uid(r), in.Question); e != nil && s.Log != nil {
			s.Log.Warn("question memory unavailable", "error", e)
		}
	}
	_ = emit("done", result)
}
