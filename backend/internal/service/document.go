package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/local/exam408-assistant/backend/internal/domain"
)

type Parser struct {
	Endpoint, Token string
	Version         string
	Mode            string
}

func (p Parser) Parse(ctx context.Context, path, mime string) (string, error) {
	pages, err := p.ParsePages(ctx, path, mime)
	if err != nil {
		return "", err
	}
	var texts []string
	for _, page := range pages {
		texts = append(texts, page.Text)
	}
	return strings.Join(texts, "\n\n"), nil
}

func ChunkText(text string, maxRunes, overlap int) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	blocks := regexp.MustCompile(`\n{2,}`).Split(text, -1)
	var out []string
	var current []rune
	flush := func() {
		if len(current) == 0 {
			return
		}
		s := strings.TrimSpace(string(current))
		if s != "" {
			out = append(out, s)
		}
		if overlap > 0 && len(current) > overlap {
			current = append([]rune(nil), current[len(current)-overlap:]...)
		} else {
			current = nil
		}
	}
	for _, block := range blocks {
		r := []rune(strings.TrimSpace(block))
		if len(r) == 0 {
			continue
		}
		for len(r) > 0 {
			room := maxRunes - len(current)
			if room <= 0 {
				flush()
				room = maxRunes - len(current)
			}
			take := len(r)
			if take > room {
				take = room
			}
			if len(current) > 0 {
				current = append(current, '\n', '\n')
			}
			current = append(current, r[:take]...)
			r = r[take:]
			if len(current) >= maxRunes {
				flush()
			}
		}
	}
	if len(current) > 0 {
		out = append(out, strings.TrimSpace(string(current)))
	}
	return out
}

type DocumentService struct {
	EmbeddingSpace string
	Store          domain.Store
	Parser         domain.DocumentParser
	Embedder       domain.EmbeddingProvider
	GraphLLM       domain.LLMProvider
	StorageRoot    string
	MaxBytes       int64
	Log            *slog.Logger
}

