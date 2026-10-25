package httpapi_test

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/local/exam408-assistant/backend/internal/config"
	"github.com/local/exam408-assistant/backend/internal/httpapi"
	"github.com/local/exam408-assistant/backend/internal/repository"
	"github.com/local/exam408-assistant/backend/internal/service"
	"github.com/local/exam408-assistant/backend/internal/testutil"
	"github.com/stretchr/testify/require"
)

type checkOnlyModel struct{ calls *int }

func (m checkOnlyModel) Answer(context.Context, string, string) (string, error) {
	*m.calls++
	return "PRIVATE-generated-text", nil
}

func TestAssistantConnectionCheckIsAdminOnlyAndRedactsOutput(t *testing.T) {
	store := testutil.NewStore()
	auth := service.AuthService{Store: store, Secret: []byte(strings.Repeat("x", 40)), TTL: time.Hour}
	user, token, err := auth.Register(context.Background(), "checker", "checker@example.invalid", "long-password-408")
	require.NoError(t, err)
	calls := 0
	server := &httpapi.Server{Platform: &repository.Postgres{}, Store: store, Auth: auth, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Config: config.Config{LLMAPIKey: "SECRET-key", EmbeddingMode: "lexical", PersonalKBEnabled: true}, RAG: service.RAGService{Embedder: service.LexicalEmbedding{Dimension: 128}, LLM: checkOnlyModel{&calls}}}
	handler := server.Handler()
	send := func(bearer string, status int) string {
		req := httptest.NewRequest("POST", "/api/v1/admin/assistant/check", strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+bearer)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		require.Equal(t, status, response.Code, response.Body.String())
		return response.Body.String()
	}
	send("", 401)
	send(token, 403)
	require.Zero(t, calls)
	user.Role = "admin"
	store.Users[user.ID] = user
	raw := send(token, 200)
	require.Contains(t, raw, `"passed":true`)
	require.Equal(t, 1, calls)
	require.NotContains(t, raw, "PRIVATE")
	require.NotContains(t, raw, "SECRET")
	server.Config.LLMAPIKey = ""
	raw = send(token, 200)
	require.Contains(t, raw, "LLM_API_KEY")
	require.Equal(t, 1, calls, "missing configuration must not invoke a model")
}
