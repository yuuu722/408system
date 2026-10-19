package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/local/exam408-assistant/backend/internal/config"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/repository"
	"github.com/local/exam408-assistant/backend/internal/service"
)

type contextKey string

const userKey contextKey = "user_id"
const requestIDKey contextKey = "request_id"

type Server struct {
	Generator       service.QuestionGenerator
	QuestionAI      service.QuestionAI
	Platform        *repository.Postgres
	Store           domain.Store
	Auth            service.AuthService
	Documents       *service.DocumentService
	Knowledge       service.KnowledgeService
	Learning        service.LearningService
	Recommendations service.RecommendationService
	RAG             service.RAGService
	Config          config.Config
	Log             *slog.Logger
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { write(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("POST /api/v1/admin/bootstrap", s.bootstrapAdmin)
	mux.HandleFunc("POST /api/v1/auth/register", s.register)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.Handle("GET /api/v1/auth/me", s.auth(http.HandlerFunc(s.me)))
	mux.Handle("GET /api/v1/knowledge-bases", s.auth(http.HandlerFunc(s.listKB)))
	mux.Handle("POST /api/v1/knowledge-bases", s.auth(http.HandlerFunc(s.createKB)))
	mux.Handle("DELETE /api/v1/knowledge-bases/{id}", s.auth(http.HandlerFunc(s.deleteKB)))
	mux.Handle("GET /api/v1/knowledge-bases/{id}/documents", s.auth(http.HandlerFunc(s.listDocuments)))
	mux.Handle("POST /api/v1/knowledge-bases/{id}/documents", s.auth(http.HandlerFunc(s.uploadDocument)))
	mux.Handle("DELETE /api/v1/documents/{id}", s.auth(http.HandlerFunc(s.deleteDocument)))
	mux.Handle("POST /api/v1/documents/{id}/reprocess", s.auth(http.HandlerFunc(s.reprocessDocument)))
	mux.Handle("POST /api/v1/documents/{id}/knowledge-points/{pointID}", s.auth(http.HandlerFunc(s.mapDocument)))
	mux.Handle("GET /api/v1/exam408/subjects", s.auth(http.HandlerFunc(s.subjects)))
	mux.Handle("GET /api/v1/exam408/knowledge-points", s.auth(http.HandlerFunc(s.points)))
	mux.Handle("GET /api/v1/exam408/knowledge-points/{id}", s.auth(http.HandlerFunc(s.point)))
	mux.Handle("POST /api/v1/exam408/learning-events", s.auth(http.HandlerFunc(s.event)))
	mux.Handle("GET /api/v1/exam408/mastery", s.auth(http.HandlerFunc(s.mastery)))
	mux.Handle("GET /api/v1/exam408/weaknesses", s.auth(http.HandlerFunc(s.weaknesses)))
	mux.Handle("GET /api/v1/exam408/recommendations", s.auth(http.HandlerFunc(s.recommendations)))
	mux.Handle("POST /api/v1/exam408/recommendations/generate", s.auth(http.HandlerFunc(s.generateRecommendations)))
	mux.Handle("GET /api/v1/exam408/dashboard", s.auth(http.HandlerFunc(s.dashboard)))
	mux.Handle("POST /api/v1/exam408/assistant/chat", s.auth(http.HandlerFunc(s.chat)))
	mux.Handle("GET /api/v1/exam408/assistant/status", s.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { write(w, 200, s.Config.AssistantStatus()) })))
	if s.Platform != nil {
		s.foundationRoutes(mux)
	}
	return s.requestID(s.cors(s.recover(mux)))
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
func uid(r *http.Request) string { v, _ := r.Context().Value(userKey).(string); return v }
func (s *Server) fail(w http.ResponseWriter, err error) {
	var request interface{ PublicStatus() int }
	if errors.As(err, &request) {
		write(w, request.PublicStatus(), map[string]string{"error": err.Error()})
		return
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		write(w, 413, map[string]string{"error": "文件超过上传上限"})
		return
	}
	var public *service.PublicError
	if errors.As(err, &public) {
		write(w, public.Status, map[string]string{"error": public.Message, "code": public.Code})
		return
	}
	status := http.StatusInternalServerError
	if errors.Is(err, repository.ErrNotFound) {
		status = 404
	} else if errors.Is(err, service.ErrUntrustedLearningEvent) {
		status = http.StatusForbidden
	} else if strings.Contains(err.Error(), "already processing") {
		status = 409
	} else if strings.Contains(err.Error(), "Invalid") || strings.Contains(err.Error(), "Incomplete") || strings.Contains(err.Error(), "No available questions") || strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "supported") || strings.Contains(err.Error(), "size") {
		status = 400
	}
	if status == 500 {
		requestID := w.Header().Get("X-Request-ID")
		s.Log.Error("request failed", "request_id", requestID, "error", err)
		write(w, status, map[string]string{"error": "internal server error", "request_id": requestID})
		return
	}
	write(w, status, map[string]string{"error": err.Error()})
}
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if raw == "" {
			write(w, 401, map[string]string{"error": "authentication required"})
			return
		}
		id, err := s.Auth.Parse(raw)
		if err != nil {
			write(w, 401, map[string]string{"error": "invalid or expired token"})
			return
		}
		u, err := s.Store.UserByID(r.Context(), id)
		if err != nil || u.Disabled {
			write(w, 401, map[string]string{"error": "账号不存在或已被停用"})
			return
		}
		if !s.Config.PersonalKBEnabled && u.Role != "admin" {
			resourceWrite := r.Method != "GET" && (strings.HasPrefix(r.URL.Path, "/api/v1/knowledge-bases") || strings.HasPrefix(r.URL.Path, "/api/v1/documents") || strings.HasPrefix(r.URL.Path, "/api/v1/exam408/assets"))
			if resourceWrite {
				write(w, 403, map[string]string{"error": "普通用户只可查看公共知识库，资料由管理员维护"})
				return
			}
			if strings.HasPrefix(r.URL.Path, "/api/v1/documents/") {
				d, e := s.Store.Document(r.Context(), id, r.PathValue("id"))
				if e != nil {
					s.fail(w, e)
					return
				}
				kb, e := s.Store.KnowledgeBase(r.Context(), id, d.KnowledgeBaseID)
				if e != nil || !kb.IsPublic {
					write(w, 403, map[string]string{"error": "仅允许查看公共资料"})
					return
				}
			}
			if strings.HasPrefix(r.URL.Path, "/api/v1/knowledge-bases/") {
				kb, e := s.Store.KnowledgeBase(r.Context(), id, r.PathValue("id"))
				if e != nil {
					s.fail(w, e)
					return
				}
				if !kb.IsPublic {
					write(w, 403, map[string]string{"error": "仅允许查看公共知识库"})
					return
				}
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, id)))
	})
}

