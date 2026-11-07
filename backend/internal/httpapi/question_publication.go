package httpapi

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/repository"
	"github.com/local/exam408-assistant/backend/internal/service"
	"net/http"
	"reflect"
	"strings"
)

// No client-supplied answers or approval flags: publish only complete choices
// reconstructed from the current stored question parse and source answer parse.
func (s *Server) publishVerifiedQuestions(w http.ResponseWriter, r *http.Request) {
	d, e := s.Store.Document(r.Context(), uid(r), r.PathValue("id"))
	if e != nil {
		s.fail(w, e)
		return
	}
	kb, e := s.Platform.KnowledgeBase(r.Context(), uid(r), d.KnowledgeBaseID)
	if e != nil {
		s.fail(w, e)
		return
	}
	if !kb.IsPublic || d.Status != "ready" {
		s.fail(w, domain.ErrForbidden)
		return
	}
	parsed, e := s.Platform.QuestionStructure(r.Context(), uid(r), d.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	points, e := s.Store.ListKnowledgePoints(r.Context())
	if e != nil {
		s.fail(w, e)
		return
	}
	rows, e := s.Platform.DocumentQuestionAssets(r.Context(), uid(r), d.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	fresh := map[string]domain.Asset{}
	for _, a := range service.ExtractQuestions(d, parsed.Pages, points) {
		fresh[a.Payload.ExtractionKey] = a
	}
	pointIDs := map[string]bool{}
	for _, p := range points {
		pointIDs[p.ID] = true
	}
	sources := map[string]repository.QuestionSourceSnapshot{d.ID: {Document: d, Parsed: parsed}}
	answerMatches := map[string]map[string]service.FileAnswer{}
	updates := []domain.Asset{}
	skipped := []map[string]string{}
candidateLoop:
	for _, a := range rows {
		if a.Payload.ReviewStatus == "source_conflict" {
			skipped = append(skipped, map[string]string{"AssetID": a.ID, "Reason": "同题来源冲突，保留待修复，不自动重发"})
			continue
		}
		if a.Payload.ReviewStatus == "published" {
			continue
		}
		reject := func(reason string) { skipped = append(skipped, map[string]string{"AssetID": a.ID, "Reason": reason}) }
		for _, old := range rows {
			if old.Payload.ReviewStatus == "published" && service.SameQuestionSourceAnchor(old, a) {
				reject("该原文题已发布，保留现有题目，避免重排后重复发布")
				continue candidateLoop
			}
		}
		f, ok := fresh[a.Payload.ExtractionKey]
		if !ok || a.Payload.ExtractionKey == "" || a.Payload.Stem != f.Payload.Stem || !reflect.DeepEqual(a.Payload.Options, f.Payload.Options) {
			reject("当前原文不能重建该题，保留为未修复记录")
			continue
		}
		if f.Payload.Issue != "" && !strings.HasPrefix(f.Payload.Issue, "未匹配知识点") {
			reject(f.Payload.Issue)
			continue
		}
		if a.Payload.DifficultyOrigin == "user" {
			reject("保留人工编辑记录")
			continue
		}
		if a.PointID == "" {
			a.PointID = f.PointID
		}
		if !pointIDs[a.PointID] {
			reject("未可靠关联知识点")
			continue
		}
		if a.Payload.AnswerOrigin != "source" || a.Payload.Answer < 0 || a.Payload.Answer >= len(a.Payload.Options) || !domain.HasQuestionExplanation(a.Payload.Explanation) {
			reject("缺少资料答案或实质解析")
			continue
		}
		if f.Payload.AnswerOrigin == "source" && f.Payload.Answer != a.Payload.Answer {
			reject("与当前题目原文答案冲突")
			continue
		}
		verified := f.Payload.AnswerOrigin == "source" && f.Payload.Answer == a.Payload.Answer && f.Payload.Explanation == a.Payload.Explanation
		if verified {
			a.Payload.Evidence = f.Payload.Evidence
			a.Payload.Source = f.Payload.Source
		}
		if !verified && len(a.Payload.Evidence) > 0 {
			sourceID := a.Payload.Evidence[0].DocumentID
			if sourceID != d.ID {
				if _, loaded := answerMatches[sourceID]; !loaded {
					answerMatches[sourceID] = map[string]service.FileAnswer{}
					ad, err := s.Store.Document(r.Context(), uid(r), sourceID)
					if err == nil && ad.KnowledgeBaseID == d.KnowledgeBaseID && ad.Status == "ready" {
						ap, err := s.Platform.QuestionStructure(r.Context(), uid(r), ad.ID)
						if err == nil {
							sources[sourceID] = repository.QuestionSourceSnapshot{Document: ad, Parsed: ap}
							entries := service.ParseDocumentAnswers(ad.OriginalFilename, ap.Pages)
							if service.SameQuestionBook(d.OriginalFilename, ad.OriginalFilename) || service.CompanionStudyBooks(d.OriginalFilename, parsed.Pages, ad.OriginalFilename, ap.Pages) {
								for _, pair := range service.MatchAnswerFile(rows, entries, "", "") {
									answerMatches[sourceID][pair.Asset.ID] = pair.Answer
								}
							}
							for _, pair := range service.MatchQuestionContent(rows, entries) {
								answerMatches[sourceID][pair.Asset.ID] = pair.Answer
							}
						}
					}
				}
				if answer, found := answerMatches[sourceID][a.ID]; found && answer.Answer == a.Payload.Answer && service.AnswerFileText(answer) == a.Payload.Explanation {
					snapshot := sources[sourceID]
					verified = true
					expected := []service.AnswerPassage{{Page: answer.Page, Quote: answer.Quote}}
					expected = append(expected, answer.Extra...)
					if len(expected) != len(a.Payload.Evidence) {
						verified = false
					}
					for i, ref := range a.Payload.Evidence {
						if i >= len(expected) || ref.DocumentID != sourceID || ref.RevisionID != snapshot.Document.ActiveRevisionID || ref.Page != expected[i].Page || ref.Excerpt != expected[i].Quote {
							verified = false
						}
					}
				}
			}
		}
		if !verified {
			reject("答案解析与当前原文不能唯一匹配，不自动发布")
			continue
		}
		a.Payload.Issue = ""
		a.Payload.ReviewStatus = "published"
		domain.PrepareAsset(&a)
		if a.Status != "ready" {
			reject(a.Payload.Issue)
			continue
		}
		updates = append(updates, a)
		if len(updates) >= 200 {
			break
		}
	}
	if len(updates) == 0 {
		s.reply(w, map[string]any{"Published": 0, "Skipped": skipped}, nil)
		return
	}
	count, e := s.Platform.PublishVerifiedQuestions(r.Context(), uid(r), d, updates, sources)
	s.reply(w, map[string]any{"Published": count, "Skipped": skipped}, e)
}
