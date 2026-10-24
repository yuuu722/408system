package service_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/local/exam408-assistant/backend/internal/service"
	"github.com/stretchr/testify/require"
)

func TestNativeWordTextAndImageOCR(t *testing.T) {
	if _, err := exec.LookPath("soffice"); err != nil {
		if _, err = exec.LookPath("libreoffice"); err != nil {
			t.Skip("LibreOffice required; included in Docker")
		}
	}
	if !service.NativeOCRAvailable() {
		t.Skip("Poppler/Tesseract required")
	}
	for _, name := range []string{"native_word_text.docx", "native_word_text.doc", "native_word_scan.docx", "native_word_scan.doc"} {
		t.Run(name, func(t *testing.T) {
			data, e := os.ReadFile(filepath.Join("testdata", name))
			require.NoError(t, e)
			path := filepath.Join(t.TempDir(), name)
			require.NoError(t, os.WriteFile(path, data, 0600))
			parser := service.Parser{Mode: "builtin"}
			start := 1
			if strings.Contains(name, "scan") {
				start = 2
			}
			pages, total, e := parser.ParsePageBatch(context.Background(), path, "", start, 1)
			require.NoError(t, e)
			require.GreaterOrEqual(t, total, start)
			require.Len(t, pages, 1)
			require.Contains(t, strings.ToLower(pages[0].Text), "memory")
			require.Equal(t, start, pages[0].Number)
			require.Equal(t, start, pages[0].Segment)
			require.Contains(t, strings.Join(pages[0].Warnings, " "), "Word转换页码")
			if start == 2 {
				require.Contains(t, strings.Join(pages[0].Warnings, " "), "OCR")
			}
			cached, e := filepath.Glob(filepath.Join(filepath.Dir(path), ".word-preview-*", "preview.pdf"))
			require.NoError(t, e)
			require.Len(t, cached, 1)
			info, e := os.Stat(cached[0])
			require.NoError(t, e)
			_, _, e = parser.ParsePageBatch(context.Background(), path, "", start, 1)
			require.NoError(t, e)
			again, e := os.Stat(cached[0])
			require.NoError(t, e)
			require.Equal(t, info.ModTime(), again.ModTime(), "must reuse cached conversion")
			ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
			defer cancel()
			<-ctx.Done()
			_, _, e = parser.ParsePageBatch(ctx, path, "", start, 1)
			require.Error(t, e)
		})
	}
}
