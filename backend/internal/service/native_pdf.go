package service

import (
	"context"
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/sourcequality"
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
		// Use geometry rather than trusting a flattened text layer, including
		// on text-rich pages where the old OCR fallback never ran.
		if layout, err := exec.CommandContext(ctx, "pdftotext", "-f", strconv.Itoa(n), "-l", strconv.Itoa(n), "-bbox-layout", "-enc", "UTF-8", path, "-").Output(); err == nil {
			if structured, err := decodePDFLayout(layout, n); err == nil && ValidBody(structured.Text) {
				page = structured
			}
		}
		invalid := !ValidBody(page.Text)
		suspect := sourcequality.SuspectFormula(page.Text)
		if invalid || suspect {
			original := page
			recognized, err := p.ocrPDFPage(ctx, path, n, suspect && !invalid)
			if err != nil {
				if invalid {
					return nil, total, true, err
				}
				page.Warnings = append(page.Warnings, "文字层含疑似公式错误，图像复读失败；请核对原页，未自动更改数字。")
			} else if invalid {
				page = recognized
			} else if !sourcequality.SuspectFormula(recognized.Text) && sameSurroundingText(original.Text, recognized.Text) {
				page = recognized
				page.Warnings = append(page.Warnings, "文字层疑似损坏，已由两种OCR布局复读一致的页面替代，仍需核对公式。")
			} else {
				page = original
				page.Warnings = append(page.Warnings, "文字层与图像复读存在分歧，未自动更改公式，请核对原页。")
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

func (p Parser) ocrPDFPage(ctx context.Context, path string, n int, verify bool) (TextPage, error) {
	dir, err := os.MkdirTemp("", "exam408-ocr-")
	if err != nil {
		return TextPage{}, err
	}
	defer os.RemoveAll(dir)
	prefix := filepath.Join(dir, "page")
	if _, err = exec.CommandContext(ctx, "pdftoppm", "-f", strconv.Itoa(n), "-l", strconv.Itoa(n), "-singlefile", "-r", "300", "-png", path, prefix).Output(); err != nil {
		return TextPage{}, fmt.Errorf("PDF第%d页渲染失败", n)
	}
	data, err := exec.CommandContext(ctx, "tesseract", prefix+".png", "stdout", "-l", p.ocrLanguages(), "--psm", "3", "tsv").Output()
	if err != nil {
		return TextPage{}, fmt.Errorf("PDF第%d页OCR失败，请检查工具与语言包", n)
	}
	page, err := decodeOCRTSV(data, n)
	if err != nil {
		return TextPage{}, err
	}
	page.Warnings = append(page.Warnings, "此页使用本地OCR识别，请核对文字和公式。")
	if verify {
		second, e := exec.CommandContext(ctx, "tesseract", prefix+".png", "stdout", "-l", p.ocrLanguages(), "--psm", "6").Output()
		if e != nil || compactRecognition(page.Text) != compactRecognition(string(second)) {
			return TextPage{}, fmt.Errorf("OCR复读不一致")
		}
		for _, b := range page.Blocks {
			if b.Confidence != nil && *b.Confidence < .8 {
				return TextPage{}, fmt.Errorf("OCR置信度不足")
			}
		}
	}
	if !ValidBody(page.Text) {
		page.Warnings = append(page.Warnings, "此页未识别到足够正文，可能是空白页或图片。")
	}
	return page, nil
}
func compactRecognition(s string) string {
	return strings.Join(strings.Fields(sourcequality.Normalize(s)), "")
}

// A replacement must preserve the surrounding content, not merely remove the
// suspicious character. This is deliberately conservative for dense formulas.
func sameSurroundingText(a, b string) bool {
	x, y := []rune(compactRecognition(a)), []rune(compactRecognition(b))
	if len(x) < 20 || len(y) < 20 || len(x) > 20000 || len(y) > 20000 {
		return false
	}
	// Bounded edit distance: at most 2% character changes are allowed.
	budget := max(2, len(x)/50)
	if len(x)-len(y) > budget || len(y)-len(x) > budget {
		return false
	}

	prev, cur := make([]int, len(y)+1), make([]int, len(y)+1)
	for j := range prev {
		prev[j] = min(j, budget+1)
		cur[j] = budget + 1
	}
	for i, c := range x {
		row := i + 1
		lo, hi := max(1, row-budget), min(len(y), row+budget)
		cur[0] = min(row, budget+1)
		if lo > 1 {
			cur[lo-1] = budget + 1
		}
		for j := lo; j <= hi; j++ {
			cost := 0
			if c != y[j-1] {
				cost = 1
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		if hi < len(y) {
			cur[hi+1] = budget + 1
		}
		prev, cur = cur, prev
	}

	return prev[len(y)] <= budget
}
