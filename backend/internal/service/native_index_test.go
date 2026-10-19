package service_test

import (
	"context"
	"github.com/local/exam408-assistant/backend/internal/service"
	"github.com/stretchr/testify/require"
	"math"
	"strings"
	"testing"
)

func TestLocalLexicalVectors(t *testing.T) {
	p := service.OpenAIEmbedding{Dimension: 1024, LocalLexical: true}
	vectors, e := p.Embed(context.Background(), []string{"虚拟内存分页管理", "虚拟内存分页管理使用页表", "计算机网络数据链路协议"})
	require.NoError(t, e)
	require.Len(t, vectors, 3)
	cosine := func(a, b []float32) float64 {
		var result float64
		for i := range a {
			result += float64(a[i] * b[i])
		}
		return result
	}
	require.Greater(t, cosine(vectors[0], vectors[1]), cosine(vectors[0], vectors[2]))
	require.InDelta(t, 1, cosine(vectors[0], vectors[0]), .00001)
	for _, v := range vectors {
		for _, x := range v {
			require.False(t, math.IsNaN(float64(x)))
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e = p.Embed(ctx, []string{"正文"})
	require.ErrorIs(t, e, context.Canceled)
}
func TestIntegratedTextAndScannedPDF(t *testing.T) {
	if !service.NativeOCRAvailable() {
		t.Skip("Poppler/Tesseract required; included in backend Docker image")
	}
	for _, name := range []string{"native_text.pdf", "native_scan.pdf"} {
		t.Run(name, func(t *testing.T) {
			// An unconfigured optional parser URL must not make normal local uploads fail.
			pages, total, e := (service.Parser{Endpoint: "http://127.0.0.1:1", Mode: "builtin"}).ParsePageBatch(context.Background(), "testdata/"+name, "application/pdf", 1, 1)
			require.NoError(t, e)
			require.Equal(t, 1, total)
			require.Len(t, pages, 1)
			require.Contains(t, strings.ToLower(pages[0].Text), "memory")
			if name == "native_scan.pdf" {
				require.NotEmpty(t, pages[0].Warnings)
			}
		})
	}
}
