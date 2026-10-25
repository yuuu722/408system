package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/local/exam408-assistant/backend/internal/service"
	"github.com/stretchr/testify/require"
)

type unavailableEmbedding struct{}

func (unavailableEmbedding) Embed(context.Context, []string) ([][]float32, error) {
	return nil, errors.New("PRIVATE-secret-endpoint")
}

func TestAssistantCheckReportsRealProviderFailureWithoutLeaking(t *testing.T) {
	report := (service.RAGService{Embedder: unavailableEmbedding{}, LLM: forbiddenLLM{t}}).CheckProviders(context.Background())
	require.False(t, report.Passed)
	require.Len(t, report.Checks, 1)
	require.NotContains(t, report.Checks[0].Message, "PRIVATE")
	report = (service.RAGService{Embedder: fixedEmbedding{}, LLM: editorialLLM{"连接正常[1]"}}).CheckProviders(context.Background())
	require.True(t, report.Passed)
	require.Len(t, report.Checks, 2)
	report = (service.RAGService{Embedder: fixedEmbedding{}, LLM: editorialLLM{""}}).CheckProviders(context.Background())
	require.False(t, report.Passed)
	require.False(t, report.Checks[1].Passed)
}
