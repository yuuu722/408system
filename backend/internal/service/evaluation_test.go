package service

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

// Synthetic contract regression only; neither an embedding benchmark nor a real-corpus quality score.
func TestSyntheticEvidenceRegression(t *testing.T) {
	raw, err := os.ReadFile("testdata/synthetic_relevance.json")
	require.NoError(t, err)
	var cases []struct {
		Name, Question, Evidence string
		Related                  bool
	}
	require.NoError(t, json.Unmarshal(raw, &cases))
	require.NotEmpty(t, cases)
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			require.Equal(t, c.Related, evidenceRelated(c.Question, c.Evidence))
		})
	}
}
