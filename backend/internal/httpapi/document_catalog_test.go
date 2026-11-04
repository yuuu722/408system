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

}
