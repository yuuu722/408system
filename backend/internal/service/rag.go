package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/sourcequality"
	"log/slog"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type RAGService struct {
	KAGEnabled bool
	Vision     *OpenAILLM
	Store      domain.Store
	Embedder   domain.EmbeddingProvider
	LLM        domain.LLMProvider
	Knowledge  KnowledgeService
	Learning   LearningService
	TopK       int
	MinScore   float64
	Log        *slog.Logger
}

func (s RAGService) Chat(ctx context.Context, uid, kid, cid, question string) (domain.ChatResult, error) {
	return s.ChatStream(ctx, uid, kid, cid, question, nil)
}
func (s RAGService) ChatStream(ctx context.Context, uid, kid, cid, question string, emit func(string, any) error, includePersonal ...bool) (domain.ChatResult, error) {
	question = strings.TrimSpace(question)
	if question == "" || len([]rune(question)) > 4000 {
		return domain.ChatResult{}, fmt.Errorf("question is required")
	}
	chosen, scopeErr := s.Store.KnowledgeBase(ctx, uid, kid)
	if scopeErr != nil {
		return domain.ChatResult{}, scopeErr
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
		if focus := protocolFocus(evidenceQuestion); len(focus) > 0 && !sharesProtocol(focus, point.Name) {
			continue
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
	candidateLimit := min(100, max(s.TopK, s.TopK*8))
	var chunks []domain.Chunk
	var trace *domain.RetrievalTrace
	if s.KAGEnabled {
		if emit != nil {
			if e := emit("status", map[string]string{"stage": "graph"}); e != nil {
				return domain.ChatResult{}, e
			}
		}
		chunks, trace, err = s.kagCandidates(ctx, uid, kid, evidenceQuestion, names, candidateLimit)
	} else if scoped, ok := s.Store.(interface {
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
	if !chosen.IsPublic {
		bases, e := s.Store.ListKnowledgeBases(ctx, uid)
		if e != nil {
			return domain.ChatResult{}, e
		}
		for _, base := range bases {
			if !base.IsPublic {
				continue
			}
			var extra []domain.Chunk
			if s.KAGEnabled {
				var t *domain.RetrievalTrace
				extra, t, e = s.kagCandidates(ctx, uid, base.ID, evidenceQuestion, names, candidateLimit)
				if t != nil && trace != nil {
					trace.Paths = append(trace.Paths, t.Paths...)
					if t.Mode == "rag-degraded" {
						trace.Mode = t.Mode
						trace.Notice = t.Notice
					}
				}
			} else {
				extra, e = s.Store.SearchChunks(ctx, uid, base.ID, vecs[0], candidateLimit)
			}
			if e != nil {
				return domain.ChatResult{}, e
			}
			chunks = append(chunks, extra...)
		}
	}
	if trace != nil {
		trace.Paths = focusedPaths(evidenceQuestion, trace.Paths)
	}
	if trace != nil && emit != nil {
		if e := emit("reasoning", trace); e != nil {
			return domain.ChatResult{}, e
		}
	}
	if store, ok := s.Store.(interface {
		ExpandTextbookContext(context.Context, string, string, []domain.Chunk) ([]domain.Chunk, error)
	}); ok {
		anchors := []domain.Chunk{}
		for _, c := range chunks {
			if c.Metadata["resource_type"] == "textbook" && c.Score >= s.MinScore && evidenceRelated(evidenceQuestion, chunkIndexText(c)) {
				anchors = append(anchors, c)
			}
		}
		neighbors, e := store.ExpandTextbookContext(ctx, uid, kid, anchors)
		if e != nil {
			s.warn("textbook context lookup failed", "error", e)
		} else {
			chunks = append(chunks, neighbors...)
		}
	}
	// Remove non-evidence before ranking/truncation so high-scoring exercise
	// sheets cannot crowd out textbook explanations from the candidate pool.
	usable := chunks[:0]
	for _, c := range chunks {
		if incomplete, _ := c.Metadata["context_incomplete"].(bool); incomplete {
			continue
		}
		kind, _ := c.Metadata["resource_type"].(string)
		if kind == "outline" || kind == "other" {
			continue
		}
		heading, _ := c.Metadata["heading_path"].(string)
		c.Content = answerEvidence(question, sourcequality.Clean(c.Content), chunkIndexText(c), heading)
		if c.Content != "" {
			usable = append(usable, c)
		}
	}
	chunks = usable
	if localIndex {
		for i := range chunks {
			if math.IsNaN(chunks[i].Score) || math.IsInf(chunks[i].Score, 0) {
				chunks[i].Score = -1
				continue
			}
			chunks[i].Score = lexicalEvidenceScore(evidenceQuestion, chunkIndexText(chunks[i]))
		}
		sort.SliceStable(chunks, func(i, j int) bool { return chunks[i].Score > chunks[j].Score })
	}
	// Authority is applied only after relevance thresholds; a barely related
	// textbook cannot displace a directly relevant source by type alone.
	sort.SliceStable(chunks, func(i, j int) bool {
		ai, aj := chunks[i].Score >= s.MinScore && evidenceRelated(evidenceQuestion, chunkIndexText(chunks[i])), chunks[j].Score >= s.MinScore && evidenceRelated(evidenceQuestion, chunkIndexText(chunks[j]))
		if ai != aj {
			return ai
		}
		ri, rj := sourcePriority(chunks[i]), sourcePriority(chunks[j])
		if ri != rj {
			return ri < rj
		}
		return questionFocusScore(question, chunks[i]) > questionFocusScore(question, chunks[j])
	})
	chunks = balanceWhyEvidence(evidenceQuestion, chunks, s.MinScore)
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
	var sourceContents []string
	dependencies := map[int][]int{}
	appendSource := func(c domain.Chunk) int {
		if strings.TrimSpace(c.SourceLocation) == "" {
			if page := citationPage(c.Metadata); page > 0 {
				c.SourceLocation = fmt.Sprintf("第%d页", page)
			} else {
				c.SourceLocation = fmt.Sprintf("文本片段%d", c.ChunkIndex+1)
			}
		}
		if heading, ok := c.Metadata["heading_path"].(string); ok && heading != "" {
			r := []rune(heading)
			if len(r) > 160 {
				heading = string(r[:160])
			}
			c.SourceLocation += " · " + heading
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
		sourceContents = append(sourceContents, c.Content)
		return citationNumber
	}
	mains := 0
	seenEvidence := map[string]bool{}
	for _, c := range chunks {
		if math.IsNaN(c.Score) || math.IsInf(c.Score, 0) || c.Score < s.MinScore || !evidenceRelated(evidenceQuestion, chunkIndexText(c)) {
			continue
		}
		contexts := premiseChunks(c)
		key := sourceWhitespace(c.Content)
		for _, context := range contexts {
			key += "\x00" + sourceWhitespace(context.Content)
		}
		if seenEvidence[key] {
			continue
		}
		seenEvidence[key] = true
		limit := s.TopK
		if questionIntent(question) == "概念与原理" {
			limit = min(limit, 3)
		}
		if mains >= limit {
			break
		}
		cost := len([]rune(c.Content)) + len([]rune(c.DocumentTitle)) + len([]rune(c.SourceLocation)) + 240
		for _, context := range contexts {
			cost += len([]rune(context.Content)) + 160
		}
		if _, bounded := s.LLM.(jsonAnswerer); bounded && len([]rune(evidence.String()))+cost > 3200 {
			continue
		}
		mains++
		refs := []int{}
		for _, context := range contexts {
			refs = append(refs, appendSource(context))
		}
		main := appendSource(c)
		if len(refs) > 0 {
			dependencies[main] = refs
			fmt.Fprintf(&evidence, "来源[%d]的前提来源为%v；若选择该正文，必须同时选取前提中的条件原句。\n", main, refs)
		}
	}

	catalog := buildAnswerFacts(question, sourceContents, citations, dependencies)
	modelEvidence := catalog.JSON()
	modelFailure := ""
	answer := "当前检索结果没有找到足以回答本题的讲解依据，依据不足，暂时无法回答。目录和未经解析的选择题选项不能作为结论依据。请确认所选知识库中已有相关教材或答案解析；未检索到不代表整个知识库不存在相关内容。"
	if len(citations) > 0 {
		if emit != nil {
			if e := emit("status", map[string]string{"stage": "generating"}); e != nil {
				return domain.ChatResult{}, e
			}
		}
		// Buffer model selections until each complete passage is matched to source text.
		// Provider output is never shown as an unchecked factual paraphrase.
		if model, ok := s.LLM.(jsonAnswerer); ok {
			answer, err = model.AnswerJSON(ctx, contextualQuestion(question, history)+factCompositionInstruction, modelEvidence)
		} else if llm, ok := s.LLM.(domain.StreamingLLM); ok {
			answer, err = llm.AnswerStream(ctx, question+composedInstruction, evidence.String(), history, func(string) error { return ctx.Err() })
		} else {
			answer, err = s.LLM.Answer(ctx, question+composedInstruction, evidence.String())
		}
		var incomplete *PublicError
		if errors.As(err, &incomplete) && incomplete.Code == "answer_incomplete" {
			if model, ok := s.LLM.(jsonAnswerer); ok {
				answer, err = model.AnswerJSON(ctx, question+factCompositionInstruction+"\n请缩短输出，先完整回答关键原因和机制，不要在半句处结束。", modelEvidence)
			}
		}
		if err != nil {
			if ctx.Err() != nil {
				return domain.ChatResult{}, ctx.Err()
			}
			s.warn("model synthesis failed", "error", err)
			var public *PublicError
			if errors.As(err, &public) {
				modelFailure = public.Message
			} else {
				modelFailure = "模型整理服务暂不可用"
			}
			answer = ""
		}
		if emit != nil {
			if e := emit("status", map[string]string{"stage": "verifying"}); e != nil {
				return domain.ChatResult{}, e
			}
		}
		// One repair pass for real JSON-capable models. Do not silently present
		// source excerpts as a successful model-composed answer.
		text, sources, verified := "", []domain.Citation(nil), false
		rejectReason := ""
		if _, ok := s.LLM.(jsonAnswerer); ok {
			text, sources, verified, rejectReason = s.factComposedAnswer(ctx, answer, question, catalog, sourceContents, citations, dependencies)
		} else {
			text, sources, verified = s.composedAnswer(ctx, answer, question, sourceContents, citations, dependencies)
		}
		if !verified && modelFailure == "" {
			if model, ok := s.LLM.(jsonAnswerer); ok {
				repaired, e := model.AnswerJSON(ctx, question+factCompositionInstruction+"\n上次输出未通过核验（"+rejectReason+"）。必须填写有效facts编号，保留条件，用更短的完整句子重新整理。", modelEvidence)
				if e == nil {
					text, sources, verified, rejectReason = s.factComposedAnswer(ctx, repaired, question, catalog, sourceContents, citations, dependencies)
				}
				if !verified {
					answer = ""
				}
			}
		}
		if !verified && modelFailure == "" {
			s.warn("answer grounding rejected", "reason", rejectReason)
		}
		if verified {
			answer, citations = text, sources
		} else if text, sources, ok := selectedSourceAnswer(answer, sourceContents, citations, dependencies); ok && adequateCoverage(question, text, sourceContents) {
			answer, citations = text, sources
		} else {
			if _, realModel := s.LLM.(jsonAnswerer); realModel {
				fallback, originalSources, _ := sentenceAnswer("", question, sourceContents, citations, dependencies)
				if len(originalSources) > 0 && adequateCoverage(question, fallback, sourceContents) {
					notice := "模型整理未通过核验"
					if modelFailure != "" {
						notice = "模型服务未完成整理"
					}
					answer = notice + "，以下为教材中可核对的完整原句（不是模型生成答案）：\n\n" + fallback
					citations = originalSources
				} else {
					answer = "本次未能生成通过教材依据与完整性核验的整理回答。检索到的原文来源保留在下方；需补充完整教材上下文后再回答。"
					if tcpReliabilityQuestion(question) {
						answer = "当前检索片段不足以完整解释TCP的可靠传输机制。需要教材中关于TCP序号、确认和重传机制的完整讲解；不会用HDLC或无线链路的资料替代。"
					}
				}
				if modelFailure != "" {
					answer = strings.TrimRight(modelFailure, "。.!！ ") + "。\n\n" + answer
				}
			} else {
				answer, citations, _ = sentenceAnswer(answer, question, sourceContents, citations, dependencies)
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
	related := []domain.Asset{}
	if store, ok := s.Store.(interface {
		RelatedQuestions(context.Context, string, string, []string) ([]domain.Asset, error)
	}); ok {
		ids := []string{}
		for _, point := range detected {
			ids = append(ids, point.ID)
		}
		var relatedErr error
		related, relatedErr = store.RelatedQuestions(ctx, uid, kid, ids)
		if relatedErr != nil {
			s.warn("related question lookup failed", "error", relatedErr)
		}
	}
	return domain.ChatResult{RelatedQuestions: related, Reasoning: trace, Answer: answer, ConversationID: cid, Sources: citations, KnowledgePoints: points}, nil
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
