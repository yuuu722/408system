package service

import (
	"context"
	"errors"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/testutil"
	"strings"
	"testing"
)

type progressParser struct{}

func (progressParser) Parse(context.Context, string, string) (string, error) {
	return strings.Repeat("操作系统虚拟内存", 900), nil
}

type progressEmbedder struct {
	store *testutil.Store
	t     *testing.T
	calls int
	fail  bool
}

func (e *progressEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	e.calls++
	d, _ := e.store.Document(context.Background(), "u", "d")
	if d.ProcessingStage != "embedding" || d.TotalChunks <= 4 {
		e.t.Fatal("missing persisted embedding stage/count")
	}
	if e.calls > 1 && (d.ProcessedChunks != 4*(e.calls-1) || d.Progress <= 20 || d.Progress >= 100) {
		e.t.Fatal("incorrect batch progress", d.Progress, d.ProcessedChunks)
	}
	if e.fail && e.calls == 2 {
		return nil, errors.New("test provider failure")
	}
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = []float32{1, 0}
	}
	return out, nil
}
func TestDocumentProgressSuccessAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		store := testutil.NewStore()
		d := domain.Document{ID: "d", UserID: "u", Status: "processing"}
		store.Docs[d.ID] = d
		embed := &progressEmbedder{store: store, t: t, fail: fail}
		s := DocumentService{Store: store, Parser: progressParser{}, Embedder: embed}
		s.Process(context.Background(), d)
		got, _ := store.Document(context.Background(), "u", "d")
		if fail {
			if got.Status != "failed" || got.Progress >= 100 || got.ProcessedChunks != 4 {
				t.Fatal("failure must preserve partial progress", got)
			}
		} else {
			if got.Status != "ready" || got.Progress != 100 || got.ProcessedChunks != got.TotalChunks {
				t.Fatal("incomplete final progress", got)
			}
		}
	}
}
