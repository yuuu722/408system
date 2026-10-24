package service

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	pdf "github.com/ledongthuc/pdf"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"golang.org/x/text/encoding/simplifiedchinese"
	textunicode "golang.org/x/text/encoding/unicode"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

type TextPage = domain.StructuredPage

func ValidBody(text string) bool {
	letters := 0
	unique := map[rune]bool{}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			letters++
			unique[r] = true
		}
	}
	return letters >= 15 && len(unique) >= 10
}

func (p Parser) ParsePages(ctx context.Context, path, mime string) (pages []TextPage, err error) {
	ext := strings.ToLower(filepath.Ext(path))
	if result, _, handled, e := p.wordBatch(ctx, path, 1, 0); handled {
		return result, e
	}
	if p.useRemote() && (ext == ".pdf" || ext == ".docx") {
		return p.remotePages(ctx, path)
	}
	defer func() {
		if recover() != nil {
			pages = nil
			err = fmt.Errorf("文件解析失败，请检查文件格式（Invalid document）")
		}
	}()
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		f, r, e := pdf.Open(path)
		if e != nil {
			return nil, e
		}
		defer f.Close()
		if r.NumPage() > 2000 {
			return nil, fmt.Errorf("文件超过2000页，请按章节拆分（Too many pages）")
		}
		for i := 1; i <= r.NumPage(); i++ {
			if e = ctx.Err(); e != nil {
				return nil, e
			}
			t, e := r.Page(i).GetPlainText(nil)
			if e != nil {
				return nil, e
			}
			if !ValidBody(t) {
				if i <= 2 || i == r.NumPage() {
					continue
				}
				return nil, fmt.Errorf("PDF第%d页未提取到足够有效正文，请进行OCR或检查页面（OCR or review required）", i)
			}
			pages = append(pages, TextPage{Number: i, Text: t})
		}
	case ".docx":
		z, e := zip.OpenReader(path)
		if e != nil {
			return nil, e
		}
		defer z.Close()
		for _, file := range z.File {
			if file.Name != "word/document.xml" {
				continue
			}
			if file.UncompressedSize64 > 128<<20 {
				return nil, fmt.Errorf("DOCX正文过大（Document too large）")
			}
			f, e := file.Open()
			if e != nil {
				return nil, e
			}
			defer f.Close()
			dec := xml.NewDecoder(io.LimitReader(f, 128<<20))
			var b strings.Builder
			inText := false
			for {
				token, e := dec.Token()
				if e == io.EOF {
					break
				}
				if e != nil {
					return nil, e
				}
				switch v := token.(type) {
				case xml.StartElement:
					if v.Name.Local == "t" {
						inText = true
					}
				case xml.CharData:
					if inText {
						b.Write(v)
					}
				case xml.EndElement:
					if v.Name.Local == "t" {
						inText = false
					}
					if v.Name.Local == "p" {
						b.WriteString("\n\n")
					}
				}
			}
			if !ValidBody(b.String()) {
				return nil, fmt.Errorf("DOCX没有足够有效文字，图片型文档需要OCR（OCR required）")
			}
			pages = append(pages, TextPage{Text: b.String()})
		}
	default:
		b, e := os.ReadFile(path)
		if e != nil {
			return nil, e
		}
		warning := ""
		if !utf8.Valid(b) {
			var decoded []byte
			var decodeErr error
			if len(b) > 1 && b[0] == 0xff && b[1] == 0xfe {
				decoded, decodeErr = textunicode.UTF16(textunicode.LittleEndian, textunicode.ExpectBOM).NewDecoder().Bytes(b)
			} else if len(b) > 1 && b[0] == 0xfe && b[1] == 0xff {
				decoded, decodeErr = textunicode.UTF16(textunicode.BigEndian, textunicode.ExpectBOM).NewDecoder().Bytes(b)
			} else {
				decoded, decodeErr = simplifiedchinese.GB18030.NewDecoder().Bytes(b)
			}
			if decodeErr != nil || strings.ContainsRune(string(decoded), '\uFFFD') {
				return nil, fmt.Errorf("文本编码无法识别，请另存为UTF-8后上传")
			}
			b = decoded
			warning = "文本由UTF-16或GB18030转换为UTF-8，请核对文字。"
		}
		b = []byte(strings.TrimPrefix(string(b), "\uFEFF"))
		if ValidBody(string(b)) {
			page := TextPage{Text: string(b)}
			if warning != "" {
				page.Warnings = []string{warning}
			}
			pages = append(pages, page)
		}
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("未提取到有效正文，需要OCR或补充文字（No usable body text）")
	}
	return pages, nil
}
