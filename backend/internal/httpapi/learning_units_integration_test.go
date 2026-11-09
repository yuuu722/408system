package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/httpapi"
	"github.com/local/exam408-assistant/backend/internal/repository"
	"github.com/local/exam408-assistant/backend/internal/service"
	"github.com/local/exam408-assistant/backend/migrations"
	"github.com/stretchr/testify/require"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLearningUnitsSourcesProgressPrivacyAndNewChecks(t *testing.T) {
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("isolated PostgreSQL required")
	}
	ctx := context.Background()
	root, e := repository.Open(ctx, raw)
	require.NoError(t, e)
	defer root.DB.Close()
	name := "units_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, e = root.DB.Exec(ctx, `CREATE DATABASE `+name)
	require.NoError(t, e)
	parsed, e := url.Parse(raw)
	require.NoError(t, e)
	parsed.Path = "/" + name
	p, e := repository.Open(ctx, parsed.String())
	require.NoError(t, e)
	defer func() {
		p.DB.Close()
		_, e := root.DB.Exec(ctx, `DROP DATABASE `+name+` WITH(FORCE)`)
		require.NoError(t, e)
	}()
	require.NoError(t, migrations.Up(ctx, p.DB))
	require.NoError(t, migrations.Up(ctx, p.DB))
	auth := service.AuthService{Store: p, Secret: []byte(strings.Repeat("u", 40)), TTL: time.Hour}
	admin, _, e := auth.Register(ctx, "unitadmin", "ua@example.invalid", "fixture-password")
	require.NoError(t, e)
	_, e = p.DB.Exec(ctx, `UPDATE users SET role='admin' WHERE id=$1`, admin.ID)
	require.NoError(t, e)
	learner, token, e := auth.Register(ctx, "unitstudent", "us@example.invalid", "fixture-password")
	require.NoError(t, e)
	other, otherToken, e := auth.Register(ctx, "unitother", "uo@example.invalid", "fixture-password")
	require.NoError(t, e)
	points, e := p.ListKnowledgePoints(ctx)
	require.NoError(t, e)
	point := points[0]
	for i := 0; i < 8; i++ {
		_, e = p.SaveAsset(ctx, admin.ID, domain.Asset{IsPublic: true, Kind: "question", PointID: point.ID, Title: fmt.Sprintf("单元合成题%d", i), Payload: domain.AssetPayload{Stem: fmt.Sprintf("第%d个系统资源应如何管理？", i), Options: []string{"采用资源管理机制", "完全忽略资源"}, Answer: 0, AnswerOrigin: "source", ReviewStatus: "published", Explanation: "题目要求管理资源，依据定义，应该采用资源管理机制。", Source: "合成测试原文"}})
		require.NoError(t, e)
	}
	kb, e := p.CreateKnowledgeBase(ctx, learner.ID, "教材样本", "")
	require.NoError(t, e)
	doc, e := p.CreateDocument(ctx, domain.Document{UserID: learner.ID, KnowledgeBaseID: kb.ID, Filename: "book.txt", OriginalFilename: "教材样本", MimeType: "text/plain", StoragePath: "fixture", ResourceType: "textbook", Purpose: "knowledge", Status: "ready"})
	require.NoError(t, e)
	require.NoError(t, p.ReplaceChunks(ctx, learner.ID, doc.ID, []domain.Chunk{{ID: uuid.NewString(), DocumentID: doc.ID, KnowledgeBaseID: kb.ID, UserID: learner.ID, Content: point.Name + "的定义与适用条件在此。", SourceLocation: "第3页", Embedding: []float32{1, 0}, Metadata: map[string]any{"page": 3, "heading_path": point.Name}}}))
	// Another user's equally matching private textbook must never become this unit's evidence.
	private, e := p.CreateKnowledgeBase(ctx, other.ID, "私有教材", "")
	require.NoError(t, e)
	hidden, e := p.CreateDocument(ctx, domain.Document{UserID: other.ID, KnowledgeBaseID: private.ID, Filename: "hidden.txt", OriginalFilename: "其他用户教材", MimeType: "text/plain", StoragePath: "fixture", ResourceType: "textbook", Purpose: "knowledge", Status: "ready"})
	require.NoError(t, e)
	require.NoError(t, p.ReplaceChunks(ctx, other.ID, hidden.ID, []domain.Chunk{{ID: uuid.NewString(), DocumentID: hidden.ID, KnowledgeBaseID: private.ID, UserID: other.ID, Content: point.Name + "私有定义。", Embedding: []float32{1, 0}}}))
	require.NoError(t, p.GenerateTasks(ctx, learner.ID))
	plan, e := p.DailyPlan(ctx, learner.ID)
	require.NoError(t, e)
	require.LessOrEqual(t, plan.AllocatedMinutes, plan.BudgetMinutes)
	require.Equal(t, "408-learning-units-v1", plan.Strategy)
	units, e := p.LearningUnits(ctx, learner.ID)
	require.NoError(t, e)
	require.Len(t, units, 1)
	u := units[0]
	require.Len(t, u.Steps, 4)
	require.NotEmpty(t, u.Knowledge)
	require.NotNil(t, u.Example)
	require.NotEmpty(t, u.Practice)
	require.GreaterOrEqual(t, u.ScheduledMinutes, 25)
	for _, s := range u.Knowledge {
		require.Equal(t, doc.ID, s.DocumentID)
	}
	for _, q := range u.Practice {
		require.NotEqual(t, domain.Family(*u.Example), domain.Family(q))
		require.Equal(t, -1, q.Payload.Answer)
		require.Empty(t, q.Payload.Explanation)
	}
	require.NoError(t, p.MarkUnitStep(ctx, learner.ID, point.ID, "knowledge", u.Steps[0].ResourceKey, true))
	require.Error(t, p.MarkUnitStep(ctx, other.ID, point.ID, "knowledge", u.Steps[0].ResourceKey, true))
	require.Error(t, p.MarkUnitStep(ctx, learner.ID, point.ID, "practice", "", true))
	refreshed, e := p.LearningUnits(ctx, learner.ID)
	require.NoError(t, e)
	require.True(t, refreshed[0].Steps[0].Completed)
	require.False(t, refreshed[0].Steps[2].Completed)
	for _, q := range u.Practice {
		a, e := p.CreateAssessment(ctx, learner.ID, "practice", point.ID, q.ID)
		require.NoError(t, e)
		_, e = p.SubmitAssessment(ctx, learner.ID, a.ID, map[string]int{q.ID: 0})
		require.NoError(t, e)
	}
	refreshed, e = p.LearningUnits(ctx, learner.ID)
	require.NoError(t, e)
	require.True(t, refreshed[0].Steps[2].Completed)
	// A submitted exercise must not replace the remaining resources mid-unit.
	require.NoError(t, p.GenerateTasks(ctx, learner.ID))
	stable, e := p.LearningUnits(ctx, learner.ID)
	require.NoError(t, e)
	require.Equal(t, len(u.Practice), len(stable[0].Practice))
	for i := range u.Practice {
		require.Equal(t, u.Practice[i].ID, stable[0].Practice[i].ID)
	}
	require.True(t, stable[0].Steps[0].Completed)
	require.NoError(t, p.MarkUnitStep(ctx, learner.ID, point.ID, "example", u.Steps[1].ResourceKey, true))
	check, e := p.CreateUnitCheck(ctx, learner.ID, point.ID)
	require.NoError(t, e)
	require.Len(t, check.Questions, 1)
	require.Equal(t, -1, check.Questions[0].Payload.Answer)
	require.NotEqual(t, domain.Family(*u.Example), domain.Family(check.Questions[0]))
	for _, q := range u.Practice {
		require.NotEqual(t, domain.Family(q), domain.Family(check.Questions[0]))
	}
	resumed, e := p.CreateUnitCheck(ctx, learner.ID, point.ID)
	require.NoError(t, e)
	require.Equal(t, check.ID, resumed.ID)
	_, e = p.SubmitAssessment(ctx, learner.ID, check.ID, map[string]int{check.Questions[0].ID: 1})
	require.NoError(t, e)
	refreshed, e = p.LearningUnits(ctx, learner.ID)
	require.NoError(t, e)
	require.True(t, refreshed[0].Steps[3].Completed)
	require.Contains(t, refreshed[0].Steps[3].Notice, "仍有错误")
	require.NoError(t, p.GenerateTasks(ctx, learner.ID))
	completed, e := p.Tasks(ctx, learner.ID)
	require.NoError(t, e)
	for _, task := range completed {
		require.True(t, task.Completed)
	}
	// Reading and example progress survive a fresh repository instance.
	same, e := repository.Open(ctx, parsed.String())
	require.NoError(t, e)
	savedUnits, e := same.LearningUnits(ctx, learner.ID)
	same.DB.Close()
	require.NoError(t, e)
	require.True(t, savedUnits[0].Steps[0].Completed)
	// HTTP progress endpoint must remain authenticated and owner scoped.
	server := (&httpapi.Server{Store: p, Platform: p, Auth: auth, Log: slog.Default()}).Handler()
	for _, tc := range []struct {
		token string
		code  int
	}{{"", 401}, {otherToken, 404}, {token, 200}} {
		body, _ := json.Marshal(map[string]any{"ResourceKey": u.Steps[0].ResourceKey, "Completed": false})
		r := httptest.NewRequest("PUT", "/api/v1/exam408/learning-units/"+point.ID+"/steps/knowledge", bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		require.Equal(t, tc.code, w.Code, w.Body.String())
	}
	short, _, e := auth.Register(ctx, "unitshort", "ut@example.invalid", "fixture-password")
	require.NoError(t, e)
	require.NoError(t, p.SaveProfile(ctx, short.ID, domain.Profile{DailyMinutes: 10, Stage: "beginner", Preference: "mixed"}))
	bank, e := p.Assets(ctx, short.ID)
	require.NoError(t, e)
	require.NotEmpty(t, bank)
	previous, e := p.CreateAssessment(ctx, short.ID, "practice", point.ID, bank[0].ID)
	require.NoError(t, e)
	_, e = p.SubmitAssessment(ctx, short.ID, previous.ID, map[string]int{previous.Questions[0].ID: 1})
	require.NoError(t, e)
	require.NoError(t, p.GenerateTasks(ctx, short.ID))
	partial, e := p.LearningUnits(ctx, short.ID)
	require.NoError(t, e)
	require.Len(t, partial, 1)
	require.Equal(t, 10, partial[0].ScheduledMinutes)
	require.Greater(t, partial[0].EstimatedMinutes, partial[0].ScheduledMinutes)
	require.False(t, partial[0].Steps[2].Completed, "pre-plan attempts do not complete this unit's independent practice")
	require.False(t, partial[0].Steps[0].Available, "private textbook must not leak to another member")

	t.Run("point study works outside daily plan and aggregates directory descendants", func(t *testing.T) {
		child := uuid.NewString()
		_, e := p.DB.Exec(ctx, `INSERT INTO exam408_knowledge_points(id,subject_id,parent_id,code,name,keywords) VALUES($1,$2,$3,'TEST.CHILD','合成子概念',ARRAY['合成子概念'])`, child, point.SubjectID, point.ID)
		require.NoError(t, e)
		childQuestion, e := p.SaveAsset(ctx, admin.ID, domain.Asset{IsPublic: true, Kind: "question", PointID: child, Title: "子知识点合成题", Payload: domain.AssetPayload{Stem: "合成子概念应当如何核对？", Options: []string{"核对定义和条件", "忽略条件"}, Answer: 0, AnswerOrigin: "source", ReviewStatus: "published", Explanation: "应当结合定义和适用条件核对，因此选择核对定义和条件。", Source: "子知识点合成资料"}})
		require.NoError(t, e)
		book, e := p.CreateDocument(ctx, domain.Document{UserID: learner.ID, KnowledgeBaseID: kb.ID, Filename: "child.txt", OriginalFilename: "子概念教材", MimeType: "text/plain", StoragePath: "fixture", ResourceType: "textbook", Purpose: "knowledge", Status: "ready"})
		require.NoError(t, e)
		require.NoError(t, p.ReplaceChunks(ctx, learner.ID, book.ID, []domain.Chunk{{ID: uuid.NewString(), DocumentID: book.ID, KnowledgeBaseID: kb.ID, UserID: learner.ID, Content: "合成 子　概念 的定义与适用条件。", Embedding: []float32{1, 0}}}))
		pack, e := p.PointStudy(ctx, learner.ID, child)
		require.NoError(t, e)
		require.Equal(t, 1, pack.DirectQuestions)
		require.NotEmpty(t, pack.Knowledge)
		require.NotEmpty(t, pack.Ancestors)
		require.Equal(t, kb.ID, pack.PreferredKnowledgeBaseID)
		require.Empty(t, pack.Exercises, "only one family: do not repeat the example as an independent exercise")
		foreign, e := p.PointStudy(ctx, other.ID, child)
		require.NoError(t, e)
		require.Empty(t, foreign.Knowledge, "private textbook excerpts must not leak")
		rootPack, e := p.PointStudy(ctx, learner.ID, point.ID)
		require.NoError(t, e)
		require.Greater(t, rootPack.SubtreeQuestions, rootPack.DirectQuestions)
		for _, q := range rootPack.Exercises {
			require.NotEqual(t, domain.Family(*rootPack.Example), domain.Family(q))
			require.Equal(t, -1, q.Payload.Answer)
			require.Empty(t, q.Payload.Explanation)
		}
		overview, e := p.LearningOverview(ctx, learner.ID)
		require.NoError(t, e)
		for _, v := range overview.Points {
			if v.ID == point.ID {
				require.Greater(t, v.SubtreeQuestions, v.AvailableQuestions)
			}
		}
		wrong, e := p.CreateAssessment(ctx, learner.ID, "practice", child, childQuestion.ID)
		require.NoError(t, e)
		_, e = p.SubmitAssessment(ctx, learner.ID, wrong.ID, map[string]int{childQuestion.ID: 1})
		require.NoError(t, e)
		pack, e = p.PointStudy(ctx, learner.ID, child)
		require.NoError(t, e)
		require.Len(t, pack.Mistakes, 1)
		correct, e := p.CreateAssessment(ctx, learner.ID, "practice", child, childQuestion.ID)
		require.NoError(t, e)
		_, e = p.SubmitAssessment(ctx, learner.ID, correct.ID, map[string]int{childQuestion.ID: 0})
		require.NoError(t, e)
		pack, e = p.PointStudy(ctx, learner.ID, child)
		require.NoError(t, e)
		require.Empty(t, pack.Mistakes, "corrected latest attempt must replace the older mistake")
		request := httptest.NewRequest("GET", "/api/v1/exam408/knowledge-points/"+child+"/study", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		require.Equal(t, 200, response.Code, response.Body.String())
	})

	t.Run("TCP textbook references do not substitute HDLC purpose paragraphs", func(t *testing.T) {
		id := uuid.NewString()
		_, e := p.DB.Exec(ctx, `INSERT INTO exam408_knowledge_points(id,subject_id,code,name,keywords) VALUES($1,$2,'TEST.TCP','TCP可靠传输与拥塞控制',ARRAY['可靠传输','确认','重传'])`, id, point.SubjectID)
		require.NoError(t, e)
		tcpBook, e := p.CreateDocument(ctx, domain.Document{UserID: learner.ID, KnowledgeBaseID: kb.ID, Filename: "tcp.txt", OriginalFilename: "TCP合成教材", MimeType: "text/plain", StoragePath: "fixture", ResourceType: "textbook", Purpose: "knowledge", Status: "ready"})
		require.NoError(t, e)
		require.NoError(t, p.ReplaceChunks(ctx, learner.ID, tcpBook.ID, []domain.Chunk{{ID: uuid.NewString(), DocumentID: tcpBook.ID, KnowledgeBaseID: kb.ID, UserID: learner.ID, ChunkIndex: 0, Content: "HDLC在数据链路层采用确认和重传来保证可靠传输。", Embedding: []float32{1, 0}}, {ID: uuid.NewString(), DocumentID: tcpBook.ID, KnowledgeBaseID: kb.ID, UserID: learner.ID, ChunkIndex: 1, Content: "TCP通过序号、确认和重传实现可靠传输。", Embedding: []float32{1, 0}}}))
		pack, e := p.PointStudy(ctx, learner.ID, id)
		require.NoError(t, e)
		require.Len(t, pack.Knowledge, 1)
		require.Contains(t, pack.Knowledge[0].Excerpt, "TCP")
		require.NotContains(t, pack.Knowledge[0].Excerpt, "HDLC")
	})

}
