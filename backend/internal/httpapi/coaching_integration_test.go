package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/local/exam408-assistant/backend/internal/config"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/httpapi"
	"github.com/local/exam408-assistant/backend/internal/repository"
	"github.com/local/exam408-assistant/backend/internal/service"
	"github.com/local/exam408-assistant/backend/migrations"
	"github.com/stretchr/testify/require"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLearningCoachPersistencePrivacyAndEvidenceBoundaries(t *testing.T) {
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("isolated PostgreSQL required")
	}
	ctx := context.Background()
	root, e := repository.Open(ctx, raw)
	require.NoError(t, e)
	defer root.DB.Close()
	name := "coach_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, e = root.DB.Exec(ctx, `CREATE DATABASE `+name)
	require.NoError(t, e)
	u, e := url.Parse(raw)
	require.NoError(t, e)
	u.Path = "/" + name
	p, e := repository.Open(ctx, u.String())
	require.NoError(t, e)
	defer func() {
		p.DB.Close()
		_, e := root.DB.Exec(ctx, `DROP DATABASE `+name+` WITH(FORCE)`)
		require.NoError(t, e)
	}()
	require.NoError(t, migrations.Up(ctx, p.DB))
	require.NoError(t, migrations.Up(ctx, p.DB))
	auth := service.AuthService{Store: p, Secret: []byte(strings.Repeat("t", 40)), TTL: time.Hour}
	admin, at, e := auth.Register(ctx, "coachadmin", "ca@example.invalid", "fixture-password")
	require.NoError(t, e)
	_, e = p.DB.Exec(ctx, `UPDATE users SET role='admin' WHERE id=$1`, admin.ID)
	require.NoError(t, e)
	student, st, e := auth.Register(ctx, "learner", "cs@example.invalid", "fixture-password")
	require.NoError(t, e)
	_, ot, e := auth.Register(ctx, "otherlearner", "co@example.invalid", "fixture-password")
	require.NoError(t, e)
	srv := &httpapi.Server{Platform: p, Store: p, Auth: auth, Log: slog.Default(), Config: config.Config{}}
	handler := srv.Handler()
	call := func(method, path, tok string, body any, code int) []byte {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		require.Equal(t, code, w.Code, w.Body.String())
		return w.Body.Bytes()
	}
	points, e := p.ListKnowledgePoints(ctx)
	require.NoError(t, e)
	require.NotEmpty(t, points)
	point := points[0]
	for _, k := range points {
		if k.Code == "OS.MEMORY" {
			point = k
			break
		}
	}
	overview := func(tok string) domain.LearningOverview {
		var v domain.LearningOverview
		require.NoError(t, json.Unmarshal(call("GET", "/api/v1/exam408/learning/overview", tok, nil, 200), &v))
		return v
	}
	before := overview(st)
	require.True(t, before.NeedsPretest)
	m := domain.LearnerMemory{Key: "preferred", Kind: "preference", Content: "请先解释适用条件，再写公式。"}
	call("PUT", "/api/v1/exam408/learning/memories/preferred", st, m, 200)
	m.Kind = "doubt"
	m.PointID = point.ID
	m.Content = "对这个知识点的前提还不清楚"
	call("PUT", "/api/v1/exam408/learning/memories/doubt", st, m, 200)
	mine := overview(st)
	require.Len(t, mine.Memories, 2)
	require.Empty(t, overview(ot).Memories)
	require.Empty(t, overview(at).Memories)
	for _, v := range mine.Points {
		require.Equal(t, 0, v.Tested, "self-report cannot change mastery")
		if v.ID == point.ID {
			require.Equal(t, 1, v.Doubts)
		}
	}
	m.Content = "现已理解，请在复习时再检查"
	m.Resolved = true
	call("PUT", "/api/v1/exam408/learning/memories/doubt", st, m, 200)
	call("DELETE", "/api/v1/exam408/learning/memories/preferred", st, nil, 200)
	require.Len(t, overview(st).Memories, 1)
	f := domain.FrequencySource{PointID: point.ID, Organization: "合成机构甲", Title: "合成统计报告", URL: "https://example.invalid/report", Excerpt: "合成测试：十个年度中有八个年度涉及这个知识点。", StartYear: 2016, EndYear: 2025, Exams: 10, Appearances: 8}
	call("POST", "/api/v1/admin/frequency-sources", st, f, 403)
	bad := f
	bad.URL = "javascript:alert(1)"
	call("POST", "/api/v1/admin/frequency-sources", at, bad, 400)
	call("POST", "/api/v1/admin/frequency-sources", at, f, 200)
	f.Organization = "合成机构乙"
	f.Appearances = 6
	call("POST", "/api/v1/admin/frequency-sources", at, f, 200)
	f.Organization = "不同统计区间"
	f.StartYear = 2025
	f.Exams = 1
	f.Appearances = 1
	call("POST", "/api/v1/admin/frequency-sources", at, f, 200)
	for _, v := range overview(st).Points {
		if v.ID == point.ID {
			require.InDelta(t, 3.8, v.Importance, .0001)
			require.Len(t, v.Sources, 3)
			require.Contains(t, v.ImportanceBasis, "2家机构")
		}
	}
	asset, e := p.SaveAsset(ctx, admin.ID, domain.Asset{IsPublic: true, Kind: "question", PointID: point.ID, Title: "教学检查合成题", Payload: domain.AssetPayload{Stem: "操作系统负责管理什么？", Options: []string{"系统资源", "天气"}, Answer: 0, AnswerOrigin: "source", ReviewStatus: "published", Explanation: "操作系统负责管理系统资源并提供服务，因此选择系统资源。", Source: "合成测试来源"}})
	require.NoError(t, e)
	require.Equal(t, "ready", asset.Status)
	a, e := p.CreateAssessment(ctx, student.ID, "practice", point.ID, asset.ID)
	require.NoError(t, e)
	require.Equal(t, -1, a.Questions[0].Payload.Answer)
	lesson := domain.ChatResult{Answer: "我们先明确资源管理这个概念，再检查你的理解。", Sources: []domain.Citation{{DocumentTitle: "合成来源", Excerpt: "操作系统负责资源管理。"}}, Generation: &domain.GenerationTrace{Verified: true}}
	var kid string
	require.NoError(t, p.DB.QueryRow(ctx, `SELECT id FROM knowledge_bases WHERE is_system`).Scan(&kid))
	_, e = p.SaveCoachSession(ctx, student.ID, kid, point.ID, "encouraging", a, lesson)
	require.NoError(t, e)
	var sessions []domain.CoachSession
	require.NoError(t, json.Unmarshal(call("GET", "/api/v1/exam408/coaching", st, nil, 200), &sessions))
	require.Len(t, sessions, 1)
	require.Nil(t, sessions[0].Assessment.Result)
	require.Equal(t, -1, sessions[0].Assessment.Questions[0].Payload.Answer)
	require.Empty(t, sessions[0].Assessment.Questions[0].Payload.Explanation)
	var others []domain.CoachSession
	require.NoError(t, json.Unmarshal(call("GET", "/api/v1/exam408/coaching", ot, nil, 200), &others))
	require.Empty(t, others)
	call("POST", "/api/v1/exam408/assessments/"+a.ID+"/submit", ot, map[string]any{"Answers": map[string]int{asset.ID: 0}}, 404)
	for i := 0; i < 2; i++ {
		call("POST", "/api/v1/exam408/assessments/"+a.ID+"/submit", st, map[string]any{"Answers": map[string]int{asset.ID: 0}}, 200)
	}
	after := overview(st)
	for _, v := range after.Points {
		if v.ID == point.ID {
			require.Equal(t, 1, v.Tested)
			require.Equal(t, 1, v.Correct)
			require.Equal(t, "样本不足", v.State)
		}
	}
	require.NoError(t, json.Unmarshal(call("GET", "/api/v1/exam408/coaching", st, nil, 200), &sessions))
	require.NotNil(t, sessions[0].Assessment.Result)
	require.Equal(t, 1, sessions[0].Assessment.Result.Counted)
	// Exercise the actual RAG/provider/verification path with a controlled model,
	// then ensure teaching pauses on a redacted question and resumes idempotently.
	doc, e := p.CreateDocument(ctx, domain.Document{UserID: admin.ID, KnowledgeBaseID: kid, Filename: "coach.txt", OriginalFilename: "内存管理教材.txt", Status: "ready", ResourceType: "textbook"})
	require.NoError(t, e)
	text := "内存管理负责内存资源的分配、回收与保护。内存管理通过地址变换为程序提供内存访问支持。"
	embed := service.LexicalEmbedding{Dimension: 16}
	vectors, e := embed.Embed(ctx, []string{text})
	require.NoError(t, e)
	require.NoError(t, p.ReplaceChunks(ctx, admin.ID, doc.ID, []domain.Chunk{{ID: uuid.NewString(), UserID: admin.ID, DocumentID: doc.ID, KnowledgeBaseID: kid, Content: text, Embedding: vectors[0], Metadata: map[string]any{"embedding_space": p.EmbeddingSpace, "resource_type": "textbook", "page": 1}}}))
	_, e = p.SaveAsset(ctx, admin.ID, domain.Asset{IsPublic: true, Kind: "question", PointID: point.ID, Title: "第二道教学检查题", Payload: domain.AssetPayload{Stem: "内存管理负责什么？", Options: []string{"内存资源分配与保护", "天气预报"}, Answer: 0, AnswerOrigin: "source", ReviewStatus: "published", Explanation: "内存管理负责分配、回收和保护，所以选择内存资源管理。", Source: "合成教材第1页"}})
	require.NoError(t, e)
	modelCalls := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelCalls++
		var body struct{ Messages []struct{ Content string } }
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		last := body.Messages[len(body.Messages)-1].Content
		answer := `{"supported":true}`
		if !strings.Contains(last, `"supported"`) {
			require.Contains(t, last, "轻松、耐心、鼓励")
			answer = `{"claims":[{"text":"内存管理通过分配、回收与保护来管理内存资源。","kind":"explanation","facts":[1]}]}`
		}
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}, "finish_reason": "stop"}}}))
	}))
	defer model.Close()
	srv.Config = config.Config{LLMAPIKey: "fixture", EmbeddingMode: "lexical"}
	srv.RAG = service.RAGService{Store: p, Embedder: embed, LLM: service.OpenAILLM{BaseURL: model.URL, APIKey: "fixture", Model: "fixture"}, Knowledge: service.KnowledgeService{Store: p}, Learning: service.LearningService{Store: p}, TopK: 3, MinScore: .01}
	var coached domain.CoachSession
	require.NoError(t, json.Unmarshal(call("POST", "/api/v1/exam408/coaching", st, map[string]string{"PointID": point.ID, "KnowledgeBaseID": kid, "Teacher": "encouraging"}, 200), &coached))
	require.True(t, coached.Lesson.Generation.Verified)
	require.NotEmpty(t, coached.Lesson.Sources)
	require.Nil(t, coached.Assessment.Result)
	require.Len(t, coached.Assessment.Questions, 1)
	require.Equal(t, -1, coached.Assessment.Questions[0].Payload.Answer)
	require.GreaterOrEqual(t, modelCalls, 2)
	var resumed domain.CoachSession
	require.NoError(t, json.Unmarshal(call("POST", "/api/v1/exam408/coaching", st, map[string]string{"PointID": point.ID, "KnowledgeBaseID": kid, "Teacher": "encouraging"}, 200), &resumed))
	require.Equal(t, coached.ID, resumed.ID)

	call("POST", "/api/v1/exam408/coaching/"+coached.ID+"/explain", st, nil, 409)
	call("POST", "/api/v1/exam408/coaching/"+coached.ID+"/explain", ot, nil, 404)
	qid := coached.Assessment.Questions[0].ID
	call("POST", "/api/v1/exam408/assessments/"+coached.Assessment.ID+"/submit", st, map[string]any{"Answers": map[string]int{qid: 0}}, 200)
	var explanation domain.ChatResult
	require.NoError(t, json.Unmarshal(call("POST", "/api/v1/exam408/coaching/"+coached.ID+"/explain", st, nil, 200), &explanation))
	require.True(t, explanation.Generation.Verified)
	require.NotEmpty(t, explanation.Sources)
	calls := modelCalls
	call("POST", "/api/v1/exam408/coaching/"+coached.ID+"/explain", st, nil, 200)
	require.Equal(t, calls, modelCalls, "validated feedback is cached, not regenerated on resume")

	stored, e := p.Assessment(ctx, student.ID, coached.Assessment.ID)
	require.NoError(t, e)
	require.Equal(t, 1, stored.Result.Correct)
	require.Equal(t, 1, stored.Result.Counted, "teacher explanation must not alter source grading")
	require.NoError(t, p.RecordQuestionDoubts(ctx, student.ID, "为什么需要"+point.Name+"？"))
	pending, e := p.Memories(ctx, student.ID)
	require.NoError(t, e)
	foundDoubt := false
	for _, m := range pending {
		if m.Key == "question:"+point.ID {
			foundDoubt = true
			require.Equal(t, "doubt", m.Kind)
			require.Contains(t, m.Content, "不代表答错")
		}
	}
	require.True(t, foundDoubt)

}
