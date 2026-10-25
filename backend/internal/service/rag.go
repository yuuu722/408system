package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"log/slog"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type RAGService struct {
	Vision    *OpenAILLM
	Store     domain.Store
	Embedder  domain.EmbeddingProvider
	LLM       domain.LLMProvider
	Knowledge KnowledgeService
	Learning  LearningService
	TopK      int
	MinScore  float64
	Log       *slog.Logger
}

func (s RAGService) Chat(ctx context.Context, uid, kid, cid, question string) (domain.ChatResult, error) {
	return s.ChatStream(ctx, uid, kid, cid, question, nil)
}
func (s RAGService) ChatStream(ctx context.Context, uid, kid, cid, question string, emit func(string, any) error, includePersonal ...bool) (domain.ChatResult, error) {
	question = strings.TrimSpace(question)
	if question == "" || len([]rune(question)) > 4000 {
		return domain.ChatResult{}, fmt.Errorf("question is required")
	}
	if _, err := s.Store.KnowledgeBase(ctx, uid, kid); err != nil {
		return domain.ChatResult{}, err
	}
	var history []domain.Message
	cs, hasHistory := s.Store.(domain.ConversationStore)
	if cid != "" && hasHistory {
		release, e := cs.LockConversation(ctx, uid, cid)
		if e != nil {
			return domain.ChatResult{}, e
		}
		defer release()
		history, e = cs.ConversationMessages(ctx, uid, cid, kid)
		if e != nil {
			return domain.ChatResult{}, e
		}
		history = boundedHistory(history)
	}
	retrievalQuestion := contextualQuestion(question, history)
	evidenceQuestion := retrievalQuestion
	if emit != nil {
		if e := emit("status", map[string]string{"stage": "understanding"}); e != nil {
			return domain.ChatResult{}, e
		}
	}
	detected, detectErr := s.Knowledge.Detect(ctx, question)
	if detectErr != nil {
		s.warn("question understanding failed", "error", detectErr)
	}
	names := []string{}
	for _, point := range detected {
		if len(names) >= 6 {
			break
		}
		names = append(names, point.Name)
	}
	if len(names) > 0 {
		retrievalQuestion += "\n检索概念：" + strings.Join(names, "、")
	}
	if emit != nil {
		if e := emit("understanding", map[string]any{"question": question, "intent": questionIntent(question), "knowledge_points": names}); e != nil {
			return domain.ChatResult{}, e
		}
	}
	if emit != nil {
		if e := emit("status", map[string]string{"stage": "retrieving"}); e != nil {
			return domain.ChatResult{}, e
		}
	}
	vecs, err := s.Embedder.Embed(ctx, []string{retrievalQuestion})
	if err != nil {
		return domain.ChatResult{}, err
	}
	if len(vecs) != 1 || len(vecs[0]) == 0 {
		return domain.ChatResult{}, fmt.Errorf("embedding provider returned an invalid vector response")
	}
	localIndex := false
	if marker, ok := s.Embedder.(interface{ IsLocalIndex() bool }); ok {
		localIndex = marker.IsLocalIndex()
	}
	candidateLimit := s.TopK
	if localIndex {
		candidateLimit = min(100, max(s.TopK, s.TopK*8))
	}
	var chunks []domain.Chunk
	if scoped, ok := s.Store.(interface {
		SearchChunksScoped(context.Context, string, string, []float32, int, bool) ([]domain.Chunk, error)
	}); ok {
		include := true
		if len(includePersonal) > 0 {
			include = includePersonal[0]
		}
		chunks, err = scoped.SearchChunksScoped(ctx, uid, kid, vecs[0], candidateLimit, include)
	} else {
		chunks, err = s.Store.SearchChunks(ctx, uid, kid, vecs[0], candidateLimit)
	}
	if err != nil {
		return domain.ChatResult{}, err
	}
	if localIndex {
		for i := range chunks {
			if math.IsNaN(chunks[i].Score) || math.IsInf(chunks[i].Score, 0) {
				chunks[i].Score = -1
				continue
			}
			chunks[i].Score = lexicalEvidenceScore(evidenceQuestion, chunks[i].Content)
		}
		sort.SliceStable(chunks, func(i, j int) bool { return chunks[i].Score > chunks[j].Score })
	}
	if cid == "" {
		title := []rune(question)
		if len(title) > 40 {
			title = title[:40]
		}
		cid, err = s.Store.CreateConversation(ctx, uid, kid, string(title))
		if err != nil {
			return domain.ChatResult{}, err
		}
	}
	if !hasHistory {
		if err = s.Store.SaveMessage(ctx, uid, cid, "user", question, nil); err != nil {
			return domain.ChatResult{}, err
		}
	}
	var evidence strings.Builder
	var citations []domain.Citation
	for _, c := range chunks {
		if math.IsNaN(c.Score) || math.IsInf(c.Score, 0) || c.Score < s.MinScore || !evidenceRelated(evidenceQuestion, c.Content) {
			continue
		}
		if len(citations) >= s.TopK {
			break
		}
		if strings.TrimSpace(c.SourceLocation) == "" {
			if page := citationPage(c.Metadata); page > 0 {
				c.SourceLocation = fmt.Sprintf("第%d页", page)
			} else {
				c.SourceLocation = fmt.Sprintf("文本片段%d", c.ChunkIndex+1)
			}
		}
		if page := citationPage(c.Metadata); page > 0 && (strings.HasSuffix(strings.ToLower(c.DocumentTitle), ".doc") || strings.HasSuffix(strings.ToLower(c.DocumentTitle), ".docx")) {
			c.SourceLocation = fmt.Sprintf("Word转换第%d页 · %s", page, c.SourceLocation)
		}
		citationNumber := len(citations) + 1
		fmt.Fprintf(&evidence, "[%d] %s（%s）\n%s\n\n", citationNumber, c.DocumentTitle, c.SourceLocation, c.Content)
		excerpt := []rune(c.Content)
		if len(excerpt) > 180 {
			excerpt = excerpt[:180]
		}
		citation := domain.Citation{DocumentID: c.DocumentID, DocumentTitle: c.DocumentTitle, ChunkID: c.ID, Location: c.SourceLocation, Excerpt: string(excerpt), Score: c.Score, Page: citationPage(c.Metadata), LocationQuality: "page"}
		if citation.Page == 0 {
			citation.LocationQuality = "chunk"
		}
		citation.RevisionID, _ = c.Metadata["revision_id"].(string)
		if raw, e := json.Marshal(c.Metadata["regions"]); e == nil {
			_ = json.Unmarshal(raw, &citation.Regions)
		}
		if len(citation.Regions) > 0 {
			citation.LocationQuality = "blocks"
		}
		citations = append(citations, citation)
	}
	answer := "当前知识库中没有找到与问题足够相关的资料，依据不足，暂时无法回答。请确认所选知识库，或上传相关资料后重试。"
	if len(citations) > 0 {
		if emit != nil {
			if e := emit("status", map[string]string{"stage": "generating"}); e != nil {
				return domain.ChatResult{}, e
			}
		}
		// Buffer model output until citation numbers have been checked. The model
		// only edits text evidence; visual inference is not an answer source.
		if llm, ok := s.LLM.(domain.StreamingLLM); ok {
			answer, err = llm.AnswerStream(ctx, question, evidence.String(), history, func(string) error { return ctx.Err() })
		} else {
			answer, err = s.LLM.Answer(ctx, question, evidence.String())
		}
		if err != nil {
			return domain.ChatResult{}, err
		}
		if !validAnswerCitations(answer, len(citations)) {
			answer = "回答的引用标注未通过校验，以下仅展示检索原文，不作为完整结论：\n\n"
			for i, c := range citations {
				answer += fmt.Sprintf("%s [%d]\n\n", c.Excerpt, i+1)
			}
		}
		if emit != nil {
			if e := emit("delta", map[string]string{"text": answer}); e != nil {
				return domain.ChatResult{}, e
			}
		}
	}
	if len(citations) == 0 && emit != nil {
		if e := emit("delta", map[string]string{"text": answer}); e != nil {
			return domain.ChatResult{}, e
		}
	}
	if ctx.Err() != nil {
		return domain.ChatResult{}, ctx.Err()
	}
	if hasHistory {
		if err = cs.SaveTurn(ctx, uid, cid, question, answer, citations); err != nil {
			return domain.ChatResult{}, err
		}
	}
	points, detectErr := s.Knowledge.Detect(ctx, question+" "+evidence.String())
	if detectErr != nil {
		s.warn("knowledge point detection failed", "error", detectErr, "conversation_id", cid)
		points = nil
	}
	for _, p := range points {
		if _, eventErr := s.Learning.Record(ctx, domain.LearningEvent{UserID: uid, SubjectID: p.SubjectID, KnowledgePointID: p.ID, EventType: "rag_question", Metadata: map[string]any{"conversation_id": cid, "question": question}}); eventErr != nil {
			s.warn("RAG learning event failed", "error", eventErr, "conversation_id", cid, "knowledge_point_id", p.ID)
		}
	}
	if !hasHistory {
		if err = s.Store.SaveMessage(ctx, uid, cid, "assistant", answer, citations); err != nil {
			return domain.ChatResult{}, err
		}
	}
	return domain.ChatResult{Answer: answer, ConversationID: cid, Sources: citations, KnowledgePoints: points}, nil
}

