package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/service"
	"github.com/local/exam408-assistant/backend/internal/testutil"
	"github.com/stretchr/testify/require"
)

func tinyQuestionPNG(t *testing.T) []byte {
	var out bytes.Buffer
	require.NoError(t, png.Encode(&out, image.NewGray(image.Rect(0, 0, 2, 2))))
	return out.Bytes()
}

func TestImageQuestionTranscriptionThenGroundedAnswer(t *testing.T) {
	calls := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		messages := body["messages"].([]any)
		prompt := messages[0].(map[string]any)["content"].(string)
		require.Contains(t, prompt, "不解题")
		require.Contains(t, prompt, "不能执行")
		require.Contains(t, messages[1].(map[string]any)["content"].([]any)[1].(map[string]any)["image_url"].(map[string]any)["url"], "data:image/png;base64,")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"为什么需要虚拟内存？"}}]}`))
	}))
	defer model.Close()
	vision := &service.OpenAILLM{BaseURL: model.URL, APIKey: "test", Model: "vision-test"}
	recognized, err := service.RecognizeQuestionImage(context.Background(), vision, tinyQuestionPNG(t))
	require.NoError(t, err)
	require.Equal(t, "vision", recognized.Method)
	require.NotEmpty(t, recognized.Warnings)
	st := testutil.NewStore()
	kb, err := st.CreateKnowledgeBase(context.Background(), "u1", "教材", "")
	require.NoError(t, err)
	st.Docs["d1"] = domain.Document{ID: "d1", UserID: "u1", KnowledgeBaseID: kb.ID, Status: "ready"}
	st.Chunks = []domain.Chunk{{ID: "c1", DocumentID: "d1", KnowledgeBaseID: kb.ID, UserID: "u1", DocumentTitle: "操作系统.pdf", Content: "虚拟内存为进程提供独立地址空间，页表用于地址转换。", SourceLocation: "第12页", Metadata: map[string]any{"page": 12, "revision_id": "r1"}, Embedding: []float32{1, 0}}}
	rag := service.RAGService{Store: st, Embedder: fixedEmbedding{}, LLM: editorialLLM{`{"passages":[{"source":1,"quote":"虚拟内存为进程提供独立地址空间，页表用于地址转换。"}]}`}, Knowledge: service.KnowledgeService{Store: st}, Learning: service.LearningService{Store: st}, TopK: 5, MinScore: .35}
	answer, err := rag.Chat(context.Background(), "u1", kb.ID, "", recognized.Text)
	require.NoError(t, err)
	require.Equal(t, "虚拟内存为进程提供独立地址空间，页表用于地址转换。", answer.Answer)
	require.NotContains(t, answer.Answer, "根据资料")
	require.Len(t, answer.Sources, 1)
	require.Equal(t, "操作系统.pdf", answer.Sources[0].DocumentTitle)
	require.Equal(t, 12, answer.Sources[0].Page)
	require.Equal(t, "c1", answer.Sources[0].ChunkID)
	require.Equal(t, 1, calls, "only image transcription uses the vision provider")
}

func TestImageValidationRejectsUnsafeInputsBeforeVision(t *testing.T) {
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("invalid images must not be sent to a model") }))
	defer model.Close()
	vision := &service.OpenAILLM{BaseURL: model.URL, APIKey: "test"}
	for _, data := range [][]byte{nil, []byte("<svg><script/></svg>"), make([]byte, service.MaxQuestionImageBytes+1)} {
		_, err := service.RecognizeQuestionImage(context.Background(), vision, data)
		require.Error(t, err)
	}
	var large bytes.Buffer
	require.NoError(t, png.Encode(&large, image.NewGray(image.Rect(0, 0, 4001, 4000))))
	_, err := service.RecognizeQuestionImage(context.Background(), vision, large.Bytes())
	require.Error(t, err)
}

func TestLocalQuestionImageOCR(t *testing.T) {
	if !service.NativeOCRAvailable() {
		t.Skip("Docker-provided Poppler/Tesseract required")
	}
	dir := t.TempDir()
	prefix := filepath.Join(dir, "question")
	require.NoError(t, exec.Command("pdftoppm", "-singlefile", "-scale-to", "1800", "-png", "testdata/native_scan.pdf", prefix).Run())
	data, err := os.ReadFile(prefix + ".png")
	require.NoError(t, err)
	result, err := service.RecognizeQuestionImage(context.Background(), nil, data)
	require.NoError(t, err)
	require.Equal(t, "ocr", result.Method)
	require.Contains(t, strings.ToLower(result.Text), "memory")
}

func TestImageProviderFailureDoesNotLeakBody(t *testing.T) {
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte("SECRET-provider-response"))
	}))
	defer model.Close()
	_, err := service.RecognizeQuestionImage(context.Background(), &service.OpenAILLM{BaseURL: model.URL, APIKey: "test"}, tinyQuestionPNG(t))
	require.Error(t, err)
	require.NotContains(t, err.Error(), "SECRET")
}