// bootstrapAdmin can promote one existing account only when no admin exists.
// The separate bootstrap token must be set out-of-band in the deployment environment.
func (s *Server) bootstrapAdmin(w http.ResponseWriter, r *http.Request) {
	if s.Platform == nil {
		write(w, http.StatusServiceUnavailable, map[string]string{"error": "admin bootstrap unavailable"})
		return
	}
	secret := strings.TrimSpace(s.Config.AdminBootstrapToken)
	if len(secret) < 32 || strings.Contains(strings.ToLower(secret), "replace") || strings.Contains(strings.ToLower(secret), "change-this") {
		write(w, http.StatusServiceUnavailable, map[string]string{"error": "管理员初始化令牌未安全配置"})
		return
	}
	var in struct {
		Token string
		Email string
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	if len(in.Token) != len(secret) || subtle.ConstantTimeCompare([]byte(in.Token), []byte(secret)) != 1 {
		write(w, http.StatusUnauthorized, map[string]string{"error": "管理员初始化令牌无效"})
		return
	}
	u, kb, err := s.Platform.BootstrapAdmin(r.Context(), in.Email)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			write(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		s.fail(w, err)
		return
	}
	write(w, http.StatusOK, map[string]any{"user": publicUser(u), "public_knowledge_base": kb, "message": "管理员已初始化；请立即从部署配置中移除 ADMIN_BOOTSTRAP_TOKEN 并重启服务"})
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return s.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := s.Store.UserByID(r.Context(), uid(r))
		if err != nil {
			s.fail(w, err)
			return
		}
		if u.Role != "admin" {
			write(w, http.StatusForbidden, map[string]string{"error": "administrator role required"})
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		for _, allowed := range s.Config.CORSOrigins {
			if strings.TrimSpace(allowed) == origin {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				break
			}
		}
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.NewString()
		w.Header().Set("X-Request-ID", requestID)
		started := time.Now()
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, requestID)))
		s.Log.Info("request completed", "request_id", requestID, "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(started).Milliseconds())
	})
}
func (s *Server) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				requestID, _ := r.Context().Value(requestIDKey).(string)
				s.Log.Error("panic", "request_id", requestID, "value", v)
				write(w, 500, map[string]string{"error": "internal server error", "request_id": requestID})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type authRequest struct{ Username, Email, Login, Password string }

