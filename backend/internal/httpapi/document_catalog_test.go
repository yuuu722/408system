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
	"mime/multipart"
	"net/http"
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

	t.Run("Neo4j outage keeps saved graph readable", func(t *testing.T) {
		unavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
		defer unavailable.Close()
		store.Neo4j = &repository.Neo4jGraph{URL: unavailable.URL, Username: "neo4j", Password: "synthetic"}
		defer func() { store.Neo4j = nil }()
		// Older imported graphs may only have the direct evidence FK.
		_, err := store.DB.Exec(ctx, `DELETE FROM knowledge_entity_evidence WHERE chunk_id IN (SELECT id FROM document_chunks WHERE document_id=$1)`, d.ID)
		require.NoError(t, err)
		var recovered domain.KnowledgeGraph
		require.NoError(t, json.Unmarshal(call("GET", "/api/v1/knowledge-bases/"+kb.ID+"/graph", token, nil, 200), &recovered))
		require.Equal(t, "postgres-recovery", recovered.Storage)
		require.NotEmpty(t, recovered.Warning)
		require.NotEmpty(t, recovered.Nodes)
	})
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
	t.Run("member quotas do not expose private libraries", func(t *testing.T) {
		call("PATCH", "/api/v1/admin/users/"+student.ID+"/quota", stoken, map[string]int{"PersonalKBLimit": 0}, 403)
		call("PATCH", "/api/v1/admin/users/"+student.ID+"/quota", token, map[string]int{"PersonalKBLimit": 1}, 200)
		private, e := store.CreateKnowledgeBase(ctx, student.ID, "private", "secret description")
		require.NoError(t, e)
		_, e = store.CreateKnowledgeBase(ctx, student.ID, "another", "")
		require.Error(t, e)
		bases, e := store.ListKnowledgeBases(ctx, editor.ID)
		require.NoError(t, e)
		for _, base := range bases {
			require.NotEqual(t, private.ID, base.ID)
		}
		call("POST", "/api/v1/knowledge-bases/"+kb.ID+"/catalog/batch", stoken, map[string]any{"DocumentIDs": []string{d.ID}, "ResourceType": "textbook", "SubjectCode": "os", "Purpose": "knowledge"}, 403)
		var result struct {
			Updated int
			Failed  map[string]string
		}
		require.NoError(t, json.Unmarshal(call("POST", "/api/v1/knowledge-bases/"+kb.ID+"/catalog/batch", token, map[string]any{"DocumentIDs": []string{d.ID}, "ResourceType": "textbook", "SubjectCode": "os", "Purpose": "both"}, 200), &result))
		require.Equal(t, 1, result.Updated)
		require.Empty(t, result.Failed)
		list, e := store.ManageUsers(ctx, editor.ID, "student", 0)
		require.NoError(t, e)
		require.Len(t, list.Users, 1)
		require.Equal(t, 1, list.Users[0].PersonalKBCount)
	})
	t.Run("promoted admin does not browse its former personal libraries", func(t *testing.T) {
		u, e := store.CreateUser(ctx, domain.User{Username: "promoted", Email: "promoted@example.invalid", PasswordHash: "synthetic"})
		require.NoError(t, e)
		personal, e := store.CreateKnowledgeBase(ctx, u.ID, "former private", "")
		require.NoError(t, e)
		privateDoc, e := store.CreateDocument(ctx, domain.Document{UserID: u.ID, KnowledgeBaseID: personal.ID, Filename: "legacy.txt", OriginalFilename: "private.txt", ResourceType: "notes", Status: "ready"})
		require.NoError(t, e)
		_, e = store.DB.Exec(ctx, `UPDATE users SET role='admin' WHERE id=$1`, u.ID)
		require.NoError(t, e)
		bases, e := store.ListKnowledgeBases(ctx, u.ID)
		require.NoError(t, e)
		for _, base := range bases {
			require.True(t, base.IsPublic)
			require.NotEqual(t, personal.ID, base.ID)
		}
		_, e = store.KnowledgeBase(ctx, u.ID, personal.ID)
		require.Error(t, e)
		_, e = store.Document(ctx, u.ID, privateDoc.ID)
		require.Error(t, e)
		_, e = store.ListDocuments(ctx, u.ID, personal.ID)
		require.Error(t, e)
		require.Error(t, store.CanMaintainDocument(ctx, u.ID, privateDoc.ID))
		_, e = store.UpdatePersonalKnowledgeBase(ctx, u.ID, personal.ID, "no-edit", "")
		require.Error(t, e)
		require.Error(t, store.DeleteKnowledgeBase(ctx, u.ID, personal.ID))
		_, e = store.CreateKnowledgeBase(ctx, u.ID, "hidden private", "")
		require.Error(t, e)
	})
	t.Run("public textbooks supplement selected personal library", func(t *testing.T) {
		publicText := "虚拟内存通过按需调入提高内存利用率，允许程序分多次装入。"
		publicDoc, e := store.CreateDocument(ctx, domain.Document{UserID: editor.ID, KnowledgeBaseID: kb.ID, Filename: "textbook.txt", OriginalFilename: "教材.txt", ResourceType: "textbook", Status: "ready"})
		require.NoError(t, e)
		require.NoError(t, store.ReplaceChunks(ctx, editor.ID, publicDoc.ID, []domain.Chunk{{ID: uuid.NewString(), DocumentID: publicDoc.ID, KnowledgeBaseID: kb.ID, UserID: editor.ID, Content: publicText, Embedding: []float32{1, 0}, Metadata: map[string]any{"page": 1, "embedding_space": store.EmbeddingSpace}}}))
		var personalID string
		require.NoError(t, store.DB.QueryRow(ctx, `SELECT id FROM knowledge_bases WHERE user_id=$1 AND NOT is_public LIMIT 1`, student.ID).Scan(&personalID))
		personalDoc, e := store.CreateDocument(ctx, domain.Document{UserID: student.ID, KnowledgeBaseID: personalID, Filename: "notes.txt", OriginalFilename: "笔记.txt", ResourceType: "notes", Status: "ready"})
		require.NoError(t, e)
		require.NoError(t, store.ReplaceChunks(ctx, student.ID, personalDoc.ID, []domain.Chunk{{ID: uuid.NewString(), DocumentID: personalDoc.ID, KnowledgeBaseID: personalID, UserID: student.ID, Content: "虚拟内存通过按需调入提高内存利用率。", Embedding: []float32{1, 0}, Metadata: map[string]any{"page": 1, "embedding_space": store.EmbeddingSpace}}}))
		rag := service.RAGService{Store: store, Embedder: syntheticEmbedding{}, LLM: service.OpenAILLM{DemoMode: true}, Knowledge: service.KnowledgeService{Store: store}, Learning: service.LearningService{Store: store}, TopK: 1, MinScore: .35}
		result, e := rag.Chat(ctx, student.ID, personalID, "", "为什么需要虚拟内存？")
		require.NoError(t, e)
		require.Len(t, result.Sources, 1)
		require.NotEqual(t, personalDoc.ID, result.Sources[0].DocumentID)
		chosenDoc, e := store.Document(ctx, student.ID, result.Sources[0].DocumentID)
		require.NoError(t, e)
		require.Equal(t, kb.ID, chosenDoc.KnowledgeBaseID)
		require.Equal(t, "textbook", chosenDoc.ResourceType)
		_, e = rag.Chat(ctx, editor.ID, personalID, "", "为什么需要虚拟内存？")
		require.Error(t, e)
	})
	t.Run("solution links require exact source and review", func(t *testing.T) {
		cat.ResourceType = "exercise"
		cat.Purpose = "both"
		call("PATCH", path+"/catalog", token, cat, 200)
		quote := "这是内存知识点的完整解析原文，第一项成立。"
		source, e := store.CreateDocument(ctx, domain.Document{UserID: editor.ID, KnowledgeBaseID: kb.ID, OriginalFilename: "解析.txt", Filename: "solution.txt", ResourceType: "solution", Status: "ready"})
		require.NoError(t, e)
		require.NoError(t, store.SaveDocumentStructure(ctx, editor.ID, source.ID, domain.ParsedDocument{Pages: []service.TextPage{{Number: 2, Text: quote}}}, nil))
		route := "/api/v1/exam408/assets/" + a.ID + "/link-solution"
		call("POST", route, stoken, map[string]any{"DocumentID": source.ID, "Page": 2, "Quote": quote}, 403)
		call("POST", route, token, map[string]any{"DocumentID": source.ID, "Page": 2, "Quote": "这段话不在教材原文里。"}, 400)
		var linked domain.Asset
		require.NoError(t, json.Unmarshal(call("POST", route, token, map[string]any{"DocumentID": source.ID, "Page": 2, "Quote": quote, "Answer": 0}, 200), &linked))
		require.Equal(t, "pending", linked.Payload.ReviewStatus)
		require.Equal(t, "unavailable", linked.Status)
		require.Equal(t, source.ID, linked.Payload.Evidence[0].DocumentID)
		linked.Payload.ReviewStatus = "published"
		require.NoError(t, json.Unmarshal(call("PUT", "/api/v1/exam408/assets/"+linked.ID, token, linked, 200), &linked))
		related, e := store.RelatedQuestions(ctx, student.ID, kb.ID, []string{linked.PointID})
		require.NoError(t, e)
		require.Len(t, related, 1)
	})

	t.Run("confirmed source subject and repeated relations update graph without OCR", func(t *testing.T) {
		var chunkID string
		require.NoError(t, store.DB.QueryRow(ctx, `SELECT id FROM document_chunks WHERE document_id=$1 LIMIT 1`, d.ID).Scan(&chunkID))
		first, second := uuid.NewString(), uuid.NewString()
		for i, id := range []string{first, second} {
			_, e := store.DB.Exec(ctx, `INSERT INTO knowledge_entities(id,user_id,knowledge_base_id,name,entity_type,evidence_chunk_id,source,confidence,description) VALUES($1,$2,$3,$4,'concept',$5,'rule_candidate',0.9,'用于验证图谱归类的合成概念。')`, id, editor.ID, kb.ID, fmt.Sprintf("合成%d页表", i), chunkID)
			require.NoError(t, e)
		}
		for i := 0; i < 2; i++ {
			_, e := store.DB.Exec(ctx, `INSERT INTO knowledge_relations(user_id,knowledge_base_id,from_entity_id,to_entity_id,relation_type,evidence_chunk_id,source,confidence) VALUES($1,$2,$3,$4,'related',$5,'rule_candidate',0.9)`, editor.ID, kb.ID, first, second, chunkID)
			require.NoError(t, e)
		}
		cat.ResourceType = "exercise"
		cat.Purpose = "both"
		cat.SubjectCode = "os"
		require.NoError(t, store.SaveCatalog(ctx, editor.ID, cat))
		graph, e := store.KnowledgeGraph(ctx, student.ID, kb.ID)
		require.NoError(t, e)
		for _, n := range graph.Nodes {
			if n.ID == first {
				require.Equal(t, "os", n.SubjectCode)
			}
		}
		count := 0
		for _, edge := range graph.Edges {
			if edge.FromEntityID == first && edge.ToEntityID == second {
				count++
				require.Equal(t, 2, edge.EvidenceCount)
			}
		}
		require.Equal(t, 1, count)
		cat.SubjectCode = "cn"
		require.NoError(t, store.SaveCatalog(ctx, editor.ID, cat))
		graph, e = store.KnowledgeGraph(ctx, student.ID, kb.ID)
		require.NoError(t, e)
		for _, n := range graph.Nodes {
			if n.ID == first {
				require.Equal(t, "cn", n.SubjectCode)
			}
		}
	})

	t.Run("neighboring textbook context stays within chapter and permission scope", func(t *testing.T) {
		book, e := store.CreateDocument(ctx, domain.Document{UserID: editor.ID, KnowledgeBaseID: kb.ID, Filename: "neighbor.txt", OriginalFilename: "教材邻接测试.txt", ResourceType: "textbook", Status: "ready"})
		require.NoError(t, e)
		chunks := []domain.Chunk{}
		for i, text := range []string{"文件系统用于组织文件目录。", "传统存储管理要求一次性装入，内存不足时作业无法运行。", "虚拟内存允许分多次调入内存。"} {
			heading := "内存管理"
			if i == 0 {
				heading = "文件管理"
			}
			chunks = append(chunks, domain.Chunk{ID: uuid.NewString(), DocumentID: book.ID, KnowledgeBaseID: kb.ID, UserID: editor.ID, ChunkIndex: i, Content: text, Embedding: []float32{1, 0}, Score: 1, Metadata: map[string]any{"page": 224 + i, "heading_path": heading, "embedding_space": store.EmbeddingSpace}})
		}
		require.NoError(t, store.ReplaceChunks(ctx, editor.ID, book.ID, chunks))
		neighbors, e := store.ExpandTextbookContext(ctx, student.ID, kb.ID, []domain.Chunk{chunks[2]})
		require.NoError(t, e)
		require.Len(t, neighbors, 1)
		require.Equal(t, chunks[1].ID, neighbors[0].ID)
		require.Contains(t, neighbors[0].Content, "无法运行")
		require.Equal(t, float64(225), neighbors[0].Metadata["page"])
	})

	t.Run("separate answer files preview then link without publishing", func(t *testing.T) {
		q, e := store.CreateDocument(ctx, domain.Document{UserID: editor.ID, KnowledgeBaseID: kb.ID, Filename: "q.txt", OriginalFilename: "测试卷题目.txt", ResourceType: "exercise", Purpose: "questions", Status: "ready"})
		require.NoError(t, e)
		a, e := store.CreateDocument(ctx, domain.Document{UserID: editor.ID, KnowledgeBaseID: kb.ID, Filename: "a.txt", OriginalFilename: "测试卷答案.txt", ResourceType: "solution", Purpose: "both", Status: "ready"})
		require.NoError(t, e)
		qparsed := domain.ParsedDocument{Pages: []service.TextPage{{Number: 1, Text: "第一章 练习\n1. 下列" + points[0].Name + "哪个正确？\nA. 甲 B. 乙"}}}
		aparsed := domain.ParsedDocument{Pages: []service.TextPage{{Number: 3, Text: "第一章 参考答案\n1. B\n解析：这是本题的完整来源解释。"}}}
		require.NoError(t, store.SaveDocumentStructure(ctx, editor.ID, q.ID, qparsed, service.ExtractQuestions(q, qparsed.Pages, points)))
		require.NoError(t, store.SaveDocumentStructure(ctx, editor.ID, a.ID, aparsed, nil))
		route := "/api/v1/documents/" + q.ID + "/answer-file"
		input := map[string]any{"AnswerDocumentID": a.ID}
		call("POST", route, stoken, input, 403)
		var preview struct {
			Fingerprint string
			Matches     []struct {
				AssetID string
				Answer  int
			}
			Applied int
		}
		require.NoError(t, json.Unmarshal(call("POST", route, token, input, 200), &preview))
		require.Len(t, preview.Matches, 1)
		require.Equal(t, 1, preview.Matches[0].Answer)
		input["Apply"] = true
		input["Fingerprint"] = "outdated"
		call("POST", route, token, input, 409)
		input["Fingerprint"] = preview.Fingerprint
		input["OnlyIncomplete"] = true
		call("POST", route, token, input, 409) // cannot change batch mode after preview
		input["OnlyIncomplete"] = false
		var result struct{ Applied int }
		require.NoError(t, json.Unmarshal(call("POST", route, token, input, 200), &result))
		require.Equal(t, 1, result.Applied)
		linked, e := store.Asset(ctx, editor.ID, preview.Matches[0].AssetID)
		require.NoError(t, e)
		require.Equal(t, 1, linked.Payload.Answer)
		require.Equal(t, "pending", linked.Payload.ReviewStatus)
		require.Equal(t, "unavailable", linked.Status)
		require.Equal(t, a.ID, linked.Payload.Evidence[0].DocumentID)
		require.Equal(t, 3, linked.Payload.Evidence[0].Page)
		call("POST", route, token, input, 409)
		// Batch mode must not overwrite a reliable but conflicting answer.
		_, e = store.DB.Exec(ctx, `UPDATE learning_assets SET payload=jsonb_set(jsonb_set(payload,'{Answer}','0'),'{Explanation}','"1.A"') WHERE id=$1`, linked.ID)
		require.NoError(t, e)
		input = map[string]any{"AnswerDocumentID": a.ID, "OnlyIncomplete": true}
		var protected struct {
			Matches []any
			Skipped []struct{ Reason string }
		}
		require.NoError(t, json.Unmarshal(call("POST", route, token, input, 200), &protected))
		require.Empty(t, protected.Matches)
		require.Len(t, protected.Skipped, 1)
		require.Contains(t, protected.Skipped[0].Reason, "冲突")
		// Inventory excludes image bytes/evidence and is restricted to admins.
		_, e = store.DB.Exec(ctx, `UPDATE learning_assets SET payload=jsonb_set(payload,'{Images}','["PRIVATE_IMAGE_BYTES"]') WHERE id=$1`, linked.ID)
		require.NoError(t, e)
		inventoryRoute := "/api/v1/knowledge-bases/" + kb.ID + "/question-inventory"
		call("GET", inventoryRoute, stoken, nil, 403)
		rawInventory := call("GET", inventoryRoute, token, nil, 200)
		require.NotContains(t, string(rawInventory), "PRIVATE_IMAGE_BYTES")
		var inventory []domain.Asset
		require.NoError(t, json.Unmarshal(rawInventory, &inventory))
		found := false
		for _, row := range inventory {
			if row.ID == linked.ID {
				found = true
				require.Equal(t, []string{"present"}, row.Payload.Images)
				require.Empty(t, row.Payload.Evidence)
			}
		}
		require.True(t, found)
		var personalID string
		require.NoError(t, store.DB.QueryRow(ctx, `SELECT id FROM knowledge_bases WHERE user_id=$1 AND NOT is_public LIMIT 1`, student.ID).Scan(&personalID))
		call("GET", "/api/v1/knowledge-bases/"+personalID+"/question-inventory", token, nil, 404)
	})
	t.Run("automatic publication uses same-book source explanation and never approves gaps", func(t *testing.T) {
		doc, err := store.CreateDocument(ctx, domain.Document{UserID: editor.ID, KnowledgeBaseID: kb.ID, Filename: "auto.txt", OriginalFilename: "自动整理习题.txt", ResourceType: "exercise", Purpose: "questions", Status: "ready"})
		require.NoError(t, err)
		parsed := domain.ParsedDocument{Pages: []service.TextPage{{Number: 1, Text: "第一章 内存管理\n1. 虚拟内存的特点是什么？\nA. 按需装入 B. 全部常驻\n2. 虚拟内存采用哪种方式？\nA. 一次性 B. 按需装入\n参考答案与解析\n1. A【解析】虚拟内存利用局部性原理，允许程序分多次调入内存运行。\n2. B"}}}
		require.NoError(t, store.SaveDocumentStructure(ctx, editor.ID, doc.ID, parsed, service.ExtractQuestions(doc, parsed.Pages, points)))
		route := "/api/v1/documents/" + doc.ID + "/publish-verified"
		call("POST", route, stoken, nil, 403)
		var result struct {
			Published int
			Skipped   []any
		}
		require.NoError(t, json.Unmarshal(call("POST", route, token, nil, 200), &result))
		require.Equal(t, 1, result.Published)
		require.Len(t, result.Skipped, 1)
		require.NoError(t, json.Unmarshal(call("POST", route, token, nil, 200), &result))
		require.Zero(t, result.Published, "rerun cannot republish or increment existing approved questions")
		rows, err := store.DocumentQuestionAssets(ctx, editor.ID, doc.ID)
		require.NoError(t, err)
		require.Len(t, rows, 2)
		for _, a := range rows {
			if a.Payload.QuestionNumber == "1" {
				require.Equal(t, "ready", a.Status)
			} else {
				require.NotEqual(t, "ready", a.Status)
			}
		}
		// A forged explanation which merely has enough words does not pass source verification.
		_, err = store.DB.Exec(ctx, `UPDATE learning_assets SET payload=jsonb_set(payload,'{Explanation}','"这是一段没有原文依据的伪造完整解析，不应发布。"') WHERE document_id=$1 AND payload->>'QuestionNumber'='2'`, doc.ID)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(call("POST", route, token, nil, 200), &result))
		require.Zero(t, result.Published)
		// Source changes between validation and commit must abort publication.
		var pending domain.Asset
		rows, err = store.DocumentQuestionAssets(ctx, editor.ID, doc.ID)
		require.NoError(t, err)
		for _, a := range rows {
			if a.Payload.QuestionNumber == "2" {
				pending = a
			}
		}
		pending.Payload.ReviewStatus = "published"
		pending.Payload.Issue = ""
		changed := parsed
		changed.Pages = append([]service.TextPage(nil), parsed.Pages...)
		changed.Pages[0].Text += "\n来源变化"
		rawChanged, _ := json.Marshal(changed)
		_, err = store.DB.Exec(ctx, `UPDATE document_parses SET payload=$2 WHERE document_id=$1`, doc.ID, rawChanged)
		require.NoError(t, err)
		_, err = store.PublishVerifiedQuestions(ctx, editor.ID, doc, []domain.Asset{pending}, map[string]repository.QuestionSourceSnapshot{doc.ID: {Document: doc, Parsed: parsed}})
		require.Error(t, err)
	})

	t.Run("different answer filename and number resolve by complete content then automatically publish", func(t *testing.T) {
		q, err := store.CreateDocument(ctx, domain.Document{UserID: editor.ID, KnowledgeBaseID: kb.ID, Filename: "contentq.txt", OriginalFilename: "独立题册.txt", ResourceType: "exercise", Purpose: "questions", Status: "ready"})
		require.NoError(t, err)
		source, err := store.CreateDocument(ctx, domain.Document{UserID: editor.ID, KnowledgeBaseID: kb.ID, Filename: "contenta.txt", OriginalFilename: "另一个书名的习题详解.txt", ResourceType: "solution", Purpose: "knowledge", Status: "ready"})
		require.NoError(t, err)
		body := "下列关于虚拟内存管理的叙述中，正确的是\nA. 允许按需调入内存 B. 所有程序必须全部常驻内存"
		qp := domain.ParsedDocument{Pages: []service.TextPage{{Number: 1, Text: "1. " + body}}}
		ap := domain.ParsedDocument{Pages: []service.TextPage{{Number: 5, Text: "87. " + body + "\n【答案】A\n【解析】虚拟内存利用局部性原理，允许程序分多次调入内存执行。"}}}
		require.NoError(t, store.SaveDocumentStructure(ctx, editor.ID, q.ID, qp, service.ExtractQuestions(q, qp.Pages, points)))
		require.NoError(t, store.SaveDocumentStructure(ctx, editor.ID, source.ID, ap, nil))
		route := "/api/v1/documents/" + q.ID
		call("POST", route+"/resolve-answers", stoken, nil, 403)
		var resolved struct{ Resolved int }
		require.NoError(t, json.Unmarshal(call("POST", route+"/resolve-answers", token, nil, 200), &resolved))
		require.Equal(t, 1, resolved.Resolved)
		var publication struct{ Published int }
		require.NoError(t, json.Unmarshal(call("POST", route+"/publish-verified", token, nil, 200), &publication))
		require.Equal(t, 1, publication.Published)
		rows, err := store.DocumentQuestionAssets(ctx, editor.ID, q.ID)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, "ready", rows[0].Status)
		require.Equal(t, source.ID, rows[0].Payload.Evidence[0].DocumentID)
		require.Equal(t, 5, rows[0].Payload.Evidence[0].Page)
		require.Equal(t, 0, rows[0].Payload.Answer)
	})

	t.Run("auto detect includes knowledge-only other books and leaves non-question documents unchanged", func(t *testing.T) {
		makeDoc := func(filename, text string) domain.Document {
			d, err := store.CreateDocument(ctx, domain.Document{UserID: editor.ID, KnowledgeBaseID: kb.ID, Filename: filename, OriginalFilename: filename, ResourceType: "other", Purpose: "knowledge", Status: "ready"})
			require.NoError(t, err)
			require.NoError(t, store.SaveCatalog(ctx, editor.ID, domain.DocumentCatalog{DocumentID: d.ID, ResourceType: "other", SubjectCode: "os", Purpose: "knowledge", ReviewState: "pending"}))
			parsed := domain.ParsedDocument{Pages: []service.TextPage{{Number: 1, Text: text}}}
			require.NoError(t, store.SaveDocumentStructure(ctx, editor.ID, d.ID, parsed, nil))
			return d
		}
		doc := makeDoc("2026操作系统.pdf", "1. 虚拟内存的特点是什么？\nA. 按需调入 B. 全部常驻\n答案：A\n解析：程序可以按需分多次调入内存，而不必全部驻留。")
		var result struct {
			Candidates int
			Catalog    domain.DocumentCatalog
		}
		require.NoError(t, json.Unmarshal(call("POST", "/api/v1/documents/"+doc.ID+"/organize", token, map[string]any{"AutoDetect": true}, 200), &result))
		require.Equal(t, 1, result.Candidates)
		require.Equal(t, "both", result.Catalog.Purpose)
		require.Equal(t, "other", result.Catalog.ResourceType)
		require.Equal(t, "os", result.Catalog.SubjectCode)
		note := makeDoc("只有知识讲解.pdf", "本节说明虚拟内存的基本概念，没有编号选择题。")
		require.NoError(t, json.Unmarshal(call("POST", "/api/v1/documents/"+note.ID+"/organize", token, map[string]any{"AutoDetect": true}, 200), &result))
		require.Zero(t, result.Candidates)
		require.Equal(t, "knowledge", result.Catalog.Purpose)
		route := "/api/v1/documents/" + doc.ID + "/question-diagnostics"
		call("GET", route, stoken, nil, 403)
		var diagnostic struct {
			FreshCandidates, AnswerEntries int
			Samples                        []struct{ Text string }
		}
		require.NoError(t, json.Unmarshal(call("GET", route, token, nil, 200), &diagnostic))
		require.Equal(t, 1, diagnostic.FreshCandidates)
		require.NotEmpty(t, diagnostic.Samples)
		require.Contains(t, diagnostic.Samples[0].Text, "虚拟内存")
		var privateID string
		require.NoError(t, store.DB.QueryRow(ctx, `SELECT id FROM documents WHERE user_id=$1 AND knowledge_base_id IN (SELECT id FROM knowledge_bases WHERE NOT is_public) LIMIT 1`, student.ID).Scan(&privateID))
		call("GET", "/api/v1/documents/"+privateID+"/question-diagnostics", token, nil, 404)
	})

	t.Run("saved workbook reference links same edition original book despite different filenames", func(t *testing.T) {
		q, err := store.CreateDocument(ctx, domain.Document{UserID: editor.ID, KnowledgeBaseID: kb.ID, Filename: "workbook.txt", OriginalFilename: "【横版】26王道操作系统选择题做题本.pdf", ResourceType: "exercise", Purpose: "questions", Status: "ready"})
		require.NoError(t, err)
		qp := domain.ParsedDocument{Pages: []service.TextPage{{Number: 1, Text: "第 1 章基础\n1.1 基础概念（答案见原书 P6）\n01. 虚拟内存的特点是什么？\nA. 按需装入 B. 全部常驻"}}}
		extracted := service.ExtractQuestions(q, qp.Pages, points)
		require.Len(t, extracted, 1)
		require.NoError(t, store.SaveDocumentStructure(ctx, editor.ID, q.ID, qp, extracted))
		makeSource := func(filename, year, answer string) domain.Document {
			d, err := store.CreateDocument(ctx, domain.Document{UserID: editor.ID, KnowledgeBaseID: kb.ID, Filename: filename, OriginalFilename: filename, ResourceType: "other", Purpose: "knowledge", Status: "ready"})
			require.NoError(t, err)
			ap := domain.ParsedDocument{Pages: []service.TextPage{{Number: 2, Text: year + " 年操作系统考研复习指导 / 王道论坛组编"}, {Number: 3, Text: "1.1.4 本节习题精选\n01. 虚拟内存的特点是什么？\nA. 按需装入 B. 全部常驻\n1.1.5 答案与解析\n1. " + answer + "\n解析：虚拟内存利用局部性原理，允许按需分多次调入程序。"}}}
			require.NoError(t, store.SaveDocumentStructure(ctx, editor.ID, d.ID, ap, nil))
			return d
		}
		source := makeSource("2026操作系统.pdf", "2026", "A")
		copySource := makeSource("【PDF】2026操作系统.pdf", "2026", "A")
		makeSource("2027操作系统.pdf", "2027", "B")
		route := "/api/v1/documents/" + q.ID
		var result struct{ Resolved, Ambiguous int }
		require.NoError(t, json.Unmarshal(call("POST", route+"/resolve-answers", token, nil, 200), &result))
		require.Equal(t, 1, result.Resolved)
		require.Zero(t, result.Ambiguous, "identical copies must not block reliable pairing")
		var publication struct{ Published int }
		require.NoError(t, json.Unmarshal(call("POST", route+"/publish-verified", token, nil, 200), &publication))
		require.Equal(t, 1, publication.Published)
		rows, err := store.DocumentQuestionAssets(ctx, editor.ID, q.ID)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, "ready", rows[0].Status)
		require.Equal(t, 0, rows[0].Payload.Answer)
		require.Contains(t, []string{source.ID, copySource.ID}, rows[0].Payload.Evidence[0].DocumentID)
		require.Contains(t, rows[0].Payload.Explanation, "局部性原理")
	})

	t.Run("directory links include ancestors without claiming textbook evidence", func(t *testing.T) {
		var child domain.KnowledgePoint
		for _, point := range points {
			if point.ParentID != "" {
				child = point
				break
			}
		}
		require.NotEmpty(t, child.ID)
		d, e := store.CreateDocument(ctx, domain.Document{UserID: editor.ID, KnowledgeBaseID: kb.ID, Filename: "dir.txt", OriginalFilename: "目录关系测试.txt", ResourceType: "textbook", Status: "ready"})
		require.NoError(t, e)
		text := child.Name + "用于本次合成测试中的明确知识点识别。"
		require.NoError(t, store.ReplaceChunks(ctx, editor.ID, d.ID, []domain.Chunk{{ID: uuid.NewString(), DocumentID: d.ID, KnowledgeBaseID: kb.ID, UserID: editor.ID, Content: text, Embedding: []float32{1, 0}, Metadata: map[string]any{"embedding_space": store.EmbeddingSpace}}}))
		graph, e := store.KnowledgeGraph(ctx, student.ID, kb.ID)
		require.NoError(t, e)
		found := false
		for _, edge := range graph.Edges {
			if edge.Source == "canonical_directory" && edge.RelationType == "part_of" {
				found = true
				require.Empty(t, edge.EvidenceChunkID)
				require.Contains(t, edge.EvidenceExcerpt, "不是上传文档")
			}
		}
		require.True(t, found)
	})

	t.Run("confirmed subject survives newer unclassified evidence and cross-subject is explicit", func(t *testing.T) {
		sourceDocs := []domain.Document{}
		for i := 0; i < 3; i++ {
			d, e := store.CreateDocument(ctx, domain.Document{UserID: editor.ID, KnowledgeBaseID: kb.ID, Filename: fmt.Sprintf("s%d.txt", i), OriginalFilename: fmt.Sprintf("来源%d.txt", i), ResourceType: "textbook", Status: "ready"})
			require.NoError(t, e)
			sourceDocs = append(sourceDocs, d)
			require.NoError(t, store.ReplaceChunks(ctx, editor.ID, d.ID, []domain.Chunk{{ID: uuid.NewString(), DocumentID: d.ID, KnowledgeBaseID: kb.ID, UserID: editor.ID, Content: "这是用于检验来源分类的独立证据原文。", Embedding: []float32{1, 0}, Metadata: map[string]any{"embedding_space": store.EmbeddingSpace}}}))
		}
		require.NoError(t, store.SaveCatalog(ctx, editor.ID, domain.DocumentCatalog{DocumentID: sourceDocs[0].ID, ResourceType: "textbook", SubjectCode: "os", Purpose: "knowledge", ReviewState: "confirmed"}))
		ids := []string{}
		for _, d := range sourceDocs {
			var id string
			require.NoError(t, store.DB.QueryRow(ctx, `SELECT id FROM document_chunks WHERE document_id=$1`, d.ID).Scan(&id))
			ids = append(ids, id)
		}
		entity := uuid.NewString()
		_, e := store.DB.Exec(ctx, `INSERT INTO knowledge_entities(id,user_id,knowledge_base_id,name,entity_type,source,confidence,evidence_chunk_id,description) VALUES($1,$2,$3,'来源分类测试概念','concept','rule_candidate',0.9,$4,'用于验证来源共识的合成概念。')`, entity, editor.ID, kb.ID, ids[1])
		require.NoError(t, e)
		_, e = store.DB.Exec(ctx, `INSERT INTO knowledge_entity_evidence(entity_id,chunk_id) VALUES($1,$2),($1,$3)`, entity, ids[0], ids[1])
		require.NoError(t, e)
		graph, e := store.KnowledgeGraph(ctx, student.ID, kb.ID)
		require.NoError(t, e)
		for _, n := range graph.Nodes {
			if n.ID == entity {
				require.Equal(t, "os", n.SubjectCode)
				require.Equal(t, "catalog_confirmed", n.SubjectState)
				require.Equal(t, sourceDocs[0].ID, n.EvidenceDocumentID)
			}
		}
		require.NoError(t, store.SaveCatalog(ctx, editor.ID, domain.DocumentCatalog{DocumentID: sourceDocs[2].ID, ResourceType: "textbook", SubjectCode: "co", Purpose: "knowledge", ReviewState: "confirmed"}))
		_, e = store.DB.Exec(ctx, `INSERT INTO knowledge_entity_evidence(entity_id,chunk_id) VALUES($1,$2)`, entity, ids[2])
		require.NoError(t, e)
		graph, e = store.KnowledgeGraph(ctx, student.ID, kb.ID)
		require.NoError(t, e)
		for _, n := range graph.Nodes {
			if n.ID == entity {
				require.Empty(t, n.SubjectCode)
				require.Equal(t, "cross_subject", n.SubjectState)
				require.ElementsMatch(t, []string{"os", "co"}, n.SubjectCodes)
			}
		}
	})

	t.Run("split layout extracted review then publish makes a student practice", func(t *testing.T) {
		doc, err := store.CreateDocument(ctx, domain.Document{UserID: editor.ID, KnowledgeBaseID: kb.ID, OriginalFilename: "分行选择题.txt", Filename: "split.txt", MimeType: "text/plain", StoragePath: "synthetic", ResourceType: "exercise", Status: "ready"})
		require.NoError(t, err)
		c := domain.DocumentCatalog{DocumentID: doc.ID, ResourceType: "exercise", SubjectCode: "os", Purpose: "questions", ReviewState: "confirmed"}
		require.NoError(t, store.SaveCatalog(ctx, editor.ID, c))
		parsed := domain.ParsedDocument{Pages: []domain.StructuredPage{{Number: 1, Text: "（１）\n虚拟内存的特点是什么？\nＡ\n按需装入\nＢ\n全部常驻\nＣ\n不使用外存\nＤ\n不支持调入\n答案：A\n解析：虚拟内存允许按需装入。"}}}
		require.NoError(t, store.SaveDocumentStructure(ctx, editor.ID, doc.ID, parsed, nil))
		var organized struct {
			Candidates int `json:"candidates"`
		}
		require.NoError(t, json.Unmarshal(call("POST", "/api/v1/documents/"+doc.ID+"/organize", token, nil, 200), &organized))
		require.Equal(t, 1, organized.Candidates)
		candidates, err := store.DocumentQuestionAssets(ctx, editor.ID, doc.ID)
		require.NoError(t, err)
		require.Len(t, candidates, 1)
		candidate := candidates[0]
		require.Equal(t, "pending", candidate.Payload.ReviewStatus)
		require.NotEqual(t, "ready", candidate.Status)
		require.Equal(t, 0, candidate.Payload.Answer)
		require.Empty(t, candidate.Payload.Issue)
		var visible []domain.Asset
		require.NoError(t, json.Unmarshal(call("GET", "/api/v1/exam408/assets", stoken, nil, 200), &visible))
		for _, a := range visible {
			require.NotEqual(t, candidate.ID, a.ID)
		}
		candidate.Title = "人工核对的分行选择题"
		require.NoError(t, json.Unmarshal(call("PUT", "/api/v1/exam408/assets/"+candidate.ID, token, candidate, 200), &candidate))
		require.NoError(t, store.RefreshQuestionCandidates(ctx, editor.ID, doc, parsed, nil))
		reviewed, err := store.Asset(ctx, editor.ID, candidate.ID)
		require.NoError(t, err)
		require.Equal(t, "人工核对的分行选择题", reviewed.Title)
		candidate = reviewed
		candidate.Payload.ReviewStatus = "published"
		require.NoError(t, json.Unmarshal(call("PUT", "/api/v1/exam408/assets/"+candidate.ID, token, candidate, 200), &candidate))
		require.Equal(t, "ready", candidate.Status)
		var exam domain.Assessment
		require.NoError(t, json.Unmarshal(call("POST", "/api/v1/exam408/assessments", stoken, map[string]string{"Kind": "practice", "AssetID": candidate.ID}, 200), &exam))
		require.Len(t, exam.Questions, 1)
		var result domain.AssessmentResult
		require.NoError(t, json.Unmarshal(call("POST", "/api/v1/exam408/assessments/"+exam.ID+"/submit", stoken, map[string]any{"Answers": map[string]int{candidate.ID: 0}}, 200), &result))
		require.Equal(t, 1, result.Counted)
		require.Equal(t, 1, result.Correct)
		require.NoError(t, store.RefreshQuestionCandidates(ctx, editor.ID, doc, parsed, nil))
		retained, err := store.Asset(ctx, editor.ID, candidate.ID)
		require.NoError(t, err)
		require.Equal(t, "ready", retained.Status, "organize with zero extraction must not delete reviewed published questions")
	})

}
