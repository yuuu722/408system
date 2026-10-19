package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestConfiguredParserKeepsMarkdownLocal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.md")
	body := "# 408 learning notes\nVirtual memory maps logical addresses to physical memory."
	require.NoError(t, os.WriteFile(path, []byte(body), 0600))
	pages, err := (Parser{Endpoint: "http://127.0.0.1:1"}).ParsePages(context.Background(), path, "text/markdown")
	require.NoError(t, err)
	require.Len(t, pages, 1)
	require.Contains(t, pages[0].Text, "Virtual memory")
}

func TestVisionStreamSendsImages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Messages []struct{ Content any } }
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		content, ok := body.Messages[len(body.Messages)-1].Content.([]any)
		require.True(t, ok)
		require.Len(t, content, 2)
		require.Equal(t, "image_url", content[1].(map[string]any)["type"])
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"合成图片解释[1]\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	answer, e := (OpenAILLM{BaseURL: server.URL, APIKey: "synthetic"}).answerWithImages(context.Background(), "图中含义", "[1] synthetic", nil, []string{"data:image/png;base64,AA=="}, nil)
	require.NoError(t, e)
	require.Contains(t, answer, "图片解释")
}

func TestExtractQuestionsAndMissingFigure(t *testing.T) {
	d := domain.Document{ID: "doc", OriginalFilename: "synthetic.txt"}
	points := []domain.KnowledgePoint{{ID: "point", Name: "内存", Keywords: []string{"内存"}}}
	pages := []TextPage{{Number: 1, Text: "1. 下列内存哪个正确？\nA. 第一项 B. 第二项\n2. 如图所示内存如何分配？\nA. 方式一 B. 方式二\n3. 下列内存哪个正确？\nA. 第一种 B. 第二种"}, {Number: 2, Text: "参考答案\n1. A"}}
	assets := ExtractQuestions(d, pages, points)
	require.Len(t, assets, 3)
	require.Equal(t, "source", assets[0].Payload.AnswerOrigin)
	require.Equal(t, 0, assets[0].Payload.Answer)
	require.Equal(t, "ready", assets[0].Status)
	require.Equal(t, "unavailable", assets[1].Status)
	require.Contains(t, assets[1].Payload.Issue, "图片")
	require.Equal(t, "ai_pending", assets[2].Payload.AnswerOrigin)
	require.Equal(t, "ready", assets[2].Status)
	require.Equal(t, 1, assets[0].Payload.Page)
	require.Contains(t, assets[0].Payload.Source, "答案第2页")
}

func TestQuestionContinuationAndAnswerPageFailures(t *testing.T) {
	d := domain.Document{ID: "doc", OriginalFilename: "synthetic.pdf"}
	points := []domain.KnowledgePoint{{ID: "point", Name: "内存"}}
	question := TextPage{Number: 1, Text: "1. 下列内存布局如图所示，哪个正确？\nA. 第一项 B. 第二项"}
	t.Run("image only continuation", func(t *testing.T) {
		pages := []TextPage{question, {Number: 2, Blocks: []domain.DocumentBlock{{ID: "figure", Kind: "image", Image: "data:image/png;base64,fixture"}}}}
		assets := ExtractQuestions(d, pages, points)
		require.Len(t, assets, 1)
		require.Len(t, assets[0].Payload.Images, 1)
		require.Equal(t, "ready", assets[0].Status)
	})
	t.Run("failed continuation", func(t *testing.T) {
		pages := []TextPage{question, {Number: 2, Warnings: []string{"OCR failed"}, Blocks: []domain.DocumentBlock{{Kind: "image", Image: "data:image/png;base64,fixture"}}}}
		assets := ExtractQuestions(d, pages, points)
		require.Len(t, assets, 1)
		require.Equal(t, "unavailable", assets[0].Status)
		require.Contains(t, assets[0].Payload.Issue, "解析异常")
	})
	t.Run("unreliable answer page", func(t *testing.T) {
		question.Text = "1. 下列内存描述哪个正确？\nA. 第一项 B. 第二项"
		pages := []TextPage{question, {Number: 2, Text: "参考答案\n1. A", Warnings: []string{"incomplete OCR"}}}
		assets := ExtractQuestions(d, pages, points)
		require.Len(t, assets, 1)
		require.Equal(t, "ai_pending", assets[0].Payload.AnswerOrigin)
		require.Equal(t, -1, assets[0].Payload.Answer)
		require.Equal(t, "ready", assets[0].Status)
	})
	t.Run("new question does not attach its figure to previous page", func(t *testing.T) {
		pages := []TextPage{{Number: 1, Text: "1. 下列内存描述哪个正确？\nA. 第一项 B. 第二项"},
			{Number: 2, Text: "2. 下列内存布局如图所示，哪个正确？\nA. 第三项 B. 第四项", Blocks: []domain.DocumentBlock{{Kind: "image", Image: "data:image/png;base64,fixture"}}}}
		assets := ExtractQuestions(d, pages, points)
		require.Len(t, assets, 2)
		require.Empty(t, assets[0].Payload.Images)
		require.Len(t, assets[1].Payload.Images, 1)
	})
}
func TestQuestionAIProtocolAndUncertainty(t *testing.T) {
	for _, raw := range []string{`{"answer":0,"explanation":"Synthetic"}`, `{"answer":-1,"explanation":"Insufficient evidence"}`, `{"answer":9,"explanation":"Wrong shape"}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Bearer synthetic", r.Header.Get("Authorization"))
			fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, raw)
		}))
		ai := QuestionAI{Text: OpenAILLM{APIKey: "synthetic", BaseURL: server.URL}}
		q := domain.Asset{Payload: domain.AssetPayload{Stem: "test", Options: []string{"A", "B"}}}
		answer, _, e := ai.Infer(context.Background(), q)
		if raw[10] == '9' {
			require.Error(t, e)
		} else {
			require.NoError(t, e)
			require.GreaterOrEqual(t, answer, -1)
			require.Less(t, answer, 2)
		}
		q.Payload.Images = []string{"data:image/png;base64,AA=="}
		_, _, e = ai.Infer(context.Background(), q)
		require.Error(t, e)
		server.Close()
	}
}
