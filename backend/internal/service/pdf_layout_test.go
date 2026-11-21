package service

import (
	"context"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPDFGeometryPreservesScriptAndSourceBoxes(t *testing.T) {
	raw := `<html><body><doc><page width="600" height="800"><flow><block><line><word xMin="10" yMin="20" xMax="20" yMax="32">2</word><word xMin="20" yMin="17" xMax="25" yMax="24">12</word><word xMin="30" yMin="20" xMax="50" yMax="32">字节</word></line></block></flow></page></doc></body></html>`
	p, e := decodePDFLayout([]byte(raw), 4)
	require.NoError(t, e)
	require.Contains(t, p.Text, "2^{12}")
	require.NotContains(t, p.Text, "212")
	require.Len(t, p.Blocks[0].Box, 4)
	require.Equal(t, 4, p.Number)
}
func TestOCRConfidenceAndLineGeometry(t *testing.T) {
	raw := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n1\t1\t0\t0\t0\t0\t0\t0\t600\t800\t-1\t\n5\t1\t1\t1\t1\t1\t10\t20\t40\t15\t95\tTCP\n5\t1\t1\t1\t1\t2\t55\t20\t40\t15\t45\tN-!\n"
	p, e := decodeOCRTSV([]byte(raw), 1)
	require.NoError(t, e)
	require.Len(t, p.Blocks, 1)
	require.NotEmpty(t, p.Warnings)
	require.InDelta(t, .7, *p.Blocks[0].Confidence, .001)
	require.Contains(t, p.Text, "N-!")
}
func TestRealPDFFormulaLayout(t *testing.T) {
	if !NativeOCRAvailable() {
		t.Skip("Poppler/Tesseract unavailable")
	}
	pages, total, handled, err := (Parser{Mode: "builtin"}).nativePDFBatch(context.Background(), "testdata/native_formula.pdf", 1, 1)
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, 1, total)
	require.Contains(t, pages[0].Text, "2^{12}")
	require.Contains(t, pages[0].Text, "N-1")
	require.NotContains(t, pages[0].Text, "212")
	require.NotEmpty(t, pages[0].Blocks[0].Box)
	require.Equal(t, "normalized-top-left-displayed-page", pages[0].CoordinateSpace)
}
func TestUncertainOCRCannotBecomeReliableChunk(t *testing.T) {
	confidence := .4
	page := TextPage{Number: 1, Blocks: []domain.DocumentBlock{{ID: "ocr", Text: "TCP confirmation contains uncertain digits N-! in scanned memory material.", Confidence: &confidence}}}
	CanonicalPage(&page)
	chunks := MappedChunks(page, "revision", "space")
	require.NotEmpty(t, chunks)
	require.Equal(t, true, chunks[0].Metadata["parser_uncertain"])
}

func TestFormulaConflictRetainsRawTextAndCannotSupplyEvidence(t *testing.T) {
	page := TextPage{Number: 1, SourceCheck: domain.SourceConflict, Blocks: []domain.DocumentBlock{
		{ID: "line", Text: "若 由 N-! 个字节组成，则需要核对原图。", Box: []float64{.1, .1, .6, .2}},
	}}
	CanonicalPage(&page)
	CanonicalPage(&page)
	require.Equal(t, "若 由 N-! 个字节组成，则需要核对原图。", page.Blocks[0].RawText)
	require.True(t, page.Blocks[0].NeedsReview)
	require.True(t, questionSpanWarning(page, 0, len([]rune(page.Text))))
	chunks := ContextualChunks([]TextPage{page, {Number: 2, Text: "因此需要额外的确认步骤。"}}, "revision", "space", "source")
	require.Len(t, chunks, 2)
	require.Equal(t, true, chunks[0].Metadata["parser_uncertain"])
	require.Equal(t, true, chunks[1].Metadata["context_incomplete"])
}

func TestLocalizedConflictKeepsAdjacentEvidence(t *testing.T) {
	page := TextPage{Number: 1, SourceCheck: domain.SourceConflict, LocalizedCheck: true, Blocks: []domain.DocumentBlock{
		{ID: "before", Text: "前一段的原文没有识别异常。"},
		{ID: "formula", Text: "序号为 N-!。"},
		{ID: "after", Text: "后一段的原文仍可检索。"},
	}}
	CanonicalPage(&page)
	chunks := MappedChunks(page, "revision", "space")
	require.Len(t, chunks, 3)
	require.Equal(t, false, chunks[0].Metadata["parser_uncertain"])
	require.Equal(t, true, chunks[1].Metadata["parser_uncertain"])
	require.Equal(t, false, chunks[2].Metadata["parser_uncertain"])
}
