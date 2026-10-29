package repository_test

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/repository"
	"github.com/local/exam408-assistant/backend/internal/service"
	"github.com/local/exam408-assistant/backend/migrations"
	"github.com/stretchr/testify/require"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type pageParser struct {
	calls   map[int]int
	fail    int
	version string
	name    string
}

func (p *pageParser) Parse(context.Context, string, string) (string, error) {
	return "", fmt.Errorf("use page batches")
}
func (p *pageParser) Fingerprint() string { return p.version }
func (p *pageParser) ParsePageBatch(_ context.Context, _ string, _ string, start, limit int) ([]service.TextPage, int, error) {
	p.calls[start]++
	if p.fail == start {
		p.fail = 0
		return nil, 3, &service.PublicError{Retryable: true, Message: "synthetic page interruption"}
	}
	text := strings.Repeat("分页地址转换保护内存缓存操作系统", 80)
	if start == 1 {
		text = "1. 下列关于" + p.name + "的描述哪个正确？\nA. 地址映射 B. 无需映射\n答案：A\n" + text
	}
	return []service.TextPage{{Number: start, Text: text}}, 3, nil
}

type countEmbedding struct {
	calls, texts int
	failAt       int
}
type zeroCostEmbedding struct{}

func (zeroCostEmbedding) Embed(_ context.Context, text []string) ([][]float32, error) {
	out := make([][]float32, len(text))
	for i := range out {
		out[i] = []float32{1, 0}
	}
	return out, nil
}

func (e *countEmbedding) Embed(_ context.Context, text []string) ([][]float32, error) {
	e.calls++
	e.texts += len(text)
	if e.calls == e.failAt {
		return nil, &service.PublicError{Retryable: true, Message: "synthetic embedding interruption"}
	}
	out := make([][]float32, len(text))
	for i := range out {
		out[i] = []float32{1, 0}
	}
	return out, nil
}

