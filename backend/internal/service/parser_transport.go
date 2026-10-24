package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (p Parser) Fingerprint() string {
	version := p.Version
	if version == "" {
		version = os.Getenv("DOCUMENT_PARSER_VERSION")
	}
	if version == "" {
		version = "408-parser-v2"
	}
	return version + "/native-v4-word/" + p.Mode + "/" + p.Endpoint
}
func (p Parser) remotePages(ctx context.Context, path string) ([]TextPage, error) {
	pages, _, e := p.remoteBatch(ctx, path, 1, 0)
	return pages, e
}
func (p Parser) ParsePageBatch(ctx context.Context, path, mime string, start, limit int) ([]TextPage, int, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if pages, total, handled, err := p.wordBatch(ctx, path, start, limit); handled {
		return pages, total, err
	}
	if p.useRemote() && (ext == ".pdf" || ext == ".docx") {
		return p.remoteBatch(ctx, path, start, limit)
	}
	if ext == ".pdf" {
		if pages, total, handled, e := p.nativePDFBatch(ctx, path, start, limit); handled {
			return pages, total, e
		}
	}
	pages, e := p.ParsePages(ctx, path, mime)
	if e != nil {
		return nil, 0, e
	}
	if start < 1 || start > len(pages) {
		return nil, len(pages), fmt.Errorf("页面序号无效（Invalid page）")
	}
	return pages[start-1 : min(start-1+limit, len(pages))], len(pages), nil
}
func (p Parser) remoteBatch(ctx context.Context, path string, start, limit int) ([]TextPage, int, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, 0, e
	}
	defer f.Close()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, e := mw.CreateFormFile("file", filepath.Base(path))
	if e != nil {
		return nil, 0, e
	}
	_ = part // Only multipart headers are buffered; file data is streamed from disk.
	header := append([]byte(nil), body.Bytes()...)
	body.Reset()
	if e = mw.WriteField("start_page", fmt.Sprint(start)); e != nil {
		return nil, 0, e
	}
	if e = mw.WriteField("page_limit", fmt.Sprint(limit)); e != nil {
		return nil, 0, e
	}
	if e = mw.Close(); e != nil {
		return nil, 0, e
	}
	info, e := f.Stat()
	if e != nil {
		return nil, 0, e
	}
	content := io.MultiReader(bytes.NewReader(header), f, bytes.NewReader(body.Bytes()))
	req, e := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(p.Endpoint, "/")+"/parse", content)
	if e != nil {
		return nil, 0, e
	}
	req.ContentLength = int64(len(header)) + info.Size() + int64(body.Len())
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+p.Token)
	resp, e := (&http.Client{Timeout: 30 * time.Minute}).Do(req)
	if e != nil {
		return nil, 0, &PublicError{Status: 503, Retryable: true, Message: "结构化解析服务不可用或超时（Parser unavailable）"}
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, 0, &PublicError{Status: 502, Retryable: resp.StatusCode >= 500, Message: fmt.Sprintf("结构化解析失败，请检查文件及解析服务（Parser HTTP %d）", resp.StatusCode)}
	}
	var result struct {
		domain.ParsedDocument
		TotalPages int
	}
	if e = json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&result); e != nil {
		return nil, 0, fmt.Errorf("解析结果无效或过大（Invalid parse result）")
	}
	if len(result.Pages) == 0 {
		return nil, 0, fmt.Errorf("没有可读取页面（No pages）")
	}
	if limit > 0 && result.TotalPages < 1 {
		return nil, 0, fmt.Errorf("解析服务需升级以支持恢复（Parser upgrade required）")
	}
	return result.Pages, result.TotalPages, nil
}
