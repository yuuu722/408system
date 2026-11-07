package httpapi

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/repository"
	"github.com/local/exam408-assistant/backend/internal/service"
	"net/http"
	"strings"
)

// Resolve differently named answer files only by exact complete problem content.
func (s *Server) resolveQuestionAnswers(w http.ResponseWriter, r *http.Request) {
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
	parsed, e := s.Platform.DocumentStructure(r.Context(), uid(r), d.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	rows, e := s.Platform.DocumentQuestionAssets(r.Context(), uid(r), d.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	need := []domain.Asset{}
	for _, a := range rows {
		if (a.Payload.Issue == "" || strings.HasPrefix(a.Payload.Issue, "未匹配知识点")) && a.Payload.ExtractionKey != "" && !domain.KeepSourceAnswer(a, -1) {
			need = append(need, a)
		}
	}
	if len(need) == 0 {
		s.reply(w, map[string]any{"Resolved": 0}, nil)
		return
	}
	docs, e := s.Store.ListDocuments(r.Context(), uid(r), d.KnowledgeBaseID)
	if e != nil {
		s.fail(w, e)
		return
	}
	type match struct {
		source     domain.Document
		parsedHash string
		proposal   service.AnswerFileProposal
	}
	matches := map[string][]match{}
	for _, ad := range docs {
		if ad.ID == d.ID || ad.Status != "ready" {
			continue
		}
		name := ad.OriginalFilename
		if ad.ResourceType != "solution" && !strings.Contains(name, "答案") && !strings.Contains(name, "解答") && !strings.Contains(name, "解析") && !strings.Contains(name, "详解") && !service.MaybeCompanionStudyBook(d.OriginalFilename, parsed.Pages, name) {
			continue
		}
		ap, err := s.Platform.DocumentStructure(r.Context(), uid(r), ad.ID)
		if err != nil {
			continue
		}
		entries := service.ParseDocumentAnswers(ad.OriginalFilename, ap.Pages)
		proposals := service.MatchQuestionContent(need, entries)
		if service.SameQuestionBook(d.OriginalFilename, ad.OriginalFilename) || service.CompanionStudyBooks(d.OriginalFilename, parsed.Pages, ad.OriginalFilename, ap.Pages) {
			numbered := service.MatchAnswerFile(need, entries, "", "")
			existing := map[string]bool{}
			for _, p := range proposals {
				existing[p.Asset.ID] = true
			}
			for _, p := range numbered {
				if !existing[p.Asset.ID] {
					proposals = append(proposals, p)
				}
			}
		}
		if len(proposals) == 0 {
			continue
		}
		parsedHash := repository.ParsedFingerprint(ap)
		for _, pair := range proposals {
			if domain.KeepSourceAnswer(pair.Asset, pair.Answer.Answer) {
				continue
			}
			matches[pair.Asset.ID] = append(matches[pair.Asset.ID], match{ad, parsedHash, pair})
		}
	}
	points, e := s.Store.ListKnowledgePoints(r.Context())
	if e != nil {
		s.fail(w, e)
		return
	}
	groups := map[string][]domain.Asset{}
	proofs := map[string]match{}
	ambiguous := 0
	for _, choices := range matches {
		m := choices[0]
		conflict := false
		for _, other := range choices[1:] {
			if !service.EquivalentFileAnswers(m.proposal.Answer, other.proposal.Answer) {
				conflict = true
				break
			}
		}
		if conflict {
			ambiguous++
			continue
		}
		updated := repository.LinkSourceAnswer(m.proposal.Asset, m.source, m.proposal.Answer.Page, m.proposal.Answer.Answer, service.AnswerFileText(m.proposal.Answer))
		if updated.PointID == "" {
			updated.PointID = service.MatchQuestionPoint(updated.Payload.Stem+"\n"+updated.Payload.Explanation, points)
			if updated.PointID != "" && strings.HasPrefix(updated.Payload.Issue, "未匹配知识点") {
				updated.Payload.Issue = ""
			}
		}
		updated.Payload.Evidence[0].Excerpt = m.proposal.Answer.Quote
		for _, extra := range m.proposal.Answer.Extra {
			updated.Payload.Evidence = append(updated.Payload.Evidence, domain.Citation{DocumentID: m.source.ID, DocumentTitle: m.source.OriginalFilename, RevisionID: m.source.ActiveRevisionID, Page: extra.Page, Excerpt: extra.Quote, LocationQuality: "page"})
		}
		groups[m.source.ID] = append(groups[m.source.ID], updated)
		proofs[m.source.ID] = m
	}
	count := 0
	for id, updates := range groups {
		m := proofs[id]
		applied, err := s.Platform.ApplyAnswerFile(r.Context(), uid(r), d, m.source, updates, repository.ParsedFingerprint(parsed), m.parsedHash)
		if err != nil {
			s.reply(w, map[string]any{"Resolved": count, "Ambiguous": ambiguous}, err)
			return
		}
		count += applied
	}
	s.reply(w, map[string]any{"Resolved": count, "Ambiguous": ambiguous}, nil)
}
