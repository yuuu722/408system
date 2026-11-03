package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/local/exam408-assistant/backend/internal/service"
	"github.com/local/exam408-assistant/backend/internal/testutil"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

type brokenGraphCheckStore struct{ *testutil.Store }

func (*brokenGraphCheckStore) CheckGraph(context.Context) error {
	return errors.New("private unavailable endpoint")
}
func TestGraphFailureDoesNotMaskActualBackendModelProbe(t *testing.T) {
	calls := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"ok\":true}"},"finish_reason":"stop"}]}`))
	}))
	defer model.Close()
	report := (service.RAGService{KAGEnabled: true, Store: &brokenGraphCheckStore{testutil.NewStore()}, Embedder: fixedEmbedding{}, LLM: service.OpenAILLM{BaseURL: model.URL, APIKey: "fixture"}}).CheckProviders(context.Background())
	require.False(t, report.Passed)
	require.Equal(t, 2, calls)
	require.Len(t, report.Checks, 5)
	require.False(t, report.Checks[1].Passed)
	require.True(t, report.Checks[3].Passed)
	require.True(t, report.Checks[4].Passed)
}
func TestBackendShortSuccessLongFailureHasSeparateDiagnosis(t *testing.T) {
	calls := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct{ Messages []struct{ Content string } }
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		if calls == 2 {
			require.Greater(t, len([]rune(body.Messages[1].Content)), 1600)
			w.WriteHeader(500)
			w.Write([]byte(`{"error":"model requires more system memory PRIVATE"}`))
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"ok\":true}"},"finish_reason":"stop"}]}`))
	}))
	defer model.Close()
	report := (service.RAGService{Embedder: fixedEmbedding{}, LLM: service.OpenAILLM{BaseURL: model.URL, APIKey: "fixture"}}).CheckProviders(context.Background())
	require.False(t, report.Passed)
	require.Len(t, report.Checks, 4)
	require.True(t, report.Checks[2].Passed)
	require.False(t, report.Checks[3].Passed)
	require.Contains(t, report.Checks[3].Message, "短请求成功")
	require.Contains(t, report.Checks[3].Message, "内存或显存不足")
	require.NotContains(t, report.Checks[3].Message, "PRIVATE")
}

func TestAdminModelRouteNeverPrintsURLCredentialsOrQuery(t *testing.T) {
	text := (service.OpenAILLM{BaseURL: "http://user:PRIVATE@host.docker.internal:11434/v1?api_key=HIDDEN", Model: "qwen2.5:3b"}).ModelRouteDescription()
	require.Contains(t, text, "host.docker.internal:11434")
	require.Contains(t, text, "qwen2.5:3b")
	require.NotContains(t, text, "PRIVATE")
	require.NotContains(t, text, "HIDDEN")
	require.NotContains(t, text, "user:")
}
