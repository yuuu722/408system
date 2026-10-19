package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func (p Parser) useRemote() bool {
	return p.Mode != "builtin" && p.Endpoint != "" && (p.Mode == "remote" || p.Token != "")
}

// Extract only the requested pages. The former fallback re-parsed the complete
// textbook for every single-page request, making large PDF processing quadratic.
func (p Parser) nativePDFBatch(ctx context.Context, path string, start, limit int) ([]TextPage, int, bool, error) {
	if _, e := exec.LookPath("pdftotext"); e != nil {
		return nil, 0, false, nil
	}
	info, e := exec.CommandContext(ctx, "pdfinfo", path).Output()
	if e != nil {
		return nil, 0, true, fmt.Errorf("无法读取PDF，可能已加密或损坏")
	}
	total := 0
	for _, line := range strings.Split(string(info), "\n") {
		if strings.HasPrefix(line, "Pages:") {
			total, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Pages:")))
		}
	}
	if total < 1 || total > 2000 || start < 1 || start > total {
		return nil, total, true, fmt.Errorf("PDF页数或请求页码无效（最多2000页）")
	}
	if limit <= 0 {
		limit = total
	}
	pages := []TextPage{}
	for n := start; n <= total && n < start+limit; n++ {
		data, e := exec.CommandContext(ctx, "pdftotext", "-f", strconv.Itoa(n), "-l", strconv.Itoa(n), "-layout", "-enc", "UTF-8", path, "-").Output()
		if e != nil {
			return nil, total, true, fmt.Errorf("PDF第%d页文字提取失败", n)
		}
		page := TextPage{Number: n, Text: strings.TrimSpace(string(data))}
		if !ValidBody(page.Text) {
			if _, e := exec.LookPath("tesseract"); e != nil {
				return nil, total, true, fmt.Errorf("PDF第%d页需要OCR；请重建包含Tesseract的后端镜像", n)
			}
			dir, e := os.MkdirTemp("", "exam408-ocr-")
			if e != nil {
				return nil, total, true, e
			}
			prefix := filepath.Join(dir, "page")
			_, e = exec.CommandContext(ctx, "pdftoppm", "-f", strconv.Itoa(n), "-l", strconv.Itoa(n), "-singlefile", "-scale-to", "2200", "-png", path, prefix).Output()
			if e == nil {
				data, e = exec.CommandContext(ctx, "tesseract", prefix+".png", "stdout", "-l", p.ocrLanguages()).Output()
			}
			os.RemoveAll(dir)
			if e != nil {
				return nil, total, true, fmt.Errorf("PDF第%d页OCR失败，请检查语言包与文件", n)
			}
			page.Text = strings.TrimSpace(string(data))
			page.Warnings = []string{"此页使用本地OCR识别，请核对文字和公式。"}
			if !ValidBody(page.Text) {
				page.Warnings = append(page.Warnings, "此页未识别到足够正文，可能是空白页或图片。")
			}
		}
		pages = append(pages, page)
	}
	return pages, total, true, nil
}

func NativeOCRAvailable() bool {
	for _, name := range []string{"pdftoppm", "pdftotext", "pdfinfo", "tesseract"} {
		if _, err := exec.LookPath(name); err != nil {
			return false
		}
	}
	return true
}

func (p Parser) ocrLanguages() string {
	if languages, err := exec.Command("tesseract", "--list-langs").Output(); err == nil && strings.Contains(string(languages), "chi_sim") {
		return "chi_sim+eng"
	}
	return "eng"
}