func TestRevisionRecoveryAndFencing(t *testing.T) {
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("TEST_DATABASE_URL enables an isolated random database")
	}
	ctx := context.Background()
	admin, e := repository.Open(ctx, raw)
	require.NoError(t, e)
	defer admin.DB.Close()
	name := "exam408_revision_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, e = admin.DB.Exec(ctx, `CREATE DATABASE `+name)
	require.NoError(t, e)
	u, e := url.Parse(raw)
	require.NoError(t, e)
	u.Path = "/" + name
	p, e := repository.Open(ctx, u.String())
	require.NoError(t, e)
	defer func() {
		p.DB.Close()
		_, e := admin.DB.Exec(ctx, `DROP DATABASE `+name+` WITH(FORCE)`)
		require.NoError(t, e)
	}()
	require.NoError(t, migrations.Up(ctx, p.DB))
	p.EmbeddingSpace = "synthetic"
	user, e := p.CreateUser(ctx, domain.User{Username: "synthetic", Email: "revision@example.invalid", PasswordHash: "not-a-password"})
	require.NoError(t, e)
	kb, e := p.CreateKnowledgeBase(ctx, user.ID, "synthetic", "")
	require.NoError(t, e)
	points, e := p.ListKnowledgePoints(ctx)
	require.NoError(t, e)
	makeDoc := func() domain.Document {
		path := filepath.Join(t.TempDir(), "synthetic.txt")
		require.NoError(t, os.WriteFile(path, []byte("immutable synthetic source"), 0600))
		d, e := p.CreateDocument(ctx, domain.Document{UserID: user.ID, KnowledgeBaseID: kb.ID, Filename: "synthetic.txt", OriginalFilename: "synthetic.txt", Purpose: "both", StoragePath: path, MimeType: "text/plain", Status: "uploaded", ResourceType: "notes"})
		require.NoError(t, e)
		return d
	}
	run := func(fn func(context.Context, domain.Document) error) {
		found, e := p.RunDocumentJob(ctx, fn)
		require.NoError(t, e)
		require.True(t, found)
	}
	wake := func(d domain.Document) {
		_, e := p.DB.Exec(ctx, `UPDATE document_jobs SET available_at=now() WHERE document_id=$1`, d.ID)
		require.NoError(t, e)
	}
	d := makeDoc()
	parser := &pageParser{calls: map[int]int{}, fail: 2, version: "v1", name: points[0].Name}
	embed := &countEmbedding{failAt: 2}
	docs := &service.DocumentService{Store: p, Parser: parser, Embedder: embed, EmbeddingSpace: "synthetic"}
	require.NoError(t, p.EnqueueDocument(ctx, user.ID, d.ID))
	run(docs.Process)
	require.Equal(t, 1, parser.calls[1])
	require.Equal(t, 1, parser.calls[2])
	wake(d)
	run(docs.Process)
	require.Equal(t, 1, parser.calls[1])
	require.Equal(t, 2, parser.calls[2])
	require.Equal(t, 1, parser.calls[3])
	var saved int
	require.NoError(t, p.DB.QueryRow(ctx, `SELECT count(*) FROM revision_chunks WHERE embedding IS NOT NULL`).Scan(&saved))
	require.Equal(t, 4, saved)
	var visible int
	require.NoError(t, p.DB.QueryRow(ctx, `SELECT count(*) FROM learning_assets WHERE document_id=$1`, d.ID).Scan(&visible))
	require.Zero(t, visible)
	wake(d)
	run(docs.Process)
	active, e := p.Document(ctx, user.ID, d.ID)
	require.NoError(t, e)
	require.Equal(t, "ready", active.Status, active.ErrorMessage)
	require.NotEmpty(t, active.ActiveRevisionID)
	require.Equal(t, 1, parser.calls[1])
	var total int
	require.NoError(t, p.DB.QueryRow(ctx, `SELECT count(*) FROM revision_chunks`).Scan(&total))
	require.Equal(t, total+min(4, total-4), embed.texts, "only one failed vector batch should be recalculated")
	parsed, e := p.DocumentStructure(ctx, user.ID, d.ID)
	require.NoError(t, e)
	require.Equal(t, active.ActiveRevisionID, parsed.RevisionID)
	require.NoError(t, p.DB.QueryRow(ctx, `SELECT count(*) FROM learning_assets WHERE document_id=$1 AND status='unavailable' AND payload->>'ReviewStatus'='pending'`, d.ID).Scan(&visible))
	require.Equal(t, 1, visible)

	t.Run("old active remains readable during failed reprocessing", func(t *testing.T) {
		parser.version = "v2"
		parser.fail = 2
		require.NoError(t, p.EnqueueDocument(ctx, user.ID, d.ID))
		run(docs.Process)
		current, e := p.Document(ctx, user.ID, d.ID)
		require.NoError(t, e)
		require.Equal(t, active.ActiveRevisionID, current.ActiveRevisionID)
		parsed, e := p.DocumentStructure(ctx, user.ID, d.ID)
		require.NoError(t, e)
		require.Equal(t, active.ActiveRevisionID, parsed.RevisionID)
		hits, e := p.SearchChunks(ctx, user.ID, kb.ID, []float32{1, 0}, 5)
		require.NoError(t, e)
		require.NotEmpty(t, hits)
		require.Equal(t, active.ActiveRevisionID, hits[0].Metadata["revision_id"])
		wake(d)
		run(docs.Process)
	})
	t.Run("embedding change reuses parsing and chunks", func(t *testing.T) {
		before := parser.calls[1] + parser.calls[2] + parser.calls[3]
		docs.EmbeddingSpace = "synthetic-v2"
		require.NoError(t, p.EnqueueDocument(ctx, user.ID, d.ID))
		run(docs.Process)
		require.Equal(t, before, parser.calls[1]+parser.calls[2]+parser.calls[3])
		docs.EmbeddingSpace = "synthetic"
	})
	t.Run("expired execution cannot write", func(t *testing.T) {
		require.NoError(t, p.EnqueueDocument(ctx, user.ID, d.ID))
		found, e := p.RunDocumentJob(ctx, func(c context.Context, old domain.Document) error {
			_, e := p.DB.Exec(ctx, `UPDATE document_jobs SET lease_until=now()-interval '1 second' WHERE document_id=$1`, old.ID)
			require.NoError(t, e)
			run(docs.Process)
			require.ErrorIs(t, p.RevisionProgress(ctx, old, "failed", 0, 0, 0), repository.ErrStaleExecution)
			return fmt.Errorf("late failure")
		})
		require.True(t, found)
		require.ErrorIs(t, e, repository.ErrStaleExecution)
		current, e := p.Document(ctx, user.ID, d.ID)
		require.NoError(t, e)
		require.Equal(t, "ready", current.Status)
	})
	t.Run("delete invalidates in-flight work", func(t *testing.T) {
		gone := makeDoc()
		require.NoError(t, p.EnqueueDocument(ctx, user.ID, gone.ID))
		found, e := p.RunDocumentJob(ctx, func(c context.Context, old domain.Document) error {
			require.NoError(t, p.DeleteDocument(ctx, user.ID, old.ID))
			return p.RevisionProgress(ctx, old, "embedding", 20, 0, 1)
		})
		require.True(t, found)
		require.ErrorIs(t, e, repository.ErrStaleExecution)
		_, e = p.Document(ctx, user.ID, gone.ID)
		require.ErrorIs(t, e, repository.ErrNotFound)
	})
	t.Run("bounded synthetic pressure", func(t *testing.T) {
		if os.Getenv("RUN_PRESSURE_TEST") != "1" {
			t.Skip("opt in with RUN_PRESSURE_TEST=1")
		}
		ids := []string{}
		for n := 0; n < 8; n++ {
			item := makeDoc()
			require.NoError(t, os.WriteFile(item.StoragePath, []byte(strings.Repeat("虚拟内存分页地址转换缓存保护隔离。", 1000)), 0600))
			ids = append(ids, item.ID)
			require.NoError(t, p.EnqueueDocument(ctx, user.ID, item.ID))
		}
		processor := service.DocumentService{Store: p, Parser: service.Parser{}, Embedder: zeroCostEmbedding{}, EmbeddingSpace: "synthetic-pressure"}
		var peak atomic.Uint64
		stop := make(chan struct{})
		sampled := make(chan struct{})
		go func() {
			defer close(sampled)
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for {
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				if m.HeapAlloc > peak.Load() {
					peak.Store(m.HeapAlloc)
				}
				select {
				case <-stop:
					return
				case <-ticker.C:
				}
			}
		}()
		start := time.Now()
		var group sync.WaitGroup
		failures := make(chan error, 4)
		for i := 0; i < 4; i++ {
			group.Add(1)
			go func() {
				defer group.Done()
				for {
					found, e := p.RunDocumentJob(ctx, processor.Process)
					if e != nil {
						failures <- e
						return
					}
					if !found {
						return
					}
				}
			}()
		}
		group.Wait()
		close(stop)
		<-sampled
		close(failures)
		for e := range failures {
			require.NoError(t, e)
		}
		var ready int
		require.NoError(t, p.DB.QueryRow(ctx, `SELECT count(*) FROM documents WHERE id::text=ANY($1::text[]) AND status='ready'`, ids).Scan(&ready))
		require.Equal(t, len(ids), ready)
		t.Logf("synthetic_pressure documents=%d workers=4 runes_per_doc=%d elapsed_ms=%d peak_process_heap_bytes=%d go=%s cpu=%d provider=local_test_double NOT_REAL_OCR", len(ids), len([]rune(strings.Repeat("虚拟内存分页地址转换缓存保护隔离。", 1000))), time.Since(start).Milliseconds(), peak.Load(), runtime.Version(), runtime.NumCPU())
	})
}
