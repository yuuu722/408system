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
	"mime/multipart"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCatalogAndQuestionReviewWorkflow(t *testing.T) {
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("isolated PostgreSQL required")
	}
	ctx := context.Background()
	admin, e := repository.Open(ctx, raw)
	require.NoError(t, e)
	defer admin.DB.Close()
	name := "exam408_catalog_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, e = admin.DB.Exec(ctx, `CREATE DATABASE `+name)
	require.NoError(t, e)
	u, e := url.Parse(raw)
	require.NoError(t, e)
	u.Path = "/" + name
	store, e := repository.Open(ctx, u.String())
	require.NoError(t, e)
	defer func() {
		store.DB.Close()
		_, err := admin.DB.Exec(ctx, `DROP DATABASE `+name+` WITH(FORCE)`)
		require.NoError(t, err)
	}()
	require.NoError(t, migrations.Up(ctx, store.DB))
	require.NoError(t, migrations.Up(ctx, store.DB))
	auth := service.AuthService{Store: store, Secret: []byte(strings.Repeat("s", 40)), TTL: time.Hour}
	editor, token, e := auth.Register(ctx, "editor", "editor@example.invalid", "synthetic-password")
	require.NoError(t, e)
	_, e = store.DB.Exec(ctx, `UPDATE users SET role='admin' WHERE id=$1`, editor.ID)
	require.NoError(t, e)
	student, stoken, e := auth.Register(ctx, "student", "student@example.invalid", "synthetic-password")
	require.NoError(t, e)
	kb, e := store.KnowledgeBase(ctx, editor.ID, "40800000-0000-4000-8000-000000000002")
	if e != nil {
		require.NoError(t, store.DB.QueryRow(ctx, `SELECT id FROM knowledge_bases WHERE is_system`).Scan(&kb.ID))
	}
	points, e := store.ListKnowledgePoints(ctx)
	require.NoError(t, e)
	d, e := store.CreateDocument(ctx, domain.Document{UserID: editor.ID, KnowledgeBaseID: kb.ID, OriginalFilename: "操作系统选择题做题本.txt", Filename: "fixture.txt", MimeType: "text/plain", StoragePath: "synthetic", Size: 20, ResourceType: "other", Status: "ready"})
	require.NoError(t, e)
	server := (&httpapi.Server{Platform: store, Store: store, Auth: auth, Log: slog.Default(), Documents: &service.DocumentService{Store: store, StorageRoot: t.TempDir(), MaxBytes: 1 << 20}, Config: config.Config{PersonalKBEnabled: false}}).Handler()
	call := func(method, path, tok string, body any, code int) []byte {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		require.Equal(t, code, w.Code, w.Body.String())
		return w.Body.Bytes()
	}

	var upload bytes.Buffer
	mw := multipart.NewWriter(&upload)
	require.NoError(t, mw.WriteField("resource_type", "past_exam"))
	require.NoError(t, mw.WriteField("purpose", "questions"))
	part, e := mw.CreateFormFile("file", "题目.txt")
	require.NoError(t, e)
	_, e = part.Write([]byte("合成上传题目文件，只验证用途保存，不调用模型。"))
	require.NoError(t, e)
	require.NoError(t, mw.Close())
	req := httptest.NewRequest("POST", "/api/v1/knowledge-bases/"+kb.ID+"/documents", &upload)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	res := httptest.NewRecorder()
	server.ServeHTTP(res, req)
	require.Equal(t, 202, res.Code, res.Body.String())
	var uploaded domain.Document
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &uploaded))
	catalogRows, e := store.Catalogs(ctx, editor.ID, kb.ID)
	require.NoError(t, e)
	foundPurpose := ""
	for _, c := range catalogRows {
		if c.DocumentID == uploaded.ID {
			foundPurpose = c.Purpose
		}
	}
	require.Equal(t, "questions", foundPurpose, "upload purpose must be stored before enqueue")
	path := "/api/v1/admin/documents/" + d.ID
	cat := domain.DocumentCatalog{DocumentID: d.ID, ResourceType: "past_exam", SubjectCode: "os", Purpose: "questions", ReviewState: "confirmed"}
	call("PATCH", path+"/catalog", stoken, cat, 403)
	call("PATCH", path+"/catalog", token, cat, 200)
	store.EmbeddingSpace = "synthetic"
	require.NoError(t, store.ReplaceChunks(ctx, editor.ID, d.ID, []domain.Chunk{{ID: uuid.NewString(), DocumentID: d.ID, UserID: editor.ID, KnowledgeBaseID: kb.ID, Content: "虚拟内存通过按需装入扩展地址空间。", Embedding: []float32{1, 0}, Metadata: map[string]any{"embedding_space": "synthetic"}}}))
	hits, e := store.SearchChunks(ctx, student.ID, kb.ID, []float32{1, 0}, 5)
	require.NoError(t, e)
	require.Empty(t, hits, "question-only file is not answer evidence")
	cat.Purpose = "both"
	call("PATCH", path+"/catalog", token, cat, 200)
	hits, e = store.SearchChunks(ctx, student.ID, kb.ID, []float32{1, 0}, 5)
	require.NoError(t, e)
	require.Len(t, hits, 1)

	pages := []service.TextPage{{Number: 1, Text: "1. 下列关于" + points[0].Name + "的说法哪项正确？\nA. 第一项\nB. 第二项\n答案：A\n解析：合成说明。"}}
	structure := domain.ParsedDocument{Pages: pages}
	require.NoError(t, store.SaveDocumentStructure(ctx, editor.ID, d.ID, structure, service.ExtractQuestions(d, pages, points)))
	var candidates []domain.Asset
	require.NoError(t, json.Unmarshal(call("GET", "/api/v1/exam408/assets", token, nil, 200), &candidates))
	require.Len(t, candidates, 1)
	a := candidates[0]
	require.Equal(t, "pending", a.Payload.ReviewStatus)
	require.Equal(t, "unavailable", a.Status)
	require.JSONEq(t, "[]", string(call("GET", "/api/v1/exam408/assets", stoken, nil, 200)))
	call("POST", path+"/organize", stoken, nil, 403)
	a.Payload.ReviewStatus = "published"
	require.NoError(t, json.Unmarshal(call("PUT", "/api/v1/exam408/assets/"+a.ID, token, a, 200), &a))
	require.Equal(t, "ready", a.Status)
	call("POST", path+"/organize", token, nil, 200)
	fresh, e := store.Asset(ctx, student.ID, a.ID)
	require.NoError(t, e)
	require.Equal(t, "published", fresh.Payload.ReviewStatus)
	require.Equal(t, a.Revision, fresh.Revision, "rebuild must preserve reviewed corrections")
	var visible []domain.Asset
	require.NoError(t, json.Unmarshal(call("GET", "/api/v1/exam408/assets", stoken, nil, 200), &visible))
	require.Len(t, visible, 1)
	cat.Purpose = "knowledge"
	call("PATCH", path+"/catalog", token, cat, 200)
	require.JSONEq(t, "[]", string(call("GET", "/api/v1/exam408/assets", stoken, nil, 200)))
	call("PUT", "/api/v1/exam408/assets/"+a.ID, token, a, 409)
	cat.ResourceType = "other"
	call("PATCH", path+"/catalog", token, cat, 400)
}
