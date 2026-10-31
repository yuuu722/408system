package httpapi

import (
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/service"
	"net/http"
	"strings"
)

// Cross-document links are explicit and quote-checked. Matching a question
// number alone is not sufficient to attach an answer from another book.
func (s *Server) linkSolution(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DocumentID, Quote string
		Page              int
		Answer            *int
	}
	if !s.input(w, r, &in) {
		return
	}
	a, e := s.Platform.Asset(r.Context(), uid(r), r.PathValue("id"))
	if e != nil {
		s.fail(w, e)
		return
	}
	actor, e := s.Store.UserByID(r.Context(), uid(r))
	if e != nil {
		s.fail(w, e)
		return
	}
	if a.IsPublic && actor.Role != "admin" {
		s.fail(w, domain.ErrForbidden)
		return
	}
	if a.Kind != "question" || in.Page < 1 || len([]rune(in.Quote)) < 8 || len([]rune(in.Quote)) > 2000 {
		s.fail(w, &domain.RequestError{Status: 400, Message: "请选择题目并填写页码及8至2000字解析原文"})
		return
	}
	d, e := s.Store.Document(r.Context(), uid(r), in.DocumentID)
	if e != nil {
		s.fail(w, e)
		return
	}
	kb, e := s.Store.KnowledgeBase(r.Context(), uid(r), d.KnowledgeBaseID)
	if e != nil {
		s.fail(w, e)
		return
	}
	if a.IsPublic && !kb.IsPublic {
		s.fail(w, domain.ErrForbidden)
		return
	}
	parsed, e := s.Platform.DocumentStructure(r.Context(), uid(r), d.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	if parsed.State == "staging" {
		s.fail(w, &domain.RequestError{Status: 409, Message: "解析版本尚未启用，请等待处理完成"})
		return
	}
	quote := strings.TrimSpace(in.Quote)
	found := false
	for _, p := range parsed.Pages {
		if p.Number == in.Page && service.CanUseQuestionPage(p) && strings.Contains(p.Text, quote) {
			found = true
		}
	}
	if !found {
		s.fail(w, &domain.RequestError{Status: 400, Message: "解析原文未匹配或页面有解析警告，请核对文件、页码与逐字原文"})
		return
	}
	if in.Answer != nil {
		if *in.Answer < 0 || *in.Answer >= len(a.Payload.Options) {
			s.fail(w, &domain.RequestError{Status: 400, Message: "答案超出选项范围"})
			return
		}
		a.Payload.Answer = *in.Answer
		a.Payload.AnswerOrigin = "source"
	}
	a.Payload.Explanation = quote
	a.Payload.Evidence = []domain.Citation{{DocumentID: d.ID, DocumentTitle: d.OriginalFilename, RevisionID: d.ActiveRevisionID, Page: in.Page, Excerpt: quote, Location: fmt.Sprintf("第%d页", in.Page), LocationQuality: "page"}}
	a.Payload.ReviewStatus = "pending"
	a.Status = "unavailable"
	saved, e := s.Platform.SaveAsset(r.Context(), uid(r), a)
	s.reply(w, saved, e)
}
