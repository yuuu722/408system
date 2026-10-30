package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"io"
	"math"
	"os"
	"strings"
)

type checkpointStore interface {
	BeginRevision(context.Context, domain.Document, domain.RevisionSpec) (domain.Revision, error)
	SaveRevisionPage(context.Context, domain.Document, string, int, int, domain.StructuredPage) error
	RevisionPages(context.Context, string, string, string) ([]domain.StructuredPage, error)
	CompleteRevisionParse(context.Context, domain.Document, string) error
	SaveRevisionChunks(context.Context, domain.Document, string, []domain.Chunk) error
	PendingRevisionChunks(context.Context, domain.Document, string, int) ([]domain.Chunk, error)
	SaveRevisionVectors(context.Context, domain.Document, string, []domain.Chunk, [][]float32) error
	RevisionCounts(context.Context, string) (int, int, error)
	RevisionProgress(context.Context, domain.Document, string, int, int, int) error
	ActivateRevision(context.Context, domain.Document, string, domain.ParsedDocument, []domain.Asset) error
}

func digest(v string) string { sum := sha256.Sum256([]byte(v)); return hex.EncodeToString(sum[:]) }
func fileDigest(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func (s *DocumentService) processRevision(ctx context.Context, d domain.Document, store checkpointStore) error {
	hash, e := fileDigest(d.StoragePath)
	if e != nil {
		return e
	}
	parserKey := "408-parser-v2"
	if p, ok := s.Parser.(interface{ Fingerprint() string }); ok {
		parserKey = p.Fingerprint()
	}
	parseKey := digest(hash + "/" + parserKey)
	chunkKey := digest(parseKey + "/context-v2/900/120")
	points, e := s.Store.ListKnowledgePoints(ctx)
	if e != nil {
		return e
	}
	raw, _ := json.Marshal(points)
	spec := domain.RevisionSpec{FileHash: hash, ParseKey: parseKey, ChunkKey: chunkKey, EmbeddingSpace: s.EmbeddingSpace, Fingerprint: digest(chunkKey + "/" + s.EmbeddingSpace + "/extract-v2/" + digest(string(raw)))}
	rev, e := store.BeginRevision(ctx, d, spec)
	if e != nil {
		return e
	}
	progress := func(stage string, percent, done, total int) error {
		return store.RevisionProgress(ctx, d, stage, percent, done, total)
	}
	pages, e := store.RevisionPages(ctx, d.UserID, d.ID, rev.ID)
	if e != nil {
		return e
	}
	if !rev.ParsedDone {
		if e = progress("parsing", 5, 0, 0); e != nil {
			return e
		}
		if batch, ok := s.Parser.(interface {
			ParsePageBatch(context.Context, string, string, int, int) ([]TextPage, int, error)
		}); ok {
			next := len(pages) + 1
			total := rev.TotalPages
			for total == 0 || next <= total {
				part, n, err := batch.ParsePageBatch(ctx, d.StoragePath, d.MimeType, next, 1)
				if err != nil {
					return err
				}
				if n < next || len(part) != 1 || total != 0 && total != n {
					return fmt.Errorf("解析分页结果不完整（Invalid page batch）")
				}
				total = n
				for _, page := range part {
					page.Segment = next - 1
					CanonicalPage(&page)
					if err = store.SaveRevisionPage(ctx, d, rev.ID, next-1, total, page); err != nil {
						return err
					}
					pages = append(pages, page)
					next++
				}
				if e = progress("parsing", 5+10*len(pages)/total, 0, 0); e != nil {
					return e
				}
			}
		} else {
			var part []TextPage
			if parser, ok := s.Parser.(interface {
				ParsePages(context.Context, string, string) ([]TextPage, error)
			}); ok {
				part, e = parser.ParsePages(ctx, d.StoragePath, d.MimeType)
			} else {
				var text string
				text, e = s.Parser.Parse(ctx, d.StoragePath, d.MimeType)
				part = []TextPage{{Text: text}}
			}
			if e != nil {
				return e
			}
			for i, page := range part {
				page.Segment = i
				CanonicalPage(&page)
				if e = store.SaveRevisionPage(ctx, d, rev.ID, i, len(part), page); e != nil {
					return e
				}
				part[i] = page
			}
			pages = part
		}
		if e = store.CompleteRevisionParse(ctx, d, rev.ID); e != nil {
			return e
		}
	}
	if !rev.Chunked {
		chunks := ContextualChunks(pages, rev.ID, s.EmbeddingSpace, d.OriginalFilename)
		if len(chunks) == 0 {
			return fmt.Errorf("未提取到有效正文，可查看原文（No extractable text）")
		}
		if e = store.SaveRevisionChunks(ctx, d, rev.ID, chunks); e != nil {
			return e
		}
	}
	for {
		total, done, err := store.RevisionCounts(ctx, rev.ID)
		if err != nil {
			return err
		}
		if e = progress("embedding", 20+70*done/max(1, total), done, total); e != nil {
			return e
		}
		chunks, err := store.PendingRevisionChunks(ctx, d, rev.ID, 4)
		if err != nil {
			return err
		}
		if len(chunks) == 0 {
			break
		}
		texts := make([]string, len(chunks))
		for i, c := range chunks {
			texts[i] = chunkIndexText(c)
		}
		vectors, err := s.Embedder.Embed(ctx, texts)
		if err != nil {
			return err
		}
		if err = validateVectors(vectors, len(chunks)); err != nil {
			return err
		}
		if e = store.SaveRevisionVectors(ctx, d, rev.ID, chunks, vectors); e != nil {
			return e
		}
	}
	if current, err := fileDigest(d.StoragePath); err != nil || current != hash {
		return fmt.Errorf("源文件已改变，请重新处理（Source changed）")
	}
	parsed := domain.ParsedDocument{RevisionID: rev.ID, Pages: pages, Engine: parserKey}
	if e = store.ActivateRevision(ctx, d, rev.ID, parsed, ExtractQuestions(d, pages, points)); e != nil {
		return e
	}
	s.extractGraphRelations(ctx, d, pages, points)
	return nil
}
func validateVectors(vectors [][]float32, n int) error {
	if len(vectors) != n || n == 0 {
		return fmt.Errorf("向量批次不完整（Incomplete embeddings）")
	}
	dim := len(vectors[0])
	if dim == 0 {
		return fmt.Errorf("向量为空（Empty embedding）")
	}
	for _, v := range vectors {
		if len(v) != dim {
			return fmt.Errorf("向量维度不一致（Embedding dimension mismatch）")
		}
		nonzero := false
		for _, x := range v {
			if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
				return fmt.Errorf("向量含无效数值（Invalid embedding values）")
			}
			nonzero = nonzero || x != 0
		}
		if !nonzero {
			return fmt.Errorf("向量无有效方向（Zero embedding）")
		}
	}
	return nil
}