func (s RAGService) figureEvidence(ctx context.Context, uid, question string, citations []domain.Citation) ([]string, string, error) {
	wants := false
	for _, term := range []string{"图", "公式", "表格", "电路", "拓扑", "diagram"} {
		if strings.Contains(question, term) {
			wants = true
		}
	}
	store, ok := s.Store.(interface {
		DocumentStructure(context.Context, string, string) (domain.ParsedDocument, error)
	})
	if !wants || !ok {
		return nil, "", nil
	}
	images := []string{}
	notes := ""
	seen := map[string]bool{}
	for i, c := range citations {
		key := fmt.Sprintf("%s:%d", c.DocumentID, c.Page)
		if seen[key] {
			continue
		}
		seen[key] = true
		parsed, e := store.DocumentStructure(ctx, uid, c.DocumentID)
		if e != nil {
			continue
		}
		for _, page := range parsed.Pages {
			if page.Number != c.Page {
				continue
			}
			for _, b := range page.Blocks {
				if b.Image != "" && len(images) < 4 {
					images = append(images, b.Image)
					notes += fmt.Sprintf("\n附图%d来自引用[%d]的第%d页，属于该页上下文。", len(images), i+1, c.Page)
				}
			}
		}
	}
	return images, notes, nil
}

func (s RAGService) warn(message string, args ...any) {
	if s.Log != nil {
		s.Log.Warn(message, args...)
	}
}

