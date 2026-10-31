package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/local/exam408-assistant/backend/internal/config"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/httpapi"
	"github.com/local/exam408-assistant/backend/internal/repository"
	"github.com/local/exam408-assistant/backend/internal/service"
	"github.com/local/exam408-assistant/backend/migrations"
	"github.com/stretchr/testify/require"
)

const graphEvidence = "虚拟内存与页面置换通过工作集窗口记录最近访问的页面，辅助分析进程的局部性。"

type graphProposal struct{}

func (graphProposal) Answer(context.Context, string, string) (string, error) {
	raw, _ := json.Marshal(map[string]any{
		"entities":  []map[string]any{{"name": "工作集窗口", "entity_type": "mechanism", "description": "记录近期访问的时间窗口", "evidence": graphEvidence, "confidence": .9}, {"name": "虚构无来源实体", "entity_type": "concept", "evidence": graphEvidence, "confidence": .99}},
		"relations": []map[string]any{{"from_point_code": "OS.VIRTUAL", "to_entity_name": "工作集窗口", "relation_type": "uses", "evidence": graphEvidence, "confidence": .9}, {"from_point_code": "OS.VIRTUAL", "to_entity_name": "工作集窗口", "relation_type": "uses", "evidence": "不存在的来源不能进入知识图谱", "confidence": .99}},
	})
	return string(raw), nil
}

