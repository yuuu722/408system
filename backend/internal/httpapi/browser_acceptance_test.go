package httpapi_test

// Opt-in isolated UI fixture. All answers/providers are synthetic; never reads .env.
import (
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
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type browserParser struct{}

func (browserParser) Parse(context.Context, string, string) (string, error) { return "", nil }
func (browserParser) ParsePages(context.Context, string, string) ([]service.TextPage, error) {
	return []service.TextPage{{Number: 1, Width: 600, Height: 800, CoordinateSpace: "normalized-top-left-displayed-page", Blocks: []domain.DocumentBlock{{ID: "heading", Kind: "paragraph_title", Text: "合成教材：虚拟内存", Box: []float64{.1, .08, .9, .15}}, {ID: "evidence", Kind: "text", Text: "虚拟内存通过地址映射实现程序隔离。页表记录虚拟页到物理页的映射。", Box: []float64{.1, .25, .9, .43}}}}}, nil
}
func TestBrowserAcceptance(t *testing.T) {
	if os.Getenv("BROWSER_ACCEPTANCE") != "1" {
		t.Skip("opt-in temporary browser fixture")
	}
	ctx := context.Background()
	admin, e := repository.Open(ctx, os.Getenv("TEST_DATABASE_URL"))
	require.NoError(t, e)
	defer admin.DB.Close()
	name := "exam408_browser_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, e = admin.DB.Exec(ctx, `CREATE DATABASE `+name)
	require.NoError(t, e)
	uri, e := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	require.NoError(t, e)
	uri.Path = "/" + name
	p, e := repository.Open(ctx, uri.String())
	require.NoError(t, e)
	defer func() {
		p.DB.Close()
		_, e := admin.DB.Exec(ctx, `DROP DATABASE `+name+` WITH(FORCE)`)
		require.NoError(t, e)
	}()
	require.NoError(t, migrations.Up(ctx, p.DB))
	p.EmbeddingSpace = "synthetic"
	auth := service.AuthService{Store: p, Secret: []byte(strings.Repeat("s", 40)), TTL: time.Hour}
	user, _, e := auth.Register(ctx, "browser_review", "browser@example.invalid", "Synthetic-408-Only!")
	require.NoError(t, e)
	kb, e := p.CreateKnowledgeBase(ctx, user.ID, "合成验收资料库", "仅用于工程验收")
	require.NoError(t, e)
	points, e := p.ListKnowledgePoints(ctx)
	require.NoError(t, e)
	point := points[0]
	for _, v := range points {
		if v.Name == "内存管理" {
			point = v
		}
	}
	source := os.Getenv("BROWSER_FIXTURE_PATH")
	require.NotEmpty(t, source)
	d, e := p.CreateDocument(ctx, domain.Document{UserID: user.ID, KnowledgeBaseID: kb.ID, Filename: "synthetic.pdf", OriginalFilename: "合成教材.pdf", StoragePath: source, MimeType: "application/pdf", Status: "uploaded", ResourceType: "notes"})
	require.NoError(t, e)
	docs := &service.DocumentService{Store: p, Parser: browserParser{}, Embedder: syntheticEmbedding{}, EmbeddingSpace: "synthetic", StorageRoot: filepath.Dir(source)}
	require.NoError(t, p.EnqueueDocument(ctx, user.ID, d.ID))
	found, e := p.RunDocumentJob(ctx, docs.Process)
	require.NoError(t, e)
	require.True(t, found)
	_, e = p.SaveAsset(ctx, user.ID, domain.Asset{Kind: "question", PointID: point.ID, Title: "合成内存练习", Payload: domain.AssetPayload{Stem: "虚拟地址是否需要映射？", Options: []string{"需要", "不需要"}, Answer: 0, AnswerOrigin: "source", Difficulty: 1, DifficultyOrigin: "user", Source: "合成样本"}})
	require.NoError(t, e)
	require.NoError(t, p.SaveProfile(ctx, user.ID, domain.Profile{DailyMinutes: 20, GoalScore: 100, ExamDate: "2027-12-01", Stage: "beginner", Preference: "text"}))
	require.NoError(t, p.GenerateTasks(ctx, user.ID))
	hits, e := p.SearchChunks(ctx, user.ID, kb.ID, []float32{1, 0}, 1)
	require.NoError(t, e)
	require.Len(t, hits, 1)
	var regions []domain.SourceRegion
	raw, _ := json.Marshal(hits[0].Metadata["regions"])
	require.NoError(t, json.Unmarshal(raw, &regions))
	rid, _ := hits[0].Metadata["revision_id"].(string)
	citations := []domain.Citation{{DocumentID: d.ID, DocumentTitle: "合成教材.pdf", ChunkID: hits[0].ID, Page: 1, RevisionID: rid, Regions: regions, Excerpt: hits[0].Content, Location: "第1页 · 合成验收", LocationQuality: "blocks"}}
	cid, e := p.CreateConversation(ctx, user.ID, kb.ID, "合成回答排版与高亮验收")
	require.NoError(t, e)
	require.NoError(t, p.SaveTurn(ctx, user.ID, cid, "虚拟内存如何实现地址隔离？", "## 地址映射\n\n虚拟内存通过**页表**实现地址映射。[1]\n\n| 名称 | 含义 |\n|---|---|\n| 虚拟页 | 进程使用的页 |\n| 物理页 | 内存中的页 |\n\n地址示意：$A=p \\times 2^{12}+d$\n\n```text\n虚拟地址 → 页表 → 物理地址\n```\n\n这是合成工程验收回答。", citations))
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := `{"questions":[{"stem":"内存映射的作用是什么？","options":["隔离地址空间","取消地址空间"],"answer":0,"explanation":"合成参考解析，非真实模型结果。","difficulty":2}]}`
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
	}))
	defer model.Close()
	srv := &httpapi.Server{Platform: p, Store: p, Auth: auth, Documents: docs, Config: config.Config{PersonalKBEnabled: true, DemoMode: true}, Log: slog.Default(), Generator: service.QuestionGenerator{Store: p, Embedder: syntheticEmbedding{}, Model: service.OpenAILLM{BaseURL: model.URL, APIKey: "synthetic", Model: "synthetic"}}}
	listener, e := net.Listen("tcp", "127.0.0.1:8080")
	require.NoError(t, e)
	server := &http.Server{Handler: srv.Handler()}
	go server.Serve(listener)
	defer server.Close()
	fmt.Println("BROWSER_FIXTURE_READY http://127.0.0.1:8080 disposable_user=browser_review provider=local_test_double")
	stop := os.Getenv("BROWSER_STOP_FILE")
	require.NotEmpty(t, stop)
	timeout := time.NewTimer(20 * time.Minute)
	defer timeout.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-timeout.C:
			t.Fatal("browser fixture timed out")
		case <-ticker.C:
			if _, e = os.Stat(stop); e == nil {
				return
			}
		}
	}
}