func citationPage(meta map[string]any) int {
	switch v := meta["page"].(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return 0
}

var answerCitationPattern = regexp.MustCompile(`\[([0-9]+)\]`)

// This checks citation integrity, not semantic entailment. Source excerpts remain
// available for independent verification of every model-organized answer.
func validAnswerCitations(answer string, count int) bool {
	refs := answerCitationPattern.FindAllStringSubmatch(answer, -1)
	if len(refs) == 0 {
		return false
	}
	for _, ref := range refs {
		n, err := strconv.Atoi(ref[1])
		if err != nil || n < 1 || n > count {
			return false
		}
	}
	return true
}

// Intent is a transparent rule label, never a generated answer or corpus fact.
func questionIntent(question string) string {
	for _, word := range []string{"区别", "比较", "对比", "异同"} {
		if strings.Contains(question, word) {
			return "比较分析"
		}
	}
	q := strings.ReplaceAll(question, "计算机", "")
	for _, word := range []string{"计算", "求解", "多少", "求出"} {
		if strings.Contains(q, word) {
			return "计算求解"
		}
	}
	for _, word := range []string{"为什么", "原理", "如何", "解释", "是什么"} {
		if strings.Contains(question, word) {
			return "概念与原理"
		}
	}
	return "资料问答"
}
