package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type syntheticEmbedding struct{}

func (syntheticEmbedding) Embed(_ context.Context, text []string) ([][]float32, error) {
	out := make([][]float32, len(text))
	for i := range out {
		out[i] = []float32{1, 0}
	}
	return out, nil
}

// The administrative URL is used only to create/drop a new random test database.
// No test data is inserted into the application's database.
func TestFoundationDatabaseAndHTTP(t *testing.T) {
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("set TEST_DATABASE_URL to run isolated PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, e := repository.Open(ctx, raw)
	require.NoError(t, e)
	defer admin.DB.Close()
	name := "exam408_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, e = admin.DB.Exec(ctx, `CREATE DATABASE `+name)
	require.NoError(t, e)
	u, e := url.Parse(raw)
	require.NoError(t, e)
	u.Path = "/" + name
	store, e := repository.Open(ctx, u.String())
	require.NoError(t, e)
	defer func() {
		store.DB.Close()
		_, e = admin.DB.Exec(ctx, `DROP DATABASE `+name+` WITH (FORCE)`)
		require.NoError(t, e)
	}()
	require.NoError(t, migrations.Up(ctx, store.DB))
	require.NoError(t, migrations.Up(ctx, store.DB))
	// This suite exercises personal-only empty-state and grading fixtures; the public curriculum has a dedicated end-to-end suite.
	_, e = store.DB.Exec(ctx, `DELETE FROM learning_assets WHERE seed_key IS NOT NULL`)
	require.NoError(t, e)
	store.EmbeddingSpace = "synthetic"
	auth := service.AuthService{Store: store, Secret: []byte(strings.Repeat("s", 40)), TTL: time.Hour}
	user, token, e := auth.Register(ctx, "synthetic", "synthetic@example.test", "synthetic-password")
	require.NoError(t, e)
	other, otherToken, e := auth.Register(ctx, "other", "other@example.test", "synthetic-password")
	require.NoError(t, e)
	docs := &service.DocumentService{Store: store, Parser: service.Parser{}, Embedder: syntheticEmbedding{}, EmbeddingSpace: "synthetic", StorageRoot: t.TempDir(), MaxBytes: 1 << 20}
	server := &httpapi.Server{Platform: store, Store: store, Auth: auth, Documents: docs, Config: config.Config{PersonalKBEnabled: true, DemoMode: true}, Log: slog.Default()}
	server.RAG = service.RAGService{Store: store, Embedder: syntheticEmbedding{}, LLM: service.OpenAILLM{DemoMode: true}, Knowledge: service.KnowledgeService{Store: store}, Learning: service.LearningService{Store: store}, TopK: 5, MinScore: .35}
	handler := server.Handler()
	call := func(method, path, tok string, body any, status int) []byte {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		require.Equal(t, status, w.Code, w.Body.String())
		return w.Body.Bytes()
	}
	points, e := store.ListKnowledgePoints(ctx)
	require.NoError(t, e)
	t.Run("empty and validation", func(t *testing.T) {
		require.JSONEq(t, "[]", string(call("GET", "/api/v1/exam408/assets", token, nil, 200)))
		call("POST", "/api/v1/exam408/assessments", token, map[string]string{"Kind": "diagnostic"}, 400)
		radar, e := store.Radar(ctx, user.ID)
		require.NoError(t, e)
		for _, s := range radar {
			require.Nil(t, s.Accuracy)
		}
		call("PUT", "/api/v1/exam408/profile", token, domain.Profile{GoalScore: 100, DailyMinutes: 60, ExamDate: "2027-12-01", Stage: "beginner", Preference: "text"}, 200)
	})
	var asset domain.Asset
	t.Run("automatic availability snapshot and grade", func(t *testing.T) {
		asset = domain.Asset{Kind: "question", PointID: points[0].ID, Title: "Synthetic question", Payload: domain.AssetPayload{Stem: "Which value is one?", Options: []string{"one", "two"}, Answer: 0, Explanation: "one means 1", Source: "Synthetic fixture", Difficulty: 1}}
		require.NoError(t, json.Unmarshal(call("POST", "/api/v1/exam408/assets", token, asset, 200), &asset))
		require.Equal(t, "ready", asset.Status)
		call("POST", "/api/v1/exam408/assets/"+asset.ID+"/publish", token, nil, 404)
		var exam domain.Assessment
		require.NoError(t, json.Unmarshal(call("POST", "/api/v1/exam408/assessments", token, map[string]string{"Kind": "diagnostic"}, 200), &exam))
		require.Equal(t, -1, exam.Questions[0].Payload.Answer)
		require.Empty(t, exam.Questions[0].Payload.Explanation)
		asset.Payload.Answer = 1
		require.NoError(t, json.Unmarshal(call("PUT", "/api/v1/exam408/assets/"+asset.ID, token, asset, 200), &asset))
		require.Equal(t, "ready", asset.Status)
		call("POST", "/api/v1/exam408/assessments/"+exam.ID+"/submit", otherToken, map[string]any{"Answers": map[string]int{asset.ID: 0}}, 404)
		var result domain.AssessmentResult
		require.NoError(t, json.Unmarshal(call("POST", "/api/v1/exam408/assessments/"+exam.ID+"/submit", token, map[string]any{"Answers": map[string]int{asset.ID: 0}}, 200), &result))
		require.Equal(t, 1, result.Correct)
		call("POST", "/api/v1/exam408/assessments/"+exam.ID+"/submit", token, map[string]any{"Answers": map[string]int{asset.ID: 1}}, 200)
		var n int
		require.NoError(t, store.DB.QueryRow(ctx, `SELECT count(*) FROM exam408_learning_events WHERE user_id=$1 AND event_type='quiz_correct'`, user.ID).Scan(&n))
		require.Equal(t, 1, n)
		exam, e = store.CreateAssessment(ctx, user.ID, "practice", "")
		require.NoError(t, e)
		_, e = store.SubmitAssessment(ctx, user.ID, exam.ID, map[string]int{asset.ID: 0})
		require.NoError(t, e)
		radar, e := store.Radar(ctx, user.ID)
		require.NoError(t, e)
		tested := 0
		for _, s := range radar {
			tested += s.Tested
		}
		require.Equal(t, 1, tested)
		require.NoError(t, store.GenerateTasks(ctx, user.ID))
		require.NoError(t, store.GenerateTasks(ctx, user.ID))
		tasks, e := store.Tasks(ctx, user.ID)
		require.NoError(t, e)
		require.Len(t, tasks, 1)
		require.Error(t, store.CompleteTask(ctx, other.ID, tasks[0].ID))
		require.NoError(t, store.CompleteTask(ctx, user.ID, tasks[0].ID))
		_, e = store.CreateAssessment(ctx, user.ID, "retest", "")
		require.Error(t, e)
	})
	t.Run("AI excluded until reversible personal confirmation", func(t *testing.T) {
		unknown := domain.Asset{Kind: "question", PointID: points[1].ID, Title: "No answer", Payload: domain.AssetPayload{Stem: "Synthetic question?", Options: []string{"one", "two"}, Answer: -1, Difficulty: 1, Source: "Synthetic"}}
		require.NoError(t, json.Unmarshal(call("POST", "/api/v1/exam408/assets", token, unknown, 200), &unknown))
		require.Equal(t, "ready", unknown.Status)
		exam, e := store.CreateAssessment(ctx, user.ID, "practice", "", unknown.ID)
		require.NoError(t, e)
		r, e := store.SubmitAssessment(ctx, user.ID, exam.ID, map[string]int{unknown.ID: -1})
		require.NoError(t, e)
		require.Equal(t, 0, r.Counted)
		require.Equal(t, 0, r.Correct)
		r, e = store.SaveAIAnswer(ctx, user.ID, exam.ID, unknown.ID, 0, "Synthetic model inference")
		require.NoError(t, e)
		require.Equal(t, 0, r.Counted)
		route := "/api/v1/exam408/assessments/" + exam.ID + "/questions/" + unknown.ID + "/confirm"
		call("POST", route, otherToken, map[string]string{"Decision": "correct"}, 404)
		call("POST", route, token, map[string]string{"Decision": "bad"}, 400)
		for _, decision := range []string{"correct", "correct", "wrong", "uncertain", "correct", "uncertain"} {
			require.NoError(t, json.Unmarshal(call("POST", route, token, map[string]string{"Decision": decision}, 200), &r))
			var n int
			require.NoError(t, store.DB.QueryRow(ctx, `SELECT count(*) FROM exam408_learning_events WHERE user_id=$1 AND metadata->>'question_id'=$2`, user.ID, unknown.ID).Scan(&n))
			want := 1
			if decision == "uncertain" {
				want = 0
			}
			require.Equal(t, want, n)
			require.Equal(t, want, r.Counted)
			m, e := store.ListMastery(ctx, user.ID, false)
			require.NoError(t, e)
			for _, v := range m {
				if v.KnowledgePointID == unknown.PointID {
					require.NotEqual(t, "uncertain", decision)
				}
			}
		}
		unavailable := unknown
		unavailable.ID = ""
		unavailable.Payload.Options = nil
		require.NoError(t, json.Unmarshal(call("POST", "/api/v1/exam408/assets", token, unavailable, 200), &unavailable))
		require.Equal(t, "unavailable", unavailable.Status)
		_, e = store.CreateAssessment(ctx, user.ID, "practice", "", unavailable.ID)
		require.Error(t, e)
	})
	kb, e := store.CreateKnowledgeBase(ctx, user.ID, "Synthetic KB", "")
	require.NoError(t, e)
	path := filepath.Join(docs.StorageRoot, "synthetic.txt")
	require.NoError(t, os.WriteFile(path, []byte("虚拟内存通过地址映射实现程序隔离并提供独立地址空间。"), 0600))
	doc, e := store.CreateDocument(ctx, domain.Document{UserID: user.ID, KnowledgeBaseID: kb.ID, Filename: "synthetic.txt", OriginalFilename: "synthetic.txt", MimeType: "text/plain", StoragePath: path, Status: "uploaded", ResourceType: "notes", Size: 90})
	require.NoError(t, e)
	t.Run("durable retry and replay", func(t *testing.T) {
		require.NoError(t, store.EnqueueDocument(ctx, user.ID, doc.ID))
		found, e := store.RunDocumentJob(ctx, func(c context.Context, d domain.Document) error {
			return &service.PublicError{Retryable: true, Message: "synthetic transient"}
		})
		require.NoError(t, e)
		require.True(t, found)
		var attempts int
		var state string
		require.NoError(t, store.DB.QueryRow(ctx, `SELECT attempts,state FROM document_jobs WHERE document_id=$1`, doc.ID).Scan(&attempts, &state))
		require.Equal(t, 1, attempts)
		require.Equal(t, "queued", state)
		_, e = store.DB.Exec(ctx, `UPDATE document_jobs SET available_at=now() WHERE document_id=$1`, doc.ID)
		require.NoError(t, e)
		work, cancel := context.WithCancel(ctx)
		found, e = store.RunDocumentJob(work, func(c context.Context, d domain.Document) error {
			second, err := store.RunDocumentJob(ctx, docs.Process)
			require.NoError(t, err)
			require.False(t, second)
			cancel()
			return c.Err()
		})
		require.Error(t, e)
		require.True(t, found)
		found, e = store.RunDocumentJob(ctx, docs.Process)
		require.NoError(t, e)
		require.True(t, found)
		d, e := store.Document(ctx, user.ID, doc.ID)
		require.NoError(t, e)
		require.Equal(t, "ready", d.Status)
		call("GET", "/api/v1/documents/"+doc.ID+"/file", otherToken, nil, 404)
		call("GET", "/api/v1/documents/"+doc.ID+"/file", token, nil, 200)
	})
	t.Run("stream history and ownership", func(t *testing.T) {
		output := call("POST", "/api/v1/exam408/assistant/stream", token, map[string]string{"KnowledgeBaseID": kb.ID, "Question": "虚拟内存是什么？"}, 200)
		require.Contains(t, string(output), "event: delta")
		require.Contains(t, string(output), "event: done")
		conversations, e := store.ListConversations(ctx, user.ID)
		require.NoError(t, e)
		require.Len(t, conversations, 1)
		cid := conversations[0].ID
		messages, e := store.ConversationMessages(ctx, user.ID, cid, kb.ID)
		require.NoError(t, e)
		require.Len(t, messages, 2)
		call("GET", "/api/v1/conversations/"+cid+"/messages", otherToken, nil, 404)
		kb2, e := store.CreateKnowledgeBase(ctx, user.ID, "Another KB", "")
		require.NoError(t, e)
		_, e = server.RAG.Chat(ctx, user.ID, kb2.ID, cid, "它有什么作用？")
		require.Error(t, e)
		_, e = server.RAG.Chat(ctx, user.ID, kb.ID, cid, "它有什么作用？")
		require.NoError(t, e)
		messages, e = store.ConversationMessages(ctx, user.ID, cid, kb.ID)
		require.NoError(t, e)
		require.Len(t, messages, 4)
		call("PATCH", "/api/v1/conversations/"+cid, token, map[string]string{"Title": "renamed"}, 200)
		call("DELETE", "/api/v1/conversations/"+cid, token, nil, 200)
	})
	t.Run("automatic extraction ownership replay and deletion", func(t *testing.T) {
		source := filepath.Join(docs.StorageRoot, "questions.txt")
		require.NoError(t, os.WriteFile(source, []byte("1. 下列关于"+points[0].Name+"的说法哪一个正确？\nA. 第一项描述 B. 第二项描述\n答案：A\n解析：这是工程合成样本。"), 0600))
		d, e := store.CreateDocument(ctx, domain.Document{UserID: user.ID, KnowledgeBaseID: kb.ID, Filename: "questions.txt", OriginalFilename: "questions.txt", Purpose: "both", StoragePath: source, MimeType: "text/plain", Status: "uploaded", ResourceType: "notes"})
		require.NoError(t, e)
		for i := 0; i < 2; i++ {
			require.NoError(t, store.EnqueueDocument(ctx, user.ID, d.ID))
			found, e := store.RunDocumentJob(ctx, docs.Process)
			require.NoError(t, e)
			require.True(t, found)
		}
		parsed, e := store.DocumentStructure(ctx, user.ID, d.ID)
		require.NoError(t, e)
		require.Len(t, parsed.Pages, 1)
		call("GET", "/api/v1/documents/"+d.ID+"/structure", otherToken, nil, 404)
		var n int
		require.NoError(t, store.DB.QueryRow(ctx, `SELECT count(*) FROM learning_assets WHERE document_id=$1 AND status='unavailable' AND payload->>'ReviewStatus'='pending'`, d.ID).Scan(&n))
		require.Equal(t, 1, n)
		require.NoError(t, store.DeleteDocument(ctx, user.ID, d.ID))
		require.NoError(t, store.DB.QueryRow(ctx, `SELECT count(*) FROM learning_assets WHERE document_id=$1`, d.ID).Scan(&n))
		require.Zero(t, n)
	})
	t.Run("budgeted plan remains stable", func(t *testing.T) {
		require.NoError(t, store.GenerateTasks(ctx, user.ID))
		before, e := store.DailyPlan(ctx, user.ID)
		require.NoError(t, e)
		require.LessOrEqual(t, before.AllocatedMinutes, before.BudgetMinutes)
		require.NoError(t, store.GenerateTasks(ctx, user.ID))
		after, e := store.DailyPlan(ctx, user.ID)
		require.NoError(t, e)
		require.Equal(t, before, after)
		call("GET", "/api/v1/exam408/tasks/plan", token, nil, 200)
		var snapshots int
		require.NoError(t, store.DB.QueryRow(ctx, `SELECT count(*) FROM assessment_result_changes WHERE user_id=$1`, user.ID).Scan(&snapshots))
		require.Greater(t, snapshots, 5)
	})
	t.Run("generated practice provenance and idempotency", func(t *testing.T) {
		calls := 0
		generatedStem := "内存地址是否需要映射？"
		model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			content := `{"questions":[{"stem":"内存地址是否需要映射？","options":["需要映射","不需要映射"],"answer":0,"explanation":"工程合成参考解析。","difficulty":2}]}`
			content = strings.Replace(content, "内存地址是否需要映射？", generatedStem, 1)
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
		}))
		defer model.Close()
		empty, e := store.CreateKnowledgeBase(ctx, user.ID, "Empty synthetic", "")
		require.NoError(t, e)
		server.Generator = service.QuestionGenerator{Store: store, Embedder: syntheticEmbedding{}, Model: service.OpenAILLM{BaseURL: model.URL, APIKey: "synthetic", Model: "synthetic"}}
		in := service.GenerationRequest{RequestID: uuid.NewString(), KnowledgeBaseID: empty.ID, PointID: points[0].ID, Count: 1}
		_, e = server.Generator.Generate(ctx, user.ID, in)
		require.Error(t, e)
		require.Zero(t, calls)
		in.RequestID = uuid.NewString()
		in.AllowUngrounded = true
		generated, e := server.Generator.Generate(ctx, user.ID, in)
		require.NoError(t, e)
		require.Len(t, generated, 1)
		require.Equal(t, "ai", generated[0].Payload.AnswerOrigin)
		require.Contains(t, generated[0].Payload.Source, "缺少检索资料")
		repeated, e := server.Generator.Generate(ctx, user.ID, in)
		require.NoError(t, e)
		require.Equal(t, generated[0].ID, repeated[0].ID)
		require.Equal(t, 1, calls)
		in.RequestID = uuid.NewString()
		duplicates, e := server.Generator.Generate(ctx, user.ID, in)
		require.NoError(t, e)
		require.Equal(t, generated[0].ID, duplicates[0].ID)
		_, e = server.Generator.Generate(ctx, other.ID, in)
		require.Error(t, e)
		exam, e := store.CreateAssessment(ctx, user.ID, "practice", "", generated[0].ID)
		require.NoError(t, e)
		result, e := store.SubmitAssessment(ctx, user.ID, exam.ID, map[string]int{generated[0].ID: 0})
		require.NoError(t, e)
		require.Zero(t, result.Counted)
		result, e = store.ConfirmAnswer(ctx, user.ID, exam.ID, generated[0].ID, "correct")
		require.NoError(t, e)
		require.Equal(t, 1, result.Counted)
		result, e = store.ConfirmAnswer(ctx, user.ID, exam.ID, generated[0].ID, "uncertain")
		require.NoError(t, e)
		require.Zero(t, result.Counted)
		variant := generated[0]
		variant.ID = ""
		variant.Payload.ContentHash = ""
		variant.Payload.Stem += "（合成变式）"
		variant, e = store.SaveAsset(ctx, user.ID, variant)
		require.NoError(t, e)
		check, e := store.CreateAssessment(ctx, user.ID, "diagnostic", points[0].ID)
		require.NoError(t, e)
		familyCount := 0
		for _, q := range check.Questions {
			if domain.Family(q) == domain.Family(generated[0]) {
				familyCount++
			}
		}
		require.Equal(t, 1, familyCount)
		retest, e := store.CreateAssessment(ctx, user.ID, "retest", points[0].ID)
		if e == nil {
			for _, q := range retest.Questions {
				require.NotEqual(t, domain.Family(generated[0]), domain.Family(q))
			}
		}
		second, e := store.CreateAssessment(ctx, user.ID, "practice", "", variant.ID)
		require.NoError(t, e)
		_, e = store.SubmitAssessment(ctx, user.ID, second.ID, map[string]int{variant.ID: 1})
		require.NoError(t, e)
		_, e = store.ConfirmAnswer(ctx, user.ID, second.ID, variant.ID, "wrong")
		require.NoError(t, e)
		radarBefore, e := store.Radar(ctx, user.ID)
		require.NoError(t, e)
		_, e = store.ConfirmAnswer(ctx, user.ID, exam.ID, generated[0].ID, "correct")
		require.NoError(t, e)
		radarAfter, e := store.Radar(ctx, user.ID)
		require.NoError(t, e)
		require.Equal(t, radarBefore, radarAfter, "late confirmation cannot overrule a newer same-family answer")
		_, e = store.ConfirmAnswer(ctx, user.ID, second.ID, variant.ID, "uncertain")
		require.NoError(t, e)
		for i := 0; i < 5; i++ {
			_, _, e := store.ReserveGeneration(ctx, other.ID, uuid.NewString(), "synthetic")
			require.NoError(t, e)
		}
		_, _, e = store.ReserveGeneration(ctx, other.ID, uuid.NewString(), "synthetic")
		var limit *domain.RequestError
		require.ErrorAs(t, e, &limit)
		require.Equal(t, 429, limit.Status)
		generatedStem = "虚拟内存如何隔离进程地址空间？"
		for _, point := range points {
			if point.Name == "内存管理" {
				in.PointID = point.ID
			}
		}
		in.RequestID = uuid.NewString()
		in.KnowledgeBaseID = kb.ID
		in.AllowUngrounded = false
		grounded, e := server.Generator.Generate(ctx, user.ID, in)
		require.NoError(t, e)
		require.NotEmpty(t, grounded[0].Payload.Evidence)
		require.NotContains(t, grounded[0].Payload.Source, "缺少检索资料")
	})
	fmt.Println("isolated PostgreSQL foundation acceptance passed")
}
