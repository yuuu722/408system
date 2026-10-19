package httpapi

import (
	"github.com/local/exam408-assistant/backend/internal/config"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMissingModelsRejectChatBeforeProcessing(t *testing.T) {
	for _, cfg := range []config.Config{{}, {LLMAPIKey: "configured"}, {EmbeddingAPIKey: "configured"}} {
		s := Server{Config: cfg}
		w := httptest.NewRecorder()
		s.chat(w, httptest.NewRequest("POST", "/api/v1/exam408/assistant/chat", strings.NewReader(`{"Question":"test"}`)))
		if w.Code != 503 || !strings.Contains(w.Body.String(), "model_not_configured") {
			t.Fatalf("unexpected response %d %s", w.Code, w.Body.String())
		}
	}
}
