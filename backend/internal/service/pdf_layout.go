package service

import (
	"encoding/xml"
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"math"
	"strconv"
	"strings"
	"unicode"
)

type pdfWord struct {
	X0   float64 `xml:"xMin,attr"`
	Y0   float64 `xml:"yMin,attr"`
	X1   float64 `xml:"xMax,attr"`
	Y1   float64 `xml:"yMax,attr"`
	Text string  `xml:",chardata"`
}
type pdfLine struct {
	Words []pdfWord `xml:"word"`
}
type pdfBlock struct {
	Lines []pdfLine `xml:"line"`
}
type pdfLayoutPage struct {
	Width  float64 `xml:"width,attr"`
	Height float64 `xml:"height,attr"`
	Flows  []struct {
		Blocks []pdfBlock `xml:"block"`
	} `xml:"flow"`
}

// Poppler's reading order and block boundaries are retained. Baseline geometry
// records superscripts/subscripts instead of flattening 2^12 into 212.
func decodePDFLayout(data []byte, number int) (TextPage, error) {
	var doc struct {
		Body struct {
			Pages []pdfLayoutPage `xml:"doc>page"`
		} `xml:"body"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return TextPage{}, err
	}
	if len(doc.Body.Pages) != 1 {
		return TextPage{}, fmt.Errorf("invalid PDF layout page")
	}
	p := doc.Body.Pages[0]
	if p.Width <= 0 || p.Height <= 0 {
		return TextPage{}, fmt.Errorf("invalid PDF geometry")
	}
	page := TextPage{Number: number, Width: p.Width, Height: p.Height, CoordinateSpace: "normalized-top-left-displayed-page"}
	for _, flow := range p.Flows {
		for _, block := range flow.Blocks {
			for _, line := range block.Lines {
				if len(line.Words) == 0 {
					continue
				}
				base := line.Words[0]
				for _, w := range line.Words {
					if w.Y1-w.Y0 > base.Y1-base.Y0 {
						base = w
					}
				}
				x0, y0, x1, y1 := math.Inf(1), math.Inf(1), 0.0, 0.0
				var text strings.Builder
				for i, w := range line.Words {
					x0 = math.Min(x0, w.X0)
					y0 = math.Min(y0, w.Y0)
					x1 = math.Max(x1, w.X1)
					y1 = math.Max(y1, w.Y1)
					value := strings.TrimSpace(w.Text)
					if value == "" {
						continue
					}
					h := base.Y1 - base.Y0
					script := i > 0 && h > 0 && w.Y1-w.Y0 < h*.85 && w.X0-line.Words[i-1].X1 < h*.7 && scriptToken(value)
					if script && w.Y1 < base.Y1-h*.15 {
						text.WriteString("^{" + value + "}")
					} else if script && w.Y0 > base.Y0+h*.3 {
						text.WriteString("_{" + value + "}")
					} else {
						if text.Len() > 0 {
							text.WriteByte(' ')
						}
						text.WriteString(value)
					}
				}
				if text.Len() > 0 {
					page.Blocks = append(page.Blocks, domain.DocumentBlock{ID: fmt.Sprintf("pdf-line-%d", len(page.Blocks)), Kind: "text", Text: text.String(), Box: []float64{x0 / p.Width, y0 / p.Height, x1 / p.Width, y1 / p.Height}})
				}
			}
			// Empty structural separator prevents columns/paragraphs merging into a claim.
			page.Blocks = append(page.Blocks, domain.DocumentBlock{ID: fmt.Sprintf("pdf-break-%d", len(page.Blocks)), Kind: "paragraph_break"})
		}
	}
	if repaired, ok := repairQuestionReadingPage(page); ok {
		return repaired, nil
	}
	CanonicalPage(&page)
	return page, nil
}
func scriptToken(s string) bool {
	if len([]rune(s)) > 8 {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) && !unicode.IsLetter(r) && !strings.ContainsRune("+-−=", r) {
			return false
		}
	}
	return true
}

// TSV retains OCR line boxes and per-line confidence for inspection. Confidence
// is recognition confidence, never a certificate of mathematical correctness.
func decodeOCRTSV(data []byte, number int) (TextPage, error) {
	page := TextPage{Number: number, CoordinateSpace: "normalized-top-left-displayed-page"}
	keys := map[string]int{}
	sums := map[int]float64{}
	counts := map[int]int{}
	for _, row := range strings.Split(string(data), "\n") {
		f := strings.SplitN(strings.TrimRight(row, "\r"), "\t", 12)
		if len(f) != 12 || f[0] == "level" {
			continue
		}
		nums := make([]float64, 11)
		valid := true
		for i := 0; i < 11; i++ {
			v, e := strconv.ParseFloat(f[i], 64)
			if e != nil {
				valid = false
				break
			}
			nums[i] = v
		}
		if !valid {
			continue
		}
		if f[0] == "1" {
			page.Width = nums[8]
			page.Height = nums[9]
			continue
		}
		if f[0] != "5" || strings.TrimSpace(f[11]) == "" {
			continue
		}
		if page.Width <= 0 || page.Height <= 0 {
			return page, fmt.Errorf("invalid OCR geometry")
		}
		key := strings.Join(f[1:5], "/")
		idx, ok := keys[key]
		box := []float64{nums[6] / page.Width, nums[7] / page.Height, (nums[6] + nums[8]) / page.Width, (nums[7] + nums[9]) / page.Height}
		if !ok {
			idx = len(page.Blocks)
			keys[key] = idx
			page.Blocks = append(page.Blocks, domain.DocumentBlock{ID: fmt.Sprintf("ocr-line-%d", idx), Kind: "text", Box: box})
		} else {
			b := page.Blocks[idx].Box
			b[0] = math.Min(b[0], box[0])
			b[1] = math.Min(b[1], box[1])
			b[2] = math.Max(b[2], box[2])
			b[3] = math.Max(b[3], box[3])
		}
		b := &page.Blocks[idx]
		if b.Text != "" {
			b.Text += " "
		}
		b.Text += f[11]
		sums[idx] += nums[10]
		counts[idx]++
	}
	if page.Width <= 0 || page.Height <= 0 {
		return page, fmt.Errorf("invalid OCR geometry")
	}
	for i := range page.Blocks {
		c := sums[i] / float64(counts[i]) / 100
		page.Blocks[i].Confidence = &c
		if c < .8 {
			page.Warnings = append(page.Warnings, fmt.Sprintf("第%d页第%d行OCR置信度较低，请核对原页数字与公式。", number, i+1))
		}
	}
	if repaired, ok := repairQuestionReadingPage(page); ok {
		return repaired, nil
	}
	CanonicalPage(&page)
	return page, nil
}
