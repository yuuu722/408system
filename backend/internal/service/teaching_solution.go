package service

import (
	"context"
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/sourcequality"
	"strings"
)

// Rephrase the reviewed solution, not the student's text. Conditions remain
// mandatory premises and the generated explanation never changes the grade.
func (s RAGService) ExplainReviewedQuestion(ctx context.Context, q domain.Asset, selected ...int) (domain.ChatResult, error) {
	out := domain.ChatResult{}
	model, ok := s.LLM.(jsonAnswerer)
	if !ok {
		return out, fmt.Errorf("讲解模型不支持结构化整理")
	}
	stem := strings.TrimSpace(q.Payload.Stem)
	text := strings.TrimSpace(q.Payload.Explanation)
	if q.Payload.AnswerOrigin != "source" || !domain.HasQuestionExplanation(text) || len([]rune(stem)) > 650 || sourcequality.SuspectFormula(stem) || sourcequality.SuspectFormula(text) {
		return out, fmt.Errorf("原题条件或原文解析不足以生成可靠详解，请先核对原文")
	}
	citation := domain.Citation{DocumentID: q.Payload.DocumentID, DocumentTitle: q.Title, RevisionID: q.Payload.RevisionID, Page: q.Payload.Page, Excerpt: text, Location: "已核对的本题参考解析"}
	if len(q.Payload.Evidence) > 0 {
		citation = q.Payload.Evidence[0]
		citation.Excerpt = text
	}
	premise := domain.Citation{DocumentID: q.Payload.DocumentID, DocumentTitle: q.Title, RevisionID: q.Payload.RevisionID, Page: q.Payload.Page, Excerpt: stem, Location: "原题给定条件（不是通用事实）"}
	contents := []string{text, stem}
	sources := []domain.Citation{citation, premise}
	deps := map[int][]int{1: {2}}
	catalog := factCatalog{Facts: []answerFact{}, Locations: map[int]string{1: citation.Location, 2: premise.Location}}
	for _, sentence := range strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '。' || r == '；' }) {
		sentence = strings.TrimSpace(sentence)
		if len([]rune(sentence)) < 8 || len([]rune(sentence)) > 600 {
			continue
		}
		if !strings.Contains(text, sentence) {
			continue
		}
		catalog.Facts = append(catalog.Facts, answerFact{ID: len(catalog.Facts) + 1, Source: 1, Text: sentence, Premises: []factPremise{{Source: 2, Text: stem}}})
		if len(catalog.Facts) == 8 {
			break
		}
	}
	if len(catalog.Facts) == 0 {
		return out, fmt.Errorf("没有可用于详解的完整原文事实")
	}
	attempt := "学生本次选择暂不确定。"
	if len(selected) > 0 && selected[0] >= 0 && selected[0] < len(q.Payload.Options) {
		attempt = fmt.Sprintf("学生本次选择第%d项：%s；资料参考答案为第%d项。", selected[0]+1, q.Payload.Options[selected[0]], q.Payload.Answer+1)
	}
	question := q.Payload.Stem
	raw, e := model.AnswerJSON(ctx, question+factCompositionInstruction+TeachingInstruction(ctx)+"\n本次作答背景（只用于讲解侧重点，不是新的教材事实，不推测学生心理或未经验证的错误原因）："+attempt+"\n这是已作答题目的讲解，按给定条件、所用定义或公式、推导和结论说明，不补造未给步骤。不重新判分。", catalog.JSON())
	if e != nil {
		return out, e
	}
	answer, citations, verified, reason := s.factComposedAnswer(ctx, raw, question, catalog, contents, sources, deps)
	if !verified {
		return out, fmt.Errorf("老师详解未通过原文核验（%s），仍保留原文解析", reason)
	}
	out.Answer = answer
	out.Sources = citations
	out.Generation = &domain.GenerationTrace{Attempted: true, ProviderReturned: true, Verified: true, Attempts: 1, Facts: len(catalog.Facts), Stage: "complete"}
	return out, nil
}
