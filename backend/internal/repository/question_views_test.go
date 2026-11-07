package repository

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestQuestionViewUsesOriginalNonzeroPageSegment(t *testing.T) {
	base := domain.ParsedDocument{Pages: []domain.StructuredPage{{Number: 16, Segment: 15, Text: "original"}}}
	view := QuestionView{ParserVersion: QuestionViewVersion, BaseFingerprint: ParsedFingerprint(base), Pages: []domain.StructuredPage{{Number: 16, Segment: 15, Text: "recovered"}}}
	merged := mergeQuestionView(base, view)
	require.Equal(t, "recovered", merged.Pages[0].Text)
	require.Equal(t, 15, merged.Pages[0].Segment)
	require.Equal(t, "original", base.Pages[0].Text)
	view.BaseFingerprint = "stale"
	require.Equal(t, "original", mergeQuestionView(base, view).Pages[0].Text)
}
