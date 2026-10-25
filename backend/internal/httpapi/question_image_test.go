package httpapi_test

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/local/exam408-assistant/backend/internal/config"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/httpapi"
	"github.com/local/exam408-assistant/backend/internal/repository"
	"github.com/local/exam408-assistant/backend/internal/service"
	"github.com/local/exam408-assistant/backend/internal/testutil"
	"github.com/stretchr/testify/require"
)

type imageAccessStore struct{ *testutil.Store }

func (s imageAccessStore) KnowledgeBase(ctx context.Context, uid, kid string) (domain.KnowledgeBase, error) {
	kb, err := s.Store.KnowledgeBase(ctx, uid, kid)
	if err != nil {
		return kb, repository.ErrNotFound
	}
	return kb, nil
}

func TestQuestionImageAPIEnforcesKnowledgeBaseAccessBeforeModel(t *testing.T) {
	ctx := context.Background()
	store := imageAccessStore{testutil.NewStore()}
	auth := service.AuthService{Store: store, Secret: []byte(strings.Repeat("k", 40)), TTL: time.Hour}
	owner, token, err := auth.Register(ctx, "image-owner", "image@example.invalid", "long-password-408")
	require.NoError(t, err)
	other, _, err := auth.Register(ctx, "other-owner", "other@example.invalid", "long-password-408")
	require.NoError(t, err)
	own, err := store.CreateKnowledgeBase(ctx, owner.ID, "我的资料", "")
	require.NoError(t, err)
	private, err := store.CreateKnowledgeBase(ctx, other.ID, "他人资料", "")
	require.NoError(t, err)
	calls := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(`{"choices":[{"message":{"content":"虚拟内存是什么？"}}]}`))
	}))
	defer model.Close()
	server := &httpapi.Server{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Store: store, Auth: auth, Config: config.Config{PersonalKBEnabled: true}, RAG: service.RAGService{Vision: &service.OpenAILLM{BaseURL: model.URL, APIKey: "fixture", Model: "vision-test"}}}
	handler := server.Handler()
	var imageBytes bytes.Buffer
	require.NoError(t, png.Encode(&imageBytes, image.NewGray(image.Rect(0, 0, 2, 2))))
	send := func(kb, credential string, data []byte, status int) {
		t.Helper()
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		require.NoError(t, mw.WriteField("KnowledgeBaseID", kb))
		part, e := mw.CreateFormFile("file", "question.png")
		require.NoError(t, e)
		_, e = part.Write(data)
		require.NoError(t, e)
		require.NoError(t, mw.Close())
		req := httptest.NewRequest("POST", "/api/v1/exam408/assistant/recognize", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+credential)
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, req)
		require.Equal(t, status, out.Code, out.Body.String())
	}
	send(own.ID, "", imageBytes.Bytes(), 401)
	send(private.ID, token, imageBytes.Bytes(), 404)
	require.Zero(t, calls)
	send(own.ID, token, []byte("<svg/>"), 400)
	require.Zero(t, calls)
	send(own.ID, token, imageBytes.Bytes(), 200)
	require.Equal(t, 1, calls)
}
