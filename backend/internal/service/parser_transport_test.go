package service

import (
	"context"
	"encoding/json"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestParserBatchTransport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.pdf")
	require.NoError(t, os.WriteFile(path, []byte("synthetic transport bytes"), 0600))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer synthetic", r.Header.Get("Authorization"))
		require.NoError(t, r.ParseMultipartForm(1<<20))
		defer r.MultipartForm.RemoveAll()
		require.Equal(t, "2", r.FormValue("start_page"))
		require.Equal(t, "1", r.FormValue("page_limit"))
		file, _, e := r.FormFile("file")
		require.NoError(t, e)
		file.Close()
		_ = json.NewEncoder(w).Encode(map[string]any{"TotalPages": 3, "Pages": []TextPage{{Number: 2, Text: "合成内容"}}})
	}))
	defer server.Close()
	pages, total, e := (Parser{Endpoint: server.URL, Token: "synthetic"}).ParsePageBatch(context.Background(), path, "application/pdf", 2, 1)
	require.NoError(t, e)
	require.Equal(t, 3, total)
	require.Equal(t, 2, pages[0].Number)
	require.True(t, pointEvidenceRelated(domain.KnowledgePoint{Name: "内存管理", Keywords: []string{"虚拟内存"}}, "虚拟内存通过地址映射隔离进程。"))
}