// Canonical text is derived from the same ordered blocks used for positioning.
func CanonicalPage(p *TextPage) {
	if len(p.Blocks) == 0 && p.Text != "" {
		p.Blocks = []domain.DocumentBlock{{ID: "text", Kind: "text", Text: p.Text}}
	}
	var text strings.Builder
	offset := 0
	for i := range p.Blocks {
		b := &p.Blocks[i]
		b.ID = fmt.Sprintf("%d:%d:%s", p.Segment, i, b.ID)
		b.Start = offset
		if b.Text != "" {
			if offset > 0 {
				text.WriteByte('\n')
				offset++
			}
			b.Start = offset
			text.WriteString(b.Text)
			offset += len([]rune(b.Text))
		}
		b.End = offset
	}
	p.Text = text.String()
}
func MappedChunks(page TextPage, rid, space string) []domain.Chunk {
	runes := []rune(page.Text)
	out := []domain.Chunk{}
	for start := 0; start < len(runes); {
		end := min(start+900, len(runes))
		hardBoundary := false
		cursor := start
		for _, line := range strings.Split(string(runes[start:end]), "\n") {
			trimmed := strings.TrimSpace(line)
			if cursor > start && len([]rune(trimmed)) <= 80 && chapterHeading.MatchString(trimmed) && !strings.ContainsAny(trimmed, "。！？；") {
				prose := false
				for _, prefixLine := range strings.Split(string(runes[start:cursor]), "\n") {
					prefixLine = strings.TrimSpace(prefixLine)
					if prefixLine != "" && !chapterHeading.MatchString(prefixLine) {
						prose = true
						break
					}
				}
				if prose {
					end = cursor
					hardBoundary = true
					break
				}
			}
			cursor += len([]rune(line)) + 1
		}
		if end < len(runes) && !hardBoundary {
			for boundary := end - 1; boundary > start+450; boundary-- {
				if strings.ContainsRune("。！？；\n", runes[boundary]) {
					end = boundary + 1
					break
				}
			}
		}
		if strings.TrimSpace(string(runes[start:end])) == "" {
			if end == len(runes) {
				break
			}
			start = end
			continue
		}
		regions := []domain.SourceRegion{}
		for _, b := range page.Blocks {
			if b.End > start && b.Start < end {
				bounds := b.Box
				if page.CoordinateSpace != "normalized-top-left-displayed-page" {
					bounds = nil
				}
				regions = append(regions, domain.SourceRegion{Page: page.Number, Segment: page.Segment, BlockID: b.ID, Start: max(start, b.Start), End: min(end, b.End), Box: bounds})
			}
		}
		out = append(out, domain.Chunk{Content: string(runes[start:end]), Metadata: map[string]any{"page": page.Number, "revision_id": rid, "embedding_space": space, "regions": regions, "offset_unit": "unicode_codepoint", "start": start, "end": end}})
		if end == len(runes) {
			break
		}
		if hardBoundary {
			start = end
			continue
		}
		next := max(start+1, end-120)
		originalNext := next
		for next < end && !strings.ContainsRune("。！？；\n", runes[next-1]) {
			next++
		}
		if next == end {
			next = originalNext
		}
		start = next
	}
	return out
}

