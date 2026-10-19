package service

import (
	"archive/zip"
	"context"
	"encoding/json"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDOCXAndQualityGate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.docx")
	f, e := os.Create(path)
	require.NoError(t, e)
	z := zip.NewWriter(f)
	w, e := z.Create("word/document.xml")
	require.NoError(t, e)
	_, e = w.Write([]byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>虚拟内存通过地址映射实现程序隔离并提供独立地址空间。</w:t></w:r></w:p></w:body></w:document>`))
	require.NoError(t, e)
	require.NoError(t, z.Close())
	require.NoError(t, f.Close())
	pages, e := (Parser{}).ParsePages(context.Background(), path, "")
	require.NoError(t, e)
	require.Len(t, pages, 1)
	require.Contains(t, pages[0].Text, "虚拟内存")
	require.False(t, ValidBody(strings.Repeat("王道计算机考研\n", 50)))
	require.False(t, ValidBody(""))
	require.Equal(t, "为什么需要虚拟内存？\n它的作用是什么？", contextualQuestion("它的作用是什么？", []domain.Message{{Role: "user", Content: "为什么需要虚拟内存？"}}))
}
func TestRealSSEParsingAndHistory(t *testing.T) {
	for _, complete := range []bool{true, false} {
		t.Run(map[bool]string{true: "completed", false: "truncated"}[complete], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Messages []map[string]string
					Stream   bool
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.True(t, body.Stream)
				require.Len(t, body.Messages, 4)
				require.Equal(t, "prior question", body.Messages[1]["content"])
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"根据资料[1]\"}}]}\n\n"))
				if complete {
					_, _ = w.Write([]byte("data: [DONE]\n\n"))
				}
			}))
			defer server.Close()
			var deltas string
			answer, e := (OpenAILLM{BaseURL: server.URL, APIKey: "synthetic"}).AnswerStream(context.Background(), "follow up", "evidence", []domain.Message{{Role: "user", Content: "prior question"}, {Role: "assistant", Content: "prior answer"}}, func(s string) error { deltas += s; return nil })
			require.Equal(t, "根据资料[1]", deltas)
			if complete {
				require.NoError(t, e)
				require.Equal(t, deltas, answer)
			} else {
				require.Error(t, e)
				require.Empty(t, answer)
			}
		})
	}
}
func TestResourceReadinessRequiresCompleteContent(t *testing.T) {
	a := domain.Asset{Kind: "question", PointID: "00000000-0000-0000-0000-000000000001", Title: "synthetic", Payload: domain.AssetPayload{Difficulty: 1}}
	require.NoError(t, ValidateAsset(a))
	domain.PrepareAsset(&a)
	require.Equal(t, "unavailable", a.Status)
	a.Payload = domain.AssetPayload{Difficulty: 1, Stem: "Which?", Options: []string{"A", "B"}, Answer: 0, Explanation: "Synthetic explanation", Source: "Synthetic fixture"}
	require.NoError(t, ValidateAsset(a))
	domain.PrepareAsset(&a)
	require.Equal(t, "ready", a.Status)
	a.Kind = "video"
	a.Payload.URL = "javascript:alert(1)"
	require.Error(t, ValidateAsset(a))
}
