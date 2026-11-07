package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/repository"
	"github.com/local/exam408-assistant/backend/internal/service"
	"net/http"
	"sort"
)

func (s *Server) answerFile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AnswerDocumentID, QuestionSection, AnswerSection, Fingerprint string
		Apply, OnlyIncomplete                                         bool
	}
	if !s.input(w, r, &in) {
		return
	}
	q, e := s.Store.Document(r.Context(), uid(r), r.PathValue("id"))
	if e != nil {
		s.fail(w, e)
		return
	}
	if e = s.Platform.CanMaintainDocument(r.Context(), uid(r), q.ID); e != nil {
		s.fail(w, e)
		return
	}
	a, e := s.Store.Document(r.Context(), uid(r), in.AnswerDocumentID)
	if e != nil {
		s.fail(w, e)
		return
	}
	if q.ID == a.ID || q.KnowledgeBaseID != a.KnowledgeBaseID {
		s.fail(w, &domain.RequestError{Status: 400, Message: "请选择同一知识库中的两个不同文件；请核对书名、版本与试卷范围"})
		return
	}
	if q.Status != "ready" || a.Status != "ready" {
		s.fail(w, &domain.RequestError{Status: 409, Message: "请等待两份资料处理就绪后配对"})
		return
	}
	qp, e := s.Platform.DocumentStructure(r.Context(), uid(r), q.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	ap, e := s.Platform.DocumentStructure(r.Context(), uid(r), a.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	assets, e := s.Platform.DocumentQuestionAssets(r.Context(), uid(r), q.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	points, e := s.Store.ListKnowledgePoints(r.Context())
	if e != nil {
		s.fail(w, e)
		return
	}
	// Reuse stored parse to recover chapter/number metadata for older imports.
	extracted := service.ExtractQuestions(q, qp.Pages, points)
	meta := map[string]domain.Asset{}
	for _, x := range extracted {
		meta[x.Payload.ExtractionKey] = x
	}
	questions := []domain.Asset{}
	for _, x := range assets {
		if x.Payload.DocumentID == q.ID && x.Kind == "question" {
			if m, ok := meta[x.Payload.ExtractionKey]; ok {
				x.Payload.QuestionNumber = m.Payload.QuestionNumber
				x.Payload.SectionKey = m.Payload.SectionKey
			}
			questions = append(questions, x)
		}
	}
	entries := service.ParseAnswerFile(ap.Pages)
	proposals := service.MatchAnswerFile(questions, entries, in.QuestionSection, in.AnswerSection)
	qsections, asections := []string{}, []string{}
	qs, as := map[string]bool{}, map[string]bool{}
	for _, x := range questions {
		if x.Payload.SectionKey != "" && !qs[x.Payload.SectionKey] {
			qsections = append(qsections, x.Payload.SectionKey)
			qs[x.Payload.SectionKey] = true
		}
	}
	for _, x := range entries {
		if x.Section != "" && !as[x.Section] {
			asections = append(asections, x.Section)
			as[x.Section] = true
		}
	}
	sort.Slice(questions, func(i, j int) bool { return questions[i].ID < questions[j].ID })
	sort.Strings(qsections)
	sort.Strings(asections)
	snapshot, _ := json.Marshal([]any{q.ActiveRevisionID, a.ActiveRevisionID, qp, ap, questions, in.QuestionSection, in.AnswerSection, in.OnlyIncomplete})
	hash := sha256.Sum256(snapshot)
	fingerprint := hex.EncodeToString(hash[:])
	updates := []domain.Asset{}
	preview := []map[string]any{}
	skipped := []map[string]any{}
	for _, p := range proposals {
		if in.OnlyIncomplete && domain.KeepSourceAnswer(p.Asset, p.Answer.Answer) {
			reason := "已有资料答案和解析，保留原记录"
			if p.Answer.Answer >= 0 && p.Answer.Answer != p.Asset.Payload.Answer {
				reason = "与已有可靠答案冲突，请人工核对"
			}
			skipped = append(skipped, map[string]any{"AssetID": p.Asset.ID, "Reason": reason})
			continue
		}
		if len(updates) >= 200 {
			break
		}
		if p.Asset.Payload.Explanation == service.AnswerFileText(p.Answer) && len(p.Asset.Payload.Evidence) > 0 && p.Asset.Payload.Evidence[0].DocumentID == a.ID && p.Asset.Payload.Evidence[0].RevisionID == a.ActiveRevisionID && (p.Answer.Answer < 0 || p.Asset.Payload.Answer == p.Answer.Answer) {
			continue
		}
		if p.Answer.Answer >= len(p.Asset.Payload.Options) {
			continue
		}
		updated := repository.LinkSourceAnswer(p.Asset, a, p.Answer.Page, p.Answer.Answer, service.AnswerFileText(p.Answer))
		updated.Payload.Evidence[0].Excerpt = p.Answer.Quote
		pages := []int{p.Answer.Page}
		for _, part := range p.Answer.Extra {
			pages = append(pages, part.Page)
			updated.Payload.Evidence = append(updated.Payload.Evidence, domain.Citation{DocumentID: a.ID, DocumentTitle: a.OriginalFilename, RevisionID: a.ActiveRevisionID, Page: part.Page, Location: fmt.Sprintf("第%d页", part.Page), Excerpt: part.Quote, LocationQuality: "page"})
		}
		updates = append(updates, updated)
		preview = append(preview, map[string]any{"AssetID": p.Asset.ID, "Title": p.Asset.Title, "Number": p.Answer.Number, "Section": p.Asset.Payload.SectionKey, "Answer": p.Answer.Answer, "Page": p.Answer.Page, "Pages": pages, "Quote": service.AnswerFileText(p.Answer)})
	}
	applied := 0
	if in.Apply {
		if in.Fingerprint == "" || in.Fingerprint != fingerprint {
			s.fail(w, &domain.RequestError{Status: 409, Message: "预览已过期，请重新预览文件和章节配对"})
			return
		}
		applied, e = s.Platform.ApplyAnswerFile(r.Context(), uid(r), q, a, updates, repository.ParsedFingerprint(qp), repository.ParsedFingerprint(ap))
		if e != nil {
			s.fail(w, e)
			return
		}
	}
	s.reply(w, map[string]any{"Fingerprint": fingerprint, "QuestionSections": qsections, "AnswerSections": asections, "Matches": preview, "Skipped": skipped, "Applied": applied, "QuestionCount": len(questions), "AnswerCount": len(entries), "Notice": "仅配对章节与题号唯一的记录，原文存在不等于已核实属于该题；请核对文件、版本和预览后关联。已发布题目不覆盖，歧义项保留人工处理，关联后仍待审核；每批最多200题，已关联且内容未变的不重复更新。"}, nil)
}
