package service

import (
	"context"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOriginalPageLayoutRecoveryUsesStoredFileAndPreservesFormula(t *testing.T) {
	if _, e := exec.LookPath("pdftotext"); e != nil {
		t.Skip("Poppler required")
	}
	data, e := os.ReadFile("testdata/native_formula.pdf")
	require.NoError(t, e)
	root := t.TempDir()
	path := filepath.Join(root, "source.pdf")
	require.NoError(t, os.WriteFile(path, data, 0600))
	safe, hash, e := StoredQuestionFile(root, path)
	require.NoError(t, e)
	require.Len(t, hash, 64)
	page, e := RecoverQuestionPageLayout(context.Background(), safe, 1)
	require.NoError(t, e)
	require.Contains(t, page.Text, "2^{12}")
	require.NotEmpty(t, page.Blocks[0].Box)
	again, e := os.ReadFile(path)
	require.NoError(t, e)
	require.Equal(t, data, again)
	_, _, e = StoredQuestionFile(t.TempDir(), path)
	require.Error(t, e)
}
func TestOnlyDetachedNumbersWithoutBoxesNeedOriginalLayoutRead(t *testing.T) {
	pages := []TextPage{{Number: 1, Text: "01.\n02.\nQuestion bodies separated from their column"}, {Number: 2, Text: "1. A normal question?\nA. First B. Second"}, {Number: 3, Segment: 1, Text: "01.\n02."}}
	require.Equal(t, []int{1}, MissingQuestionLayoutPages(pages))
	pages[0].Blocks = []domain.DocumentBlock{{Box: []float64{0, 0, 1, 1}}}
	require.Empty(t, MissingQuestionLayoutPages(pages))
}

func TestTargetedScannedPageRecoveryAddsOCRCoordinates(t *testing.T) {
	if !NativeOCRAvailable() {
		t.Skip("local OCR tools required")
	}
	page, e := RecoverQuestionPageLayout(context.Background(), "testdata/native_scan.pdf", 1)
	require.NoError(t, e)
	require.Contains(t, strings.ToLower(page.Text), "memory")
	require.NotEmpty(t, page.Blocks)
	require.NotEmpty(t, page.Blocks[0].Box)
	require.NotEmpty(t, page.Warnings)
}
