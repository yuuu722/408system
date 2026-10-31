package service

import (
	"context"
	"errors"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"regexp"
	"strings"
	"time"
)

var subquestionSeparator = regexp.MustCompile(`以及|并且|分别|[；;\n]`)

func planKnowledgeQuestion(question string, names []string) []string {
	out := []string{question}
	seen := map[string]bool{question: true}
	for _, part := range append(subquestionSeparator.Split(question, -1), names...) {
		part = strings.TrimSpace(part)
		if len([]rune(part)) >= 2 && !seen[part] && len(out) < 4 {
			out = append(out, part)
			seen[part] = true
		}
	}
	return out
}
func (s RAGService) kagCandidates(ctx context.Context, uid, kid, question string, names []string, limit int) ([]domain.Chunk, *domain.RetrievalTrace, error) {
	planner := planKnowledgeQuestion(question, names)
	trace := &domain.RetrievalTrace{Mode: "kag", Subquestions: planner, Paths: []domain.KnowledgePath{}, Notice: "图谱路径用于定位证据，不代表其本身证明了因果或结论；回答仅使用校验后的原文。"}
	store, ok := s.Store.(interface {
		RetrieveGraphEvidence(context.Context, string, string, []string) (domain.GraphRetrieval, error)
	})
	if !ok {
		return nil, trace, &domain.RequestError{Status: 503, Message: "KAG图谱检索未接入，请检查Neo4j配置"}
	}
	graphCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	graph, e := store.RetrieveGraphEvidence(graphCtx, uid, kid, append([]string{question}, names...))
	cancel()
	degraded := false
	if e != nil {
		if ctx.Err() != nil {
			return nil, trace, ctx.Err()
		}
		var unavailable *domain.RequestError
		if !(errors.As(e, &unavailable) && unavailable.Status == 503) && !errors.Is(e, context.DeadlineExceeded) {
			return nil, trace, e
		}
		degraded = true
		graph = domain.GraphRetrieval{Paths: []domain.KnowledgePath{}}
		trace.Mode = "rag-degraded"
		trace.Notice = "Neo4j暂不可用，本次仅使用所选知识库的文档检索，不宣称完成图谱推理。"
	}
	trace.Paths = graph.Paths
	if len(graph.Paths) == 0 && !degraded {
		trace.Notice = "本题未匹配到图谱路径，以下仅使用文档检索证据，不宣称已完成图谱推理。"
	}
	queries := append([]string{}, planner...)
	if len(graph.Terms) > 0 {
		terms := graph.Terms
		if len(terms) > 8 {
			terms = terms[:8]
		}
		queries = append(queries, question+"\n关联概念："+strings.Join(terms, "、"))
	}
	vectors, e := s.Embedder.Embed(ctx, queries)
	if e != nil {
		return nil, trace, e
	}
	if len(vectors) != len(queries) {
		return nil, trace, &domain.RequestError{Status: 503, Message: "检索向量数量不正确"}
	}
	chunks := []domain.Chunk{}
	seen := map[string]bool{}
	add := func(c domain.Chunk) {
		if !seen[c.ID] {
			chunks = append(chunks, c)
			seen[c.ID] = true
		}
	}
	// Graph candidates still undergo the same original-question relevance guard.
	for _, c := range graph.Chunks {
		add(c)
	}
	for _, v := range vectors {
		if len(v) == 0 {
			return nil, trace, &domain.RequestError{Status: 503, Message: "检索向量为空"}
		}
		found, e := s.Store.SearchChunks(ctx, uid, kid, v, limit)
		if e != nil {
			return nil, trace, e
		}
		for _, c := range found {
			add(c)
		}
	}
	return chunks, trace, nil
}
