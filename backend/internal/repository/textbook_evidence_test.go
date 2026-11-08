package repository_test

import (
	"context"
	"github.com/google/uuid"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/repository"
	"github.com/local/exam408-assistant/backend/migrations"
	"github.com/stretchr/testify/require"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestLiteralTextbookRecallFindsCauseAndRespectsScopeAndPurpose(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	ctx := context.Background()
	admin, e := repository.Open(ctx, dsn)
	require.NoError(t, e)
	defer admin.DB.Close()
	name := "exam408_intent_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, e = admin.DB.Exec(ctx, `CREATE DATABASE `+name)
	require.NoError(t, e)
	u, e := url.Parse(dsn)
	require.NoError(t, e)
	u.Path = "/" + name
	p, e := repository.Open(ctx, u.String())
	require.NoError(t, e)
	defer func() {
		p.DB.Close()
		_, err := admin.DB.Exec(ctx, `DROP DATABASE `+name+` WITH(FORCE)`)
		require.NoError(t, err)
	}()
	require.NoError(t, migrations.Up(ctx, p.DB))
	p.EmbeddingSpace = "fixture"
	a, e := p.CreateUser(ctx, domain.User{Username: "a", Email: "a@fixture.invalid", PasswordHash: "fixture"})
	require.NoError(t, e)
	b, e := p.CreateUser(ctx, domain.User{Username: "b", Email: "b@fixture.invalid", PasswordHash: "fixture"})
	require.NoError(t, e)
	ka, e := p.CreateKnowledgeBase(ctx, a.ID, "a", "")
	require.NoError(t, e)
	kb, e := p.CreateKnowledgeBase(ctx, b.ID, "b", "")
	require.NoError(t, e)
	add := func(uid, kid, kind, purpose, text string) domain.Document {
		doc, err := p.CreateDocument(ctx, domain.Document{UserID: uid, KnowledgeBaseID: kid, Filename: uuid.NewString() + ".txt", OriginalFilename: "合成教材.txt", MimeType: "text/plain", StoragePath: "fixture", ResourceType: kind, Purpose: purpose, Status: "ready"})
		require.NoError(t, err)
		require.NoError(t, p.ReplaceChunks(ctx, uid, doc.ID, []domain.Chunk{{ID: uuid.NewString(), DocumentID: doc.ID, UserID: uid, KnowledgeBaseID: kid, Content: text, Embedding: []float32{1, 0}, Metadata: map[string]any{"embedding_space": "fixture", "heading_path": "3.2 虚拟内存的基本概念"}}}))
		return doc
	}
	cause := "传统存储管理具有一次性、驻留性，作业必须全部装入，内存不足时无法运行。"
	expected := add(a.ID, ka.ID, "textbook", "knowledge", cause)
	add(a.ID, ka.ID, "textbook", "knowledge", "虚拟内存基于局部性，按需装入。内存满时置换页面。")
	add(a.ID, ka.ID, "outline", "knowledge", cause)
	add(a.ID, ka.ID, "textbook", "questions", cause)
	add(b.ID, kb.ID, "textbook", "knowledge", cause)
	found, e := p.SearchTextbookEvidence(ctx, a.ID, ka.ID, []string{"虚拟内存"}, []string{"一次性", "驻留性", "全部装入", "局部性"}, 12)
	require.NoError(t, e)
	require.Len(t, found, 2)
	require.Equal(t, expected.ID, found[0].DocumentID)
	for _, c := range found {
		require.Equal(t, ka.ID, c.KnowledgeBaseID)
		require.Equal(t, "textbook", c.Metadata["resource_type"])
	}
	_, e = p.SearchTextbookEvidence(ctx, a.ID, kb.ID, []string{"虚拟内存"}, []string{"一次性"}, 12)
	require.Error(t, e)
	t.Run("PPP purpose recall does not substitute MTU fields or another user's data", func(t *testing.T) {
		purpose := "PPP用于点到点链路，支持多种网络层协议，并提供建立和配置链路的功能。"
		book := add(a.ID, ka.ID, "textbook", "knowledge", purpose)
		add(a.ID, ka.ID, "textbook", "knowledge", "PPP帧首部包含四个字段，信息部分长度受MTU限制。")
		add(a.ID, ka.ID, "outline", "knowledge", purpose)
		add(b.ID, kb.ID, "textbook", "knowledge", purpose)
		rows, err := p.SearchTextbookEvidence(ctx, a.ID, ka.ID, []string{"PPP"}, []string{"用于", "提供", "支持", "功能"}, 12)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, book.ID, rows[0].DocumentID)
	})

	t.Run("comparison prefers body evidence for both objects over broad chapter mentions", func(t *testing.T) {
		add(a.ID, ka.ID, "textbook", "knowledge", "进程参与资源管理、调度、通信和共享，服务器进程还负责建立FTP连接。")
		expected := add(a.ID, ka.ID, "textbook", "knowledge", "进程是资源分配的基本单位，线程是调度的基本单位。")
		rows, e := p.SearchTextbookEvidence(ctx, a.ID, ka.ID, []string{"进程", "线程"}, []string{"比较", "区别", "资源", "调度", "通信", "共享", "基本单位"}, 1)
		require.NoError(t, e)
		require.Len(t, rows, 1)
		require.Equal(t, expected.ID, rows[0].DocumentID)
	})

}
