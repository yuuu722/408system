package service

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModelHTTP500IsClassifiedWithoutLeakingProviderBody(t *testing.T) {
	for _, tc := range []struct{ body, code string }{{`{"error":"model requires more system memory SECRET"}`, "model_memory_exhausted"}, {`{"error":"llama runner process has terminated SECRET"}`, "model_runner_failed"}, {`{"error":"model qwen not found SECRET"}`, "model_not_found"}, {`{"error":"unknown private failure SECRET"}`, "model_call_failed"}} {
		resp := &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader(tc.body))}
		e := llmHTTPFailure(resp)
		var public *PublicError
		require.ErrorAs(t, e, &public)
		require.Equal(t, tc.code, public.Code)
		require.NotContains(t, e.Error(), "SECRET")
	}
}

func TestKnownJSONCompatibility500RetriesWithoutFormatButMemoryFailureDoesNot(t *testing.T) {
	for _, formatFailure := range []bool{true, false} {
		calls := 0
		model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			if calls == 1 {
				require.NotNil(t, body["response_format"])
				w.WriteHeader(500)
				if formatFailure {
					w.Write([]byte(`{"error":"unsupported response_format"}`))
				} else {
					w.Write([]byte(`{"error":"model requires more system memory"}`))
				}
				return
			}
			require.Nil(t, body["response_format"])
			w.Write([]byte(`{"choices":[{"message":{"content":"{\"ok\":true}"},"finish_reason":"stop"}]}`))
		}))
		_, err := (OpenAILLM{BaseURL: model.URL, APIKey: "fixture"}).AnswerJSON(context.Background(), "只返回JSON", "")
		if formatFailure {
			require.NoError(t, err)
			require.Equal(t, 2, calls)
		} else {
			require.Error(t, err)
			require.Equal(t, 1, calls)
		}
		model.Close()
	}
}