// A fresh installation, real PostgreSQL/pgvector, HTTP permissions, actual text
// parsing and durable worker jobs. Model responses are deterministic fixtures;
// these checks establish contracts rather than a commercial model's quality.
func TestPublicCurriculumEndToEnd(t *testing.T) {
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("isolated PostgreSQL required")
	}
	ctx := context.Background()
	admin, e := repository.Open(ctx, raw)
	require.NoError(t, e)
	defer admin.DB.Close()
	name := "exam408_public_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, e = admin.DB.Exec(ctx, `CREATE DATABASE `+name)
	require.NoError(t, e)
	parsed, e := url.Parse(raw)
	require.NoError(t, e)
	parsed.Path = "/" + name
	store, e := repository.Open(ctx, parsed.String())
	require.NoError(t, e)
	defer func() {
		store.DB.Close()
		_, e := admin.DB.Exec(ctx, `DROP DATABASE `+name+` WITH(FORCE)`)
		require.NoError(t, e)
	}()
	require.NoError(t, migrations.Up(ctx, store.DB))
	require.NoError(t, migrations.Up(ctx, store.DB))
	var seeded int
	require.NoError(t, store.DB.QueryRow(ctx, `SELECT count(*) FROM learning_assets WHERE seed_key IS NOT NULL`).Scan(&seeded))
	require.Zero(t, seeded, "fresh installations must contain no sample lessons or exercises")
	// Historical sample content is an explicit test fixture, never startup data.
	fixture, e := os.ReadFile("../../migrations/000014_public_curriculum.up.sql")
	require.NoError(t, e)
	for _, line := range strings.Split(string(fixture), "\n") {
		if strings.HasPrefix(line, "INSERT INTO learning_assets") {
			_, e = store.DB.Exec(ctx, line)
			require.NoError(t, e)
		}
	}
	store.EmbeddingSpace = "synthetic"
	root := t.TempDir()
	rows, e := store.DB.Query(ctx, `SELECT id::text FROM learning_assets WHERE kind='material' AND seed_key IS NOT NULL`)
	require.NoError(t, e)
	var fixtures []string
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		fixtures = append(fixtures, id)
	}
	rows.Close()
	for _, id := range fixtures {
		require.NoError(t, store.SyncPublicMaterial(ctx, root, id))
	}
	require.NoError(t, store.EnsurePublicDocuments(ctx, root))
	require.NoError(t, store.EnsurePublicDocuments(ctx, root))
	auth := service.AuthService{Store: store, Secret: []byte(strings.Repeat("k", 40)), TTL: time.Hour}
	docs := &service.DocumentService{Store: store, Parser: service.Parser{}, Embedder: syntheticEmbedding{}, EmbeddingSpace: "synthetic", StorageRoot: root, MaxBytes: 1 << 20}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := &httpapi.Server{Platform: store, Store: store, Auth: auth, Documents: docs, Config: config.Config{PersonalKBEnabled: true, DemoMode: true, AdminBootstrapToken: strings.Repeat("b", 40)}, Log: logger}
	server.RAG = service.RAGService{Store: store, Embedder: syntheticEmbedding{}, LLM: service.OpenAILLM{DemoMode: true}, Knowledge: service.KnowledgeService{Store: store}, Learning: service.LearningService{Store: store}, TopK: 100, MinScore: .35, Log: logger}
	handler := server.Handler()
	call := func(method, path, token string, body any, status int) []byte {
		t.Helper()
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		require.Equal(t, status, w.Code, w.Body.String())
		return w.Body.Bytes()
	}
	register := func(name string) (domain.User, string) {
		t.Helper()
		b := call("POST", "/api/v1/auth/register", "", map[string]string{"Username": name, "Email": name + "@example.invalid", "Password": "test-password-408"}, 201)
		var v struct {
			User  struct{ ID, Role string }
			Token string
		}
		require.NoError(t, json.Unmarshal(b, &v))
		require.Equal(t, "student", v.User.Role)
		u, e := store.UserByID(ctx, v.User.ID)
		require.NoError(t, e)
		return u, v.Token
	}
	student, token := register("new-student")
	editor, editorToken := register("editor")
	other, otherToken := register("other")
	bases, e := store.ListKnowledgeBases(ctx, student.ID)
	require.NoError(t, e)
	require.Len(t, bases, 1)
	public := bases[0]
	require.True(t, public.IsPublic)
	baseline, e := store.KnowledgeGraph(ctx, student.ID, public.ID)
	require.NoError(t, e)
	require.GreaterOrEqual(t, len(baseline.Nodes), 50)
	require.NotEmpty(t, baseline.Edges)
	points, e := store.ListKnowledgePoints(ctx)
	require.NoError(t, e)
	require.GreaterOrEqual(t, len(points), 50)
	var point domain.KnowledgePoint
	for _, p := range points {
		if p.Code == "OS.VIRTUAL" {
			point = p
		}
	}
	require.NotEmpty(t, point.ID)
	// The student has uploaded nothing but can read lessons, practice and receive a plan.
	assets, e := store.Assets(ctx, student.ID)
	require.NoError(t, e)
	require.Len(t, assets, 76)
	var question, lesson domain.Asset
	answers := map[string]int{}
	for _, a := range assets {
		require.True(t, a.IsPublic)
		if a.Kind == "question" {
			answers[a.ID] = a.Payload.Answer
			question = a
		}
		if a.PointID == point.ID && a.Kind == "material" {
			lesson = a
			require.NotEmpty(t, a.Payload.Stem)
			require.Contains(t, a.Payload.Explanation, "例题")
		}
	}
	var exam domain.Assessment
	require.NoError(t, json.Unmarshal(call("POST", "/api/v1/exam408/assessments", token, map[string]string{"Kind": "diagnostic"}, 200), &exam))
	require.Len(t, exam.Questions, 12)
	selected := map[string]int{}
	for _, q := range exam.Questions {
		require.Equal(t, -1, q.Payload.Answer)
		require.Empty(t, q.Payload.Explanation)
		selected[q.ID] = answers[q.ID]
	}
	var result domain.AssessmentResult
	require.NoError(t, json.Unmarshal(call("POST", "/api/v1/exam408/assessments/"+exam.ID+"/submit", token, map[string]any{"Answers": selected}, 200), &result))
	require.Equal(t, 12, result.Correct)
	call("PUT", "/api/v1/exam408/profile", token, domain.Profile{GoalScore: 110, DailyMinutes: 60, ExamDate: "2027-12-01", Stage: "beginner", Preference: "text"}, 200)
	call("POST", "/api/v1/exam408/tasks/generate", token, nil, 200)
	tasks, e := store.Tasks(ctx, student.ID)
	require.NoError(t, e)
	require.NotEmpty(t, tasks)
	// A locked service identity cannot log in; a real first administrator may still bootstrap once.
	call("POST", "/api/v1/auth/login", "", map[string]string{"Login": "__408_system__", "Password": "!"}, 400)
	call("POST", "/api/v1/admin/bootstrap", editorToken, map[string]string{"Email": editor.Email, "Token": strings.Repeat("b", 40)}, 200)
	call("POST", "/api/v1/admin/bootstrap", editorToken, map[string]string{"Email": other.Email, "Token": strings.Repeat("b", 40)}, 409)
	require.NoError(t, json.Unmarshal(call("GET", "/api/v1/auth/me", editorToken, nil, 200), &struct{ Role string }{}))
	question.Title = "student tampering"
	call("PUT", "/api/v1/exam408/assets/"+question.ID, token, question, 403)
	call("DELETE", "/api/v1/exam408/assets/"+question.ID, token, nil, 404)
	call("POST", "/api/v1/exam408/assets", token, domain.Asset{IsPublic: true, Kind: "question", PointID: point.ID, Title: "Forbidden", Payload: question.Payload}, 403)
	call("DELETE", "/api/v1/knowledge-bases/"+public.ID, token, nil, 404)
	call("PUT", "/api/v1/admin/knowledge-points/"+point.ID, token, point, 403)
	point.Description += " 管理员维护的目录说明。"
	call("PUT", "/api/v1/admin/knowledge-points/"+point.ID, editorToken, point, 200)
	drain := func() {
		t.Helper()
		for n := 0; n < 100; n++ {
			found, e := store.RunDocumentJob(ctx, docs.Process)
			require.NoError(t, e)
			if !found {
				break
			}
		}
		var failed int
		require.NoError(t, store.DB.QueryRow(ctx, `SELECT count(*) FROM document_jobs WHERE state<>'completed'`).Scan(&failed))
		require.Zero(t, failed)
	}
	call("PUT", "/api/v1/exam408/assets/"+lesson.ID, editorToken, lesson, 409)
	unchanged, e := store.Asset(ctx, student.ID, lesson.ID)
	require.NoError(t, e)
	require.Equal(t, lesson.Revision, unchanged.Revision)
	drain() // also proves nil graph provider does not crash document processing
	call("GET", "/api/v1/exam408/knowledge-points/"+point.ID, token, nil, 200)
	// Public lesson edits are re-indexed, rather than leave an old vector behind.
	lesson.Payload.Stem += " 工作集是最近访问页面的集合。"
	var saved domain.Asset
	require.NoError(t, json.Unmarshal(call("PUT", "/api/v1/exam408/assets/"+lesson.ID, editorToken, lesson, 200), &saved))
	require.NotEmpty(t, saved.Payload.DocumentID)
	drain()
	var chat domain.ChatResult
	require.NoError(t, json.Unmarshal(call("POST", "/api/v1/exam408/assistant/chat", token, map[string]any{"KnowledgeBaseID": public.ID, "Question": "虚拟内存与页面置换"}, 200), &chat))
	require.NotEmpty(t, chat.Sources)
	require.NotEmpty(t, chat.ConversationID)
	for _, c := range chat.Sources {
		d, e := store.Document(ctx, student.ID, c.DocumentID)
		require.NoError(t, e)
		kb, e := store.KnowledgeBase(ctx, student.ID, d.KnowledgeBaseID)
		require.NoError(t, e)
		require.True(t, kb.IsPublic)
		call("GET", "/api/v1/documents/"+c.DocumentID+"/file", token, nil, 200)
	}
	upload := func(kid, tok, text string, status int) domain.Document {
		t.Helper()
		var b bytes.Buffer
		mw := multipart.NewWriter(&b)
		require.NoError(t, mw.WriteField("resource_type", "notes"))
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", `form-data; name="file"; filename="synthetic.txt"`)
		h.Set("Content-Type", "text/plain")
		part, e := mw.CreatePart(h)
		require.NoError(t, e)
		_, e = part.Write([]byte(text))
		require.NoError(t, e)
		require.NoError(t, mw.Close())
		req := httptest.NewRequest("POST", "/api/v1/knowledge-bases/"+kid+"/documents", &b)
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		require.Equal(t, status, w.Code, w.Body.String())
		var d domain.Document
		if status == 202 {
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &d))
		}
		return d
	}
	upload(public.ID, token, "学生不能维护公共文档", 403)
	call("POST", "/api/v1/documents/"+saved.Payload.DocumentID+"/reprocess", token, nil, 403)
	call("DELETE", "/api/v1/documents/"+saved.Payload.DocumentID, token, nil, 403)
	docs.GraphLLM = graphProposal{}
	d := upload(public.ID, editorToken, graphEvidence, 202)
	drain()
	graph, e := store.KnowledgeGraph(ctx, student.ID, public.ID)
	require.NoError(t, e)
	var candidate domain.GraphNode
	for _, n := range graph.Nodes {
		require.NotEqual(t, "虚构无来源实体", n.Name)
		if n.Name == "工作集窗口" {
			candidate = n
		}
	}
	require.NotEmpty(t, candidate.ID)
	require.Equal(t, d.ID, candidate.EvidenceDocumentID)
	require.Contains(t, candidate.EvidenceExcerpt, "工作集窗口")
	hasRelation := false
	for _, edge := range graph.Edges {
		if edge.ToEntityID == candidate.ID {
			hasRelation = true
			require.Equal(t, "uses", edge.RelationType)
			require.Equal(t, d.ID, edge.EvidenceDocumentID)
			require.Contains(t, edge.EvidenceExcerpt, "工作集窗口")
		}
	}
	require.True(t, hasRelation)
	before := len(graph.Nodes)
	call("POST", "/api/v1/documents/"+d.ID+"/reprocess", editorToken, nil, 202)
	drain()
	graph, e = store.KnowledgeGraph(ctx, student.ID, public.ID)
	require.NoError(t, e)
	require.Len(t, graph.Nodes, before)
	// A second public copy reuses canonical and candidate node identities.
	upload(public.ID, editorToken, graphEvidence, 202)
	drain()
	shared, e := store.KnowledgeGraph(ctx, student.ID, public.ID)
	require.NoError(t, e)
	require.Len(t, shared.Nodes, before)
	// The personal corpus requires explicit opt-in, and remains invisible to other students.
	private, e := store.CreateKnowledgeBase(ctx, student.ID, "Personal", "")
	require.NoError(t, e)
	own := upload(private.ID, token, "私有蓝鲸缓存策略是本用户的实验笔记，其他用户不得访问。", 202)
	drain()
	call("GET", "/api/v1/documents/"+own.ID+"/file", otherToken, nil, 404)
	call("GET", "/api/v1/knowledge-bases/"+private.ID+"/graph", otherToken, nil, 404)
	call("POST", "/api/v1/exam408/assistant/chat", otherToken, map[string]string{"KnowledgeBaseID": private.ID, "Question": "私有蓝鲸缓存策略"}, 404)
	for _, include := range []bool{false, true} {
		var reply domain.ChatResult
		require.NoError(t, json.Unmarshal(call("POST", "/api/v1/exam408/assistant/chat", token, map[string]any{"KnowledgeBaseID": private.ID, "Question": "私有蓝鲸缓存策略", "IncludePersonal": include}, 200), &reply))
		require.NotEmpty(t, reply.Sources)
		for _, source := range reply.Sources {
			require.Equal(t, own.ID, source.DocumentID)
		}

	}
	after, e := store.KnowledgeGraph(ctx, student.ID, public.ID)
	require.NoError(t, e)
	require.Len(t, after.Nodes, before, "personal uploads must not change the public graph")
	// Default deployment: students can only read public corpora; administrators
	// have separate management APIs and can manage accounts without database edits.
	server.Config.PersonalKBEnabled = false
	call("POST", "/api/v1/knowledge-bases", token, map[string]string{"Name": "Forbidden"}, 403)
	call("GET", "/api/v1/documents/"+own.ID+"/file", token, nil, 403)
	call("GET", "/api/v1/admin/users", token, nil, 403)
	var userPage repository.UserPage
	rawPage := call("GET", "/api/v1/admin/users", editorToken, nil, 200)
	require.NotContains(t, string(rawPage), "PasswordHash")
	require.NoError(t, json.Unmarshal(rawPage, &userPage))
	require.Equal(t, 3, userPage.Total)
	call("PATCH", "/api/v1/admin/users/"+other.ID, editorToken, map[string]any{"Role": "admin", "Disabled": false}, 200)
	call("GET", "/api/v1/admin/users", otherToken, nil, 200)
	call("PATCH", "/api/v1/admin/users/"+other.ID, editorToken, map[string]any{"Role": "student", "Disabled": true}, 200)
	call("GET", "/api/v1/auth/me", otherToken, nil, 401)
	call("POST", "/api/v1/auth/login", "", map[string]string{"Login": other.Email, "Password": "test-password-408"}, 400)
	call("PATCH", "/api/v1/admin/users/"+other.ID, editorToken, map[string]any{"Role": "student", "Disabled": false}, 200)
	call("GET", "/api/v1/auth/me", otherToken, nil, 200)
	call("GET", "/api/v1/admin/users", otherToken, nil, 403)
	call("PATCH", "/api/v1/admin/users/"+editor.ID, editorToken, map[string]any{"Role": "student", "Disabled": true}, 409)
	var managedKB domain.KnowledgeBase
	require.NoError(t, json.Unmarshal(call("POST", "/api/v1/knowledge-bases", editorToken, map[string]string{"Name": "408 新公共专题", "Description": "管理员建立的公共资料"}, 201), &managedKB))
	require.True(t, managedKB.IsPublic)
	call("PATCH", "/api/v1/admin/knowledge-bases/"+managedKB.ID, token, map[string]string{"Name": "Tamper"}, 403)
	call("PATCH", "/api/v1/admin/knowledge-bases/"+managedKB.ID, editorToken, map[string]string{"Name": "408 公共专题更新", "Description": "管理员维护"}, 200)
	call("POST", "/api/v1/admin/knowledge-points", editorToken, domain.KnowledgePoint{SubjectID: point.SubjectID, Code: "OS.ADMIN_TEST", Name: "页框测试目录", Description: "管理员新增的规范目录"}, 200)
	scopedDoc := upload(managedKB.ID, editorToken, graphEvidence, 202)
	drain()
	var scopedReply domain.ChatResult
	require.NoError(t, json.Unmarshal(call("POST", "/api/v1/exam408/assistant/chat", token, map[string]string{"KnowledgeBaseID": managedKB.ID, "Question": "虚拟内存与页面置换"}, 200), &scopedReply))
	require.NotEmpty(t, scopedReply.Sources)
	for _, source := range scopedReply.Sources {
		require.Equal(t, scopedDoc.ID, source.DocumentID, "other public libraries must not leak into the selected corpus")
	}
	call("DELETE", "/api/v1/documents/"+scopedDoc.ID, editorToken, nil, 204)
	// More than the old 20MB boundary is accepted; don't waste time indexing a
	// synthetic repeated blob. Oversize bodies fail before any task is enqueued.
	docs.MaxBytes = 32 << 20
	big := upload(managedKB.ID, editorToken, strings.Repeat("x", 21<<20), 202)
	require.Greater(t, big.Size, int64(20<<20))
	call("DELETE", "/api/v1/documents/"+big.ID, editorToken, nil, 204)
	docs.MaxBytes = 1024
	upload(managedKB.ID, editorToken, strings.Repeat("x", 2048), 413)
	docs.MaxBytes = 32 << 20
	// Real local vectors and rule-based graph definitions need no model fixture.
	docs.GraphLLM = nil
	docs.Embedder = service.LexicalEmbedding{Dimension: 128}
	docs.EmbeddingSpace = "lexical-test-v1"
	store.EmbeddingSpace = docs.EmbeddingSpace
	stalled := upload(managedKB.ID, editorToken, "这是一次因旧模型配置失败而需要批量恢复的公共资料处理测试。", 202)
	require.NoError(t, store.UpdateDocumentStatus(ctx, stalled.ID, "failed", "向量模型尚未配置"))
	_, e = store.DB.Exec(ctx, `UPDATE document_jobs SET state='failed' WHERE document_id=$1`, stalled.ID)
	require.NoError(t, e)
	call("POST", "/api/v1/admin/documents/retry-failed", token, map[string]string{"KnowledgeBaseID": managedKB.ID}, 403)
	require.JSONEq(t, `{"queued":1}`, string(call("POST", "/api/v1/admin/documents/retry-failed", editorToken, map[string]string{"KnowledgeBaseID": managedKB.ID}, 200)))
	drain()
	recovered, e := store.Document(ctx, student.ID, stalled.ID)
	require.NoError(t, e)
	require.Equal(t, "ready", recovered.Status)
	call("DELETE", "/api/v1/documents/"+stalled.ID, editorToken, nil, 204)
	original := "局部窗口：虚拟内存与页面置换使用窗口记录最近访问的页面，以分析进程的局部性。"
	first := upload(managedKB.ID, editorToken, original, 202)
	drain()
	second := upload(managedKB.ID, editorToken, original, 202)
	drain()
	// Delete the newest evidence pointer first; the older file must still support the node.
	first, second = second, first
	findNode := func(name string) (domain.GraphNode, bool) {
		t.Helper()
		g, e := store.KnowledgeGraph(ctx, student.ID, managedKB.ID)
		require.NoError(t, e)
		for _, n := range g.Nodes {
			if n.Name == name {
				return n, true
			}
		}
		return domain.GraphNode{}, false
	}
	node, ok := findNode("局部窗口")
	require.True(t, ok)
	require.Equal(t, "rule_candidate", node.Source)
	call("DELETE", "/api/v1/documents/"+first.ID, editorToken, nil, 204)
	surviving, ok := findNode("局部窗口")
	require.True(t, ok)
	require.Equal(t, node.ID, surviving.ID)
	require.Equal(t, second.ID, surviving.EvidenceDocumentID)
	replace := func(text string, status int) {
		t.Helper()
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		part, e := mw.CreateFormFile("file", "replacement.txt")
		require.NoError(t, e)
		_, e = part.Write([]byte(text))
		require.NoError(t, e)
		require.NoError(t, mw.Close())
		req := httptest.NewRequest("PUT", "/api/v1/documents/"+second.ID+"/file", &body)
		req.Header.Set("Authorization", "Bearer "+editorToken)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		require.Equal(t, status, w.Code, w.Body.String())
	}
	replacement := "帧集合：内存管理维护物理页框列表，跟踪已经分配的页面，并清理失效的记录。"
	replace(replacement, 202)
	require.Equal(t, original, string(call("GET", "/api/v1/documents/"+second.ID+"/file", token, nil, 200)), "until activation citations must still open the old source")
	drain()
	_, ok = findNode("局部窗口")
	require.False(t, ok, "replaced definitions and their edges must disappear")
	overview, e := store.KnowledgePointOverview(ctx, student.ID, point.ID)
	require.NoError(t, e)
	for _, resource := range overview.Resources {
		require.NotEqual(t, second.ID, resource.ID, "replaced file must not keep obsolete automatic point associations")
	}
	updated, ok := findNode("帧集合")
	require.True(t, ok)
	require.Equal(t, second.ID, updated.EvidenceDocumentID)
	require.Equal(t, replacement, string(call("GET", "/api/v1/documents/"+second.ID+"/file", token, nil, 200)))
	graph, e = store.KnowledgeGraph(ctx, student.ID, managedKB.ID)
	require.NoError(t, e)
	require.NotEmpty(t, graph.Edges)
	call("DELETE", "/api/v1/documents/"+second.ID, editorToken, nil, 204)
	graph, e = store.KnowledgeGraph(ctx, student.ID, managedKB.ID)
	require.NoError(t, e)
	require.Empty(t, graph.Nodes)
	require.Empty(t, graph.Edges)

}