var safeName = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func (s *DocumentService) Upload(ctx context.Context, uid, kid, resourceType string, h *multipart.FileHeader, replaceID ...string) (domain.Document, error) {
	kb, err := s.Store.KnowledgeBase(ctx, uid, kid)
	if err != nil {
		return domain.Document{}, err
	}
	if kb.IsPublic {
		u, e := s.Store.UserByID(ctx, uid)
		if e != nil {
			return domain.Document{}, e
		}
		if u.Role != "admin" {
			return domain.Document{}, domain.ErrForbidden
		}
	}
	allowed := map[string]bool{"text/plain": true, "text/markdown": true, "application/pdf": true, "application/msword": true, "application/vnd.openxmlformats-officedocument.wordprocessingml.document": true}
	mime := strings.TrimSpace(strings.Split(h.Header.Get("Content-Type"), ";")[0])
	ext := strings.ToLower(filepath.Ext(h.Filename))
	if mime == "application/octet-stream" || mime == "" {
		if ext == ".pdf" {
			mime = "application/pdf"
		}
		if ext == ".doc" {
			mime = "application/msword"
		}
		if ext == ".docx" {
			mime = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		} else if ext == ".md" || ext == ".markdown" {
			mime = "text/markdown"
		} else if ext == ".txt" {
			mime = "text/plain"
		}
	}
	if !allowed[mime] || !(ext == ".txt" || ext == ".md" || ext == ".markdown" || ext == ".pdf" || ext == ".doc" || ext == ".docx") {
		return domain.Document{}, fmt.Errorf("仅支持 TXT、Markdown、PDF、DOC 和 DOCX（Unsupported file type）")
	}
	if h.Size <= 0 || h.Size > s.MaxBytes {
		return domain.Document{}, &domain.RequestError{Status: 413, Message: fmt.Sprintf("文件须为1字节至%d MB", s.MaxBytes>>20)}
	}
	if resourceType == "" || resourceType == "auto" {
		resourceType = SuggestDocumentCatalog(domain.Document{OriginalFilename: h.Filename}).ResourceType
	}
	purpose := SuggestDocumentCatalog(domain.Document{OriginalFilename: h.Filename, ResourceType: resourceType}).Purpose
	if len(replaceID) > 1 && replaceID[1] != "" {
		purpose = replaceID[1]
	}
	if purpose != "knowledge" && purpose != "questions" && purpose != "both" {
		return domain.Document{}, fmt.Errorf("不支持的资料用途")
	}
	if resourceType == "" {
		resourceType = "other"
	}
	validTypes := map[string]bool{"textbook": true, "notes": true, "past_exam": true, "exercise": true, "mock_exam": true, "solution": true, "courseware": true, "outline": true, "other": true}
	if !validTypes[resourceType] {
		return domain.Document{}, &domain.RequestError{Status: 400, Message: "不支持的资料类型"}
	}
	dir := filepath.Join(s.StorageRoot, uid, kid)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return domain.Document{}, err
	}
	name := uuid.NewString() + "-" + safeName.ReplaceAllString(filepath.Base(h.Filename), "_")
	path := filepath.Join(dir, name)
	src, err := h.Open()
	if err != nil {
		return domain.Document{}, err
	}
	defer src.Close()
	dst, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		return domain.Document{}, err
	}
	copied, copyErr := io.Copy(dst, io.LimitReader(src, s.MaxBytes+1))
	closeErr := dst.Close()
	if copied > s.MaxBytes {
		os.Remove(path)
		return domain.Document{}, &domain.RequestError{Status: 413, Message: "文件超过上传上限"}
	}
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(path)
		if copyErr != nil {
			return domain.Document{}, copyErr
		}
		return domain.Document{}, closeErr
	}
	d := domain.Document{Purpose: purpose, KnowledgeBaseID: kid, UserID: uid, Filename: name, OriginalFilename: filepath.Base(h.Filename), MimeType: mime, Size: h.Size, StoragePath: path, ResourceType: resourceType, Status: "uploaded"}
	if len(replaceID) > 0 && replaceID[0] != "" {
		old, e := s.Store.Document(ctx, uid, replaceID[0])
		if e != nil {
			os.Remove(path)
			return d, e
		}
		d.ID = old.ID
		d.UserID = old.UserID
		if store, ok := s.Store.(interface {
			ReplaceDocument(context.Context, string, domain.Document) (domain.Document, error)
		}); ok {
			d, e = store.ReplaceDocument(ctx, uid, d)
			if e != nil {
				os.Remove(path)
			}
			return d, e
		}
		os.Remove(path)
		return d, fmt.Errorf("文件替换暂不可用")
	}

	d, err = s.Store.CreateDocument(ctx, d)
	if err != nil {
		_ = os.Remove(path)
		return d, err
	}
	if q, ok := s.Store.(interface {
		EnqueueDocument(context.Context, string, string) error
	}); ok {
		if err = q.EnqueueDocument(ctx, uid, d.ID); err != nil {
			return d, err
		}
		d.Status = "processing"
		return d, nil
	}
	if err = s.Store.UpdateDocumentStatus(ctx, d.ID, "processing", ""); err != nil {
		_ = s.Store.DeleteDocument(ctx, uid, d.ID)
		_ = os.Remove(path)
		return d, err
	}
	s.start(d)
	return d, nil
}
func (s *DocumentService) Process(ctx context.Context, d domain.Document) error {
	if store, ok := s.Store.(checkpointStore); ok {
		return s.processRevision(ctx, d, store)
	}
	s.progress(ctx, d.ID, "parsing", 5, 0, 0)
	var pages []TextPage
	var err error
	if parser, ok := s.Parser.(interface {
		ParsePages(context.Context, string, string) ([]TextPage, error)
	}); ok {
		pages, err = parser.ParsePages(ctx, d.StoragePath, d.MimeType)
	} else {
		var text string
		text, err = s.Parser.Parse(ctx, d.StoragePath, d.MimeType)
		pages = []TextPage{{Text: text}}
	}
	if err != nil {
		s.fail(ctx, d.ID, err)
		return err
	}
	if store, ok := s.Store.(interface {
		SaveDocumentStructure(context.Context, string, string, domain.ParsedDocument, []domain.Asset) error
	}); ok {
		points, e := s.Store.ListKnowledgePoints(ctx)
		if e != nil {
			s.fail(ctx, d.ID, e)
			return e
		}
		parsed := domain.ParsedDocument{Pages: pages, Engine: "408-structured-v1"}
		assets := ExtractQuestions(d, pages, points)
		if e = store.SaveDocumentStructure(ctx, d.UserID, d.ID, parsed, assets); e != nil {
			s.fail(ctx, d.ID, e)
			return e
		}
	}
	s.progress(ctx, d.ID, "chunking", 15, 0, 0)
	var parts []string
	var pageNumbers []int
	for _, page := range pages {
		for _, part := range ChunkText(page.Text, 900, 120) {
			parts = append(parts, part)
			pageNumbers = append(pageNumbers, page.Number)
		}
	}
	if len(parts) == 0 {
		err = fmt.Errorf("未提取到有效正文，需要 OCR 或更换文件（No extractable text）")
		s.fail(ctx, d.ID, err)
		return err
	}
	// Progress advances only after successful batches, never on a timer.
	// Small batches also keep local CPU requests below the provider timeout.
	s.progress(ctx, d.ID, "embedding", 20, 0, len(parts))
	var vectors [][]float32
	for start := 0; start < len(parts); start += 4 {
		end := min(start+4, len(parts))
		batch, embedErr := s.Embedder.Embed(ctx, parts[start:end])
		if embedErr != nil {
			s.fail(ctx, d.ID, embedErr)
			return embedErr
		}
		if len(batch) != end-start {
			err = fmt.Errorf("embedding provider returned an incomplete batch")
			s.fail(ctx, d.ID, err)
			return err
		}
		vectors = append(vectors, batch...)
		s.progress(ctx, d.ID, "embedding", 20+70*end/len(parts), end, len(parts))
	}
	chunks := make([]domain.Chunk, len(parts))
	for i, part := range parts {
		chunks[i] = domain.Chunk{ID: uuid.NewString(), DocumentID: d.ID, KnowledgeBaseID: d.KnowledgeBaseID, UserID: d.UserID, Content: part, ChunkIndex: i, SourceLocation: fmt.Sprintf("chunk %d", i+1), Metadata: map[string]any{"original_filename": d.OriginalFilename, "embedding_space": s.EmbeddingSpace, "page": pageNumbers[i]}, Embedding: vectors[i]}
		if pageNumbers[i] > 0 {
			chunks[i].SourceLocation = fmt.Sprintf("PDF第%d页 · 分块%d", pageNumbers[i], i+1)
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	s.progress(ctx, d.ID, "saving", 95, len(parts), len(parts))
	if err = s.Store.ReplaceChunks(ctx, d.UserID, d.ID, chunks); err != nil {
		s.fail(ctx, d.ID, err)
		return err
	}
	if err = s.Store.UpdateDocumentStatus(ctx, d.ID, "ready", ""); err != nil {
		s.logError("document ready status update failed", d.ID, err)
	}
	return err
}
func (s *DocumentService) fail(ctx context.Context, id string, err error) {
	// A timed-out task must still persist its terminal state.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	s.logError("document processing failed", id, err)
	if updateErr := s.Store.UpdateDocumentStatus(ctx, id, "failed", err.Error()); updateErr != nil {
		s.logError("document failure status update failed", id, updateErr)
	}
}
func (s *DocumentService) Reprocess(ctx context.Context, uid, did string) error {
	d, err := s.Store.Document(ctx, uid, did)
	if err != nil {
		return err
	}
	if access, ok := s.Store.(interface {
		CanMaintainDocument(context.Context, string, string) error
	}); ok {
		if err = access.CanMaintainDocument(ctx, uid, did); err != nil {
			return err
		}
	}
	if d.Status == "processing" {
		return fmt.Errorf("document is already processing")
	}
	if q, ok := s.Store.(interface {
		EnqueueDocument(context.Context, string, string) error
	}); ok {
		return q.EnqueueDocument(ctx, uid, d.ID)
	}
	if err = s.Store.UpdateDocumentStatus(ctx, d.ID, "processing", ""); err != nil {
		return err
	}
	s.start(d)
	return nil
}

func (s *DocumentService) progress(ctx context.Context, id, stage string, percent, done, total int) {
	if err := s.Store.UpdateDocumentProgress(ctx, id, stage, percent, done, total); err != nil {
		s.logError("document progress update failed", id, err)
	}
}

func (s *DocumentService) start(d domain.Document) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		s.Process(ctx, d)
	}()
}

func (s *DocumentService) Delete(ctx context.Context, uid, did string) error {
	d, err := s.Store.Document(ctx, uid, did)
	if err != nil {
		return err
	}
	if err = s.Store.DeleteDocument(ctx, uid, did); err != nil {
		return err
	}
	_ = os.Remove(d.StoragePath)
	return nil
}

func (s *DocumentService) logError(message, documentID string, err error) {
	if s.Log != nil {
		s.Log.Error(message, "document_id", documentID, "error", err)
	}
}
