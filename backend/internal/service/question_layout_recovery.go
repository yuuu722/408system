package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/sourcequality"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func StoredQuestionFile(root, path string) (string, string, error) {
	root, e := filepath.Abs(root)
	if e != nil {
		return "", "", e
	}
	root, e = filepath.EvalSymlinks(root)
	if e != nil {
		return "", "", e
	}
	path, e = filepath.Abs(path)
	if e != nil {
		return "", "", e
	}
	path, e = filepath.EvalSymlinks(path)
	if e != nil {
		return "", "", fmt.Errorf("原上传文件不可读")
	}
	rel, e := filepath.Rel(root, path)
	if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("禁止读取上传目录外的文件")
	}
	f, e := os.Open(path)
	if e != nil {
		return "", "", fmt.Errorf("原上传文件不可读")
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("原上传文件不是普通文件")
	}
	hash := sha256.New()
	if _, e = io.Copy(hash, f); e != nil {
		return "", "", e
	}
	return path, hex.EncodeToString(hash.Sum(nil)), nil
}

// Read only the selected original PDF page. Prefer its native text/boxes; if
// the PDF is image-only, OCR that page to TSV to recover confidence and boxes.
// No model call, full-document OCR or embedding rebuild is performed.
func RecoverQuestionPageLayout(ctx context.Context, path string, page int) (TextPage, error) {
	if page < 1 || page > 2000 {
		return TextPage{}, fmt.Errorf("PDF页码无效")
	}
	ctx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	nativeCtx, stop := context.WithTimeout(ctx, 15*time.Second)
	raw, e := exec.CommandContext(nativeCtx, "pdftotext", "-f", strconv.Itoa(page), "-l", strconv.Itoa(page), "-bbox-layout", "-enc", "UTF-8", path, "-").Output()
	stop()
	if e == nil {
		if p, err := decodePDFLayout(raw, page); err == nil && ValidBody(p.Text) {
			if sourcequality.SuspectFormula(p.Text) {
				p.Warnings = append(p.Warnings, "文字层仍含疑似公式错误，未猜改数字与符号。")
			}
			return p, nil
		}
	}
	if !NativeOCRAvailable() {
		return TextPage{}, fmt.Errorf("PDF第%d页文字层不足，且局部OCR工具不可用", page)
	}
	p, e := (Parser{Mode: "builtin"}).ocrPDFPage(ctx, path, page, false)
	if e != nil || !ValidBody(p.Text) {
		return TextPage{}, fmt.Errorf("PDF第%d页局部OCR未恢复足够正文，保留原解析", page)
	}
	return p, nil
}

func MissingQuestionLayoutPages(pages []TextPage) []int {
	out := []int{}
	seen := map[int]bool{}
	counts := map[int]int{}
	for _, p := range pages {
		counts[p.Number]++
	}
	for _, p := range pages {
		if p.Number < 1 || counts[p.Number] != 1 || seen[p.Number] {
			continue
		}
		n := 0
		for _, line := range strings.Split(p.Text, "\n") {
			if isolatedQuestionNumber.MatchString(questionLine(line)) {
				n++
			}
		}
		boxes := 0
		for _, b := range p.Blocks {
			if len(b.Box) == 4 {
				boxes++
			}
		}
		if (n >= 2 && boxes == 0) || sourcequality.SuspectFormula(p.Text) {
			out = append(out, p.Number)
			seen[p.Number] = true
		}
	}
	return out
}