// extractGraphRelations asks the configured model for candidate relationships,
// then delegates all persistence to a validator that requires canonical point
// codes, an allowed relation type, high confidence, and exact source evidence.
// Extraction is best-effort: a model outage must not invalidate indexed text.
func (s *DocumentService) extractGraphRelations(ctx context.Context, d domain.Document, pages []TextPage, points []domain.KnowledgePoint) {
	if s.GraphLLM == nil || len(points) < 2 {
		return
	}
	store, ok := s.Store.(domain.GraphCandidateStore)
	if !ok {
		return
	}

	var pointList strings.Builder
	for _, p := range points {
		fmt.Fprintf(&pointList, "- %s: %s\n", p.Code, p.Name)
	}
	var source strings.Builder
	for _, page := range pages {
		if source.Len() >= 12000 {
			break
		}
		source.WriteString(page.Text)
		source.WriteString("\n\n")
	}
	text := []rune(source.String())
	if len(text) > 12000 {
		source.Reset()
		source.WriteString(string(text[:12000]))
	}
	if strings.TrimSpace(source.String()) == "" {
		return
	}

	prompt := "你是计算机考研教材知识图谱候选抽取器。只能把明确有原文证据的内容作为候选，不得臆造。\n" +
		"entities 用于提取资料中的概念、算法、数据结构、协议、公式、机制或组件。entity_type 只能是 concept、algorithm、data_structure、protocol、formula、mechanism、component。实体名称要简洁且具体；evidence 必须是原文中连续、逐字一致的 12 至 300 个字符，并且包含该实体名称。\n" +
		"relations 用于提取明确支持的关系。relation_type 只能是 prerequisite、related、part_of、implements、uses、applies_to。若关系端点属于下面的规范知识点，优先填写其 from_point_code/to_point_code；若端点是本次 entities 中的新候选，则填写 from_entity_name/to_entity_name。prerequisite 的 from 端表示先修知识点，to 端表示后续知识点。关系 evidence 必须是原文中连续、逐字一致的 12 至 300 个字符，并且同一证据片段所在文本块应同时包含关系两端的名称。confidence 为 0 到 1 的保守置信度。\n" +
		"只输出合法 JSON，不要 Markdown 或额外解释。格式：{\"entities\":[{\"name\":\"...\",\"entity_type\":\"concept\",\"description\":\"...\",\"evidence\":\"原文连续摘录且包含实体名称\",\"confidence\":0.85}],\"relations\":[{\"from_point_code\":\"...\",\"to_point_code\":\"...\",\"from_entity_name\":\"...\",\"to_entity_name\":\"...\",\"relation_type\":\"related\",\"evidence\":\"原文连续摘录\",\"confidence\":0.85}]}。无可靠候选时返回空数组。\n" +
		"规范知识点代码与名称：\n" + pointList.String() + "\n资料原文：\n" + source.String()
	raw, err := s.GraphLLM.Answer(ctx, prompt, "")
	if err != nil {
		s.logError("graph candidate extraction failed", d.ID, err)
		return
	}
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(strings.TrimSpace(raw), "```")
	var response struct {
		Entities []struct {
			Name        string  `json:"name"`
			Type        string  `json:"entity_type"`
			Description string  `json:"description"`
			Evidence    string  `json:"evidence"`
			Confidence  float64 `json:"confidence"`
		} `json:"entities"`
		Relations []struct {
			FromCode   string  `json:"from_point_code"`
			ToCode     string  `json:"to_point_code"`
			FromName   string  `json:"from_entity_name"`
			ToName     string  `json:"to_entity_name"`
			Type       string  `json:"relation_type"`
			Evidence   string  `json:"evidence"`
			Confidence float64 `json:"confidence"`
		} `json:"relations"`
	}
	if err = json.Unmarshal([]byte(raw), &response); err != nil {
		s.logError("graph candidate JSON invalid", d.ID, err)
		return
	}
	if len(response.Entities) > 200 {
		response.Entities = response.Entities[:200]
	}
	if len(response.Relations) > 200 {
		response.Relations = response.Relations[:200]
	}
	entityCandidates := make([]domain.GraphEntityCandidate, 0, len(response.Entities))
	for _, e := range response.Entities {
		entityCandidates = append(entityCandidates, domain.GraphEntityCandidate{Name: e.Name, EntityType: e.Type, Description: e.Description, Evidence: e.Evidence, Confidence: e.Confidence})
	}
	relationCandidates := make([]domain.GraphRelationCandidate, 0, len(response.Relations))
	for _, r := range response.Relations {
		relationCandidates = append(relationCandidates, domain.GraphRelationCandidate{FromPointCode: r.FromCode, ToPointCode: r.ToCode, FromName: r.FromName, ToName: r.ToName, RelationType: r.Type, Evidence: r.Evidence, Confidence: r.Confidence})
	}
	if err = store.AddValidatedGraphCandidates(ctx, d.UserID, d.KnowledgeBaseID, d.ID, entityCandidates, relationCandidates); err != nil {
		s.logError("graph candidate validation failed", d.ID, err)
	}
}
