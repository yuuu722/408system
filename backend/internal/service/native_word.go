package service

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// One renderer per process limits peak memory; document job fencing prevents
// multiple workers from publishing the same revision. Finished PDFs are cached.
var wordRenderer = make(chan struct{}, 1)

func (p Parser) wordBatch(ctx context.Context, path string, start, limit int) ([]TextPage, int, bool, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".doc" && ext != ".docx" {
		return nil, 0, false, nil
	}
	if ext == ".docx" && p.useRemote() {
		return nil, 0, false, nil
	}
	if ext == ".docx" {
		z, e := zip.OpenReader(path)
		if e != nil {
			return nil, 0, true, fmt.Errorf("DOCX文件损坏或格式不正确")
		}
		officePackage := false
		for _, f := range z.File {
			if f.Name == "[Content_Types].xml" {
				officePackage = true
				break
			}
		}
		z.Close()
		if !officePackage {
			return nil, 0, false, nil
		} // Legacy minimal text-only packages.
	}
	tool, err := exec.LookPath("soffice")
	if err != nil {
		tool, err = exec.LookPath("libreoffice")
	}
	if err != nil {
		if ext == ".docx" {
			return nil, 0, false, nil
		} // Text-only fallback for source development.
		return nil, 0, true, fmt.Errorf("旧版Word解析需要LibreOffice，请更新并重建后端及worker镜像")
	}
	pdf, err := renderWordPDF(ctx, path, tool)
	if err != nil {
		return nil, 0, true, err
	}
	var pages []TextPage
	var total int
	if p.useRemote() {
		pages, total, err = p.remoteBatch(ctx, pdf, start, limit)
	} else {
		var handled bool
		pages, total, handled, err = p.nativePDFBatch(ctx, pdf, start, limit)
		if !handled {
			err = fmt.Errorf("Word解析需要PDF提取和OCR工具，请重建后端及worker镜像")
		}
	}
	for i := range pages {
		pages[i].Segment = pages[i].Number
		pages[i].Warnings = append(pages[i].Warnings, "Word转换页码：来自服务端排版，不保证与Word/WPS原始页码一致；请下载原文件核对。")
	}
	return pages, total, true, err
}

func renderWordPDF(ctx context.Context, path, tool string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	key := sha256.Sum256([]byte(fmt.Sprintf("word-v1:%s:%d:%d", path, info.Size(), info.ModTime().UnixNano())))
	cache := filepath.Join(filepath.Dir(path), fmt.Sprintf(".word-preview-%x", key[:12]))
	result := filepath.Join(cache, "preview.pdf")
	select {
	case wordRenderer <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	defer func() { <-wordRenderer }()
	if f, e := os.Open(result); e == nil {
		head := make([]byte, 5)
		_, e = io.ReadFull(f, head)
		f.Close()
		if e == nil && string(head) == "%PDF-" {
			return result, nil
		}
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".docx" {
		z, e := zip.OpenReader(path)
		if e != nil {
			return "", fmt.Errorf("DOCX文件损坏或格式不正确")
		}
		var expanded uint64
		valid := len(z.File) <= 20000
		for _, f := range z.File {
			if f.UncompressedSize64 > 4<<30 || expanded > 4<<30-f.UncompressedSize64 {
				valid = false
				break
			}
			expanded += f.UncompressedSize64
			if strings.HasSuffix(f.Name, ".xml") && f.UncompressedSize64 > 128<<20 {
				valid = false
				break
			}
		}
		z.Close()
		if !valid {
			return "", fmt.Errorf("DOCX解压内容过大，请拆分文档")
		}
	} else {
		f, e := os.Open(path)
		if e != nil {
			return "", e
		}
		head := make([]byte, 8)
		_, e = io.ReadFull(f, head)
		f.Close()
		if e != nil || string(head) != "\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1" {
			return "", fmt.Errorf("DOC格式无效：请上传真正的Word 97–2003文档，或另存为DOCX")
		}
	}
	dir, err := os.MkdirTemp("", "exam408-word-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	source := filepath.Join(dir, "input"+ext)
	in, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.OpenFile(source, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(out, in)
	closeErr := out.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	profile := filepath.Join(dir, "profile")
	if err = os.MkdirAll(filepath.Join(profile, "user"), 0700); err != nil {
		return "", err
	}
	// Never execute document macros or automatically update external fields/links.
	settings := `<?xml version="1.0" encoding="UTF-8"?><oor:items xmlns:oor="http://openoffice.org/2001/registry"><item oor:path="/org.openoffice.Office.Common/Security/Scripting"><prop oor:name="MacroSecurityLevel" oor:op="fuse"><value>3</value></prop></item><item oor:path="/org.openoffice.Office.Writer/Content/Update"><prop oor:name="Link" oor:op="fuse"><value>0</value></prop></item></oor:items>`
	if err = os.WriteFile(filepath.Join(profile, "user", "registrymodifications.xcu"), []byte(settings), 0600); err != nil {
		return "", err
	}
	renderCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	profileURL := (&url.URL{Scheme: "file", Path: profile}).String()
	cmd := exec.CommandContext(renderCtx, tool, "-env:UserInstallation="+profileURL, "--headless", "--nologo", "--nodefault", "--norestore", "--convert-to", "pdf:writer_pdf_Export", "--outdir", dir, source)
	if _, err = cmd.Output(); err != nil {
		if renderCtx.Err() != nil {
			return "", fmt.Errorf("Word转换超时或任务取消，请拆分文档后重试")
		}
		return "", fmt.Errorf("Word转换失败：文件可能已加密、损坏或格式不受支持")
	}
	generated := filepath.Join(dir, "input.pdf")
	f, err := os.Open(generated)
	if err != nil {
		return "", fmt.Errorf("Word转换未生成PDF，请检查文件是否加密或损坏")
	}
	defer f.Close()
	if err = os.MkdirAll(cache, 0700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(cache, "preview-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	_, err = io.Copy(tmp, f)
	closeErr = tmp.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err = os.Rename(tmp.Name(), result); err != nil {
		return "", err
	}
	return result, nil
}
