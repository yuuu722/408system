package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"log/slog"
	"math"
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
	var chunks []domain.Chunk
	if scoped, ok := s.Store.(interface {
		SearchChunksScoped(context.Context, string, string, []float32, int, bool) ([]domain.Chunk, error)
	}); ok {
		include := true
		if len(includePersonal) > 0 {
			include = includePersonal[0]
		}
		chunks, err = scoped.SearchChunksScoped(ctx, uid, kid, vecs[0], s.TopK, include)
	} else {
		chunks, err = s.Store.SearchChunks(ctx, uid, kid, vecs[0], s.TopK)
	}
	if err != nil {
		return domain.ChatResult{}, err
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
		if math.IsNaN(c.Score) || math.IsInf(c.Score, 0) || c.Score < s.MinScore || !evidenceRelated(retrievalQuestion, c.Content) {
			continue
		}
		if len(citations) >= s.TopK {
			break
		}
		citationNumber := len(citations) + 1
		fmt.Fprintf(&evidence, "[%d] %s（%s）\n%s\n\n", citationNumber, c.DocumentTitle, c.SourceLocation, c.Content)
		excerpt := []rune(c.Content)
		if len(excerpt) > 180 {
			excerpt = excerpt[:180]
		}
		citation := domain.Citation{DocumentID: c.DocumentID, DocumentTitle: c.DocumentTitle, ChunkID: c.ID, Location: c.SourceLocation, Excerpt: string(excerpt), Score: c.Score, Page: citationPage(c.Metadata), LocationQuality: "page"}
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
		images, imageEvidence, imageErr := s.figureEvidence(ctx, uid, question, citations)
		if imageErr != nil {
			return domain.ChatResult{}, imageErr
		}
		if len(images) > 0 {
			if s.Vision == nil || s.Vision.APIKey == "" {
				return domain.ChatResult{}, &PublicError{Status: 503, Message: "本次问题需要理解原图，视觉模型尚未配置。可先查看引用原文（Vision model required）"}
			}
			answer, err = s.Vision.answerWithImages(ctx, question, evidence.String()+imageEvidence, history, images, func(delta string) error {
				if emit != nil {
					return emit("delta", map[string]string{"text": delta})
				}
				return nil
			})
		} else if llm, ok := s.LLM.(domain.StreamingLLM); ok {
			answer, err = llm.AnswerStream(ctx, question, evidence.String(), history, func(delta string) error {
				if emit != nil {
					return emit("delta", map[string]string{"text": delta})
				}
				return nil
			})
		} else {
			answer, err = s.LLM.Answer(ctx, question, evidence.String())
			if err == nil && emit != nil {
				err = emit("delta", map[string]string{"text": answer})
			}
		}
		if err != nil {
			return domain.ChatResult{}, err
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
