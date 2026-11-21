package service

import (
	"context"
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/sourcequality"
	"image"
	"image/png"
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
		page.SourceCheck = domain.SourceUnknown
		invalid := !ValidBody(page.Text)
		suspect := sourcequality.SuspectFormula(page.Text) || strings.ContainsRune(page.Text, '�')
		if suspect && !invalid && p.checkPDFRegions(ctx, path, &page) {
			CanonicalPage(&page)
			pages = append(pages, page)
			continue
		}
		if invalid || suspect || n%25 == 0 {
			recognized, err := p.ocrPDFPage(ctx, path, n)
			switch {
			case err != nil && ctx.Err() != nil:
				return nil, total, true, ctx.Err()
			case invalid && (err != nil || !ValidBody(recognized.Text)):
				// A bad page should not discard successful pages in the same file.
				page.SourceCheck = domain.SourceConflict
				page.Warnings = append(page.Warnings, "此页未能恢复有效正文，已保留原页供核对，本页不进入检索或测评。")
			case err != nil && suspect:
				page.Warnings = append(page.Warnings, "图像复读失败，疑点待核对原页。")
			case err != nil:
				// A sampled check that could not run remains unknown.
			case invalid && !suspect:
				page = recognized
				page.SourceCheck = domain.SourceUnknown
			case !suspect && ValidBody(recognized.Text) && !sourceSpanNeedsReview(recognized, 0, len([]rune(recognized.Text))) &&
				compactRecognition(page.Text) == compactRecognition(recognized.Text):
				page.SourceCheck = domain.SourceCorroborated
			case suspect && compactRecognition(page.Text) == compactRecognition(recognized.Text):
				// Agreement on suspicious characters does not resolve the doubt.
				page.Warnings = append(page.Warnings, "文字层与图像复读相同，疑点仍需核对原页。")
			default:
				page.SourceCheck = domain.SourceConflict
				page.Warnings = append(page.Warnings, "文字层与图像复读存在分歧，保留原提取结果，请核对原页。")
			}
		}
		CanonicalPage(&page)

		pages = append(pages, page)
	}
	return pages, total, true, nil
}

// Re-read bounded suspicious regions from the rendered page. Keep their
// original text even when OCR agrees, and leave adjacent lines searchable.
func (p Parser) checkPDFRegions(ctx context.Context, path string, page *TextPage) bool {
	if page.CoordinateSpace != "normalized-top-left-displayed-page" {
		return false
	}
	var indices []int
	for i, block := range page.Blocks {
		if blockNeedsReview(block) {
			if len(block.Box) != 4 || len(indices) == 6 {
				return false
			}
			indices = append(indices, i)
		}
	}
	if len(indices) == 0 {
		return false
	}
	dir, err := os.MkdirTemp("", "exam408-region-")
	if err != nil {
		return false
	}
	defer os.RemoveAll(dir)
	prefix := filepath.Join(dir, "page")
	number := strconv.Itoa(page.Number)
	if err := exec.CommandContext(ctx, "pdftoppm", "-f", number, "-l", number, "-singlefile", "-scale-to", "3500", "-png", path, prefix).Run(); err != nil {
		return false
	}
	file, err := os.Open(prefix + ".png")
	if err != nil {
		return false
	}
	img, err := png.Decode(file)
	file.Close()
	if err != nil {
		return false
	}
	cropper, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return false
	}
	bounds := img.Bounds()
	x := func(v float64) int { return bounds.Min.X + int(v*float64(bounds.Dx())) }
	y := func(v float64) int { return bounds.Min.Y + int(v*float64(bounds.Dy())) }
	languages := p.ocrLanguages()
	checked := 0
	for _, index := range indices {
		block := &page.Blocks[index]
		box := block.Box
		rect := image.Rect(x(box[0])-8, y(box[1])-8, x(box[2])+8, y(box[3])+8).Intersect(bounds)
		if rect.Empty() || rect.Dx() < 16 || rect.Dy() < 12 {
			continue
		}
		crop, err := os.Create(filepath.Join(dir, "region.png"))
		if err != nil {
			continue
		}
		err = png.Encode(crop, cropper.SubImage(rect))
		crop.Close()
		if err != nil {
			continue
		}
		data, err := exec.CommandContext(ctx, "tesseract", crop.Name(), "stdout", "-l", languages, "--psm", "7").Output()
		if err != nil || strings.TrimSpace(string(data)) == "" {
			continue
		}
		checked++
		block.NeedsReview = true
		if compactRecognition(block.Text) != compactRecognition(string(data)) {
			page.SourceCheck = domain.SourceConflict
		}
	}
	if checked == 0 {
		return false
	}
	page.LocalizedCheck = true
	page.Warnings = append(page.Warnings, fmt.Sprintf("局部图像复读完成 %d/%d 个疑点，保留原提取文字，请核对原页。", checked, len(indices)))
	return true
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

func (p Parser) ocrPDFPage(ctx context.Context, path string, n int) (TextPage, error) {
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
	if !ValidBody(page.Text) {
		page.Warnings = append(page.Warnings, "此页未识别到足够正文，可能是空白页或图片。")
	}
	return page, nil
}
func compactRecognition(s string) string {
	return strings.Join(strings.Fields(sourcequality.Normalize(s)), "")
}