func publicUser(u domain.User) any {
	return map[string]any{"id": u.ID, "username": u.Username, "email": u.Email, "role": u.Role, "created_at": u.CreatedAt}
}
func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var in authRequest
	if err := decode(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	u, t, err := s.Auth.Register(r.Context(), in.Username, in.Email, in.Password)
	if err != nil {
		s.fail(w, err)
		return
	}
	write(w, 201, map[string]any{"user": publicUser(u), "token": t})
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in authRequest
	if err := decode(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	u, t, err := s.Auth.Login(r.Context(), in.Login, in.Password)
	if err != nil {
		s.fail(w, err)
		return
	}
	write(w, 200, map[string]any{"user": publicUser(u), "token": t})
}
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, err := s.Store.UserByID(r.Context(), uid(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	write(w, 200, publicUser(u))
}
func (s *Server) createKB(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name, Description string }
	if err := decode(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		s.fail(w, fmt.Errorf("name is required"))
		return
	}
	var k domain.KnowledgeBase
	var err error
	if !s.Config.PersonalKBEnabled {
		k, err = s.Platform.CreatePublicKnowledgeBase(r.Context(), uid(r), in.Name, in.Description)
	} else {
		k, err = s.Store.CreateKnowledgeBase(r.Context(), uid(r), in.Name, in.Description)
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	write(w, 201, k)
}
func (s *Server) listKB(w http.ResponseWriter, r *http.Request) {
	v, err := s.Store.ListKnowledgeBases(r.Context(), uid(r))
	if !s.Config.PersonalKBEnabled {
		filtered := []domain.KnowledgeBase{}
		for _, kb := range v {
			if kb.IsPublic {
				filtered = append(filtered, kb)
			}
		}
		v = filtered
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	write(w, 200, v)
}
func (s *Server) deleteKB(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteKnowledgeBase(r.Context(), uid(r), r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) listDocuments(w http.ResponseWriter, r *http.Request) {
	v, err := s.Store.ListDocuments(r.Context(), uid(r), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	write(w, 200, v)
}
func fileHeader(r *http.Request) (*multipart.FileHeader, error) {
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		return nil, err
	}
	_, h, err := r.FormFile("file")
	return h, err
}
func (s *Server) uploadDocument(w http.ResponseWriter, r *http.Request) {
	maxBytes := s.Documents.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 200 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
	defer func() {
		if r.MultipartForm != nil {
			r.MultipartForm.RemoveAll()
		}
	}()
	h, err := fileHeader(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	d, err := s.Documents.Upload(r.Context(), uid(r), r.PathValue("id"), r.FormValue("resource_type"), h)
	if err != nil {
		s.fail(w, err)
		return
	}
	write(w, 202, d)
}
func (s *Server) deleteDocument(w http.ResponseWriter, r *http.Request) {
	if err := s.Documents.Delete(r.Context(), uid(r), r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) reprocessDocument(w http.ResponseWriter, r *http.Request) {
	if err := s.Documents.Reprocess(r.Context(), uid(r), r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	write(w, 202, map[string]string{"status": "processing"})
}
func (s *Server) mapDocument(w http.ResponseWriter, r *http.Request) {
	if _, err := s.Store.Document(r.Context(), uid(r), r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Store.MapDocumentToPoint(r.Context(), uid(r), r.PathValue("id"), r.PathValue("pointID")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) subjects(w http.ResponseWriter, r *http.Request) {
	v, err := s.Store.ListSubjects(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	write(w, 200, v)
}
func (s *Server) points(w http.ResponseWriter, r *http.Request) {
	v, err := s.Store.ListKnowledgePoints(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	write(w, 200, v)
}
func (s *Server) point(w http.ResponseWriter, r *http.Request) {
	p, err := s.Store.KnowledgePoint(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	rels, err := s.Store.PointRelations(r.Context(), p.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	overview, err := s.Store.KnowledgePointOverview(r.Context(), uid(r), p.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	write(w, 200, map[string]any{"knowledge_point": p, "relations": rels, "resources": overview.Resources, "mastery": overview.Mastery, "recent_events": overview.RecentEvents})
}
func (s *Server) event(w http.ResponseWriter, r *http.Request) {
	var e domain.LearningEvent
	if err := decode(r, &e); err != nil {
		s.fail(w, err)
		return
	}
	e.UserID = uid(r)
	saved, err := s.Learning.RecordUserEvent(r.Context(), e)
	if err != nil {
		s.fail(w, err)
		return
	}
	write(w, 201, saved)
}
func (s *Server) mastery(w http.ResponseWriter, r *http.Request) {
	v, err := s.Store.ListMastery(r.Context(), uid(r), false)
	if err != nil {
		s.fail(w, err)
		return
	}
	write(w, 200, v)
}
func (s *Server) weaknesses(w http.ResponseWriter, r *http.Request) {
	v, err := s.Store.ListMastery(r.Context(), uid(r), true)
	if err != nil {
		s.fail(w, err)
		return
	}
	write(w, 200, v)
}
func (s *Server) recommendations(w http.ResponseWriter, r *http.Request) {
	v, err := s.Store.ListRecommendations(r.Context(), uid(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	write(w, 200, v)
}
func (s *Server) generateRecommendations(w http.ResponseWriter, r *http.Request) {
	v, err := s.Recommendations.Generate(r.Context(), uid(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	write(w, 200, v)
}
func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	v, err := s.Store.Dashboard(r.Context(), uid(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	write(w, 200, v)
}
func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	if status := s.Config.AssistantStatus(); !status.Enabled {
		write(w, 503, map[string]string{"error": status.Message, "code": "model_not_configured"})
		return
	}
	var in struct {
		KnowledgeBaseID, ConversationID, Question string
		IncludePersonal                           bool
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, err)
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
	v, err := s.RAG.ChatStream(r.Context(), uid(r), in.KnowledgeBaseID, in.ConversationID, in.Question, nil, in.IncludePersonal)
	if err != nil {
		s.fail(w, err)
		return
	}
	write(w, 200, v)
}
