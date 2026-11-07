package domain

import (
	"strconv"
	"strings"
)

// Only public, ready canonical families are collapsed. Private member records
// and administrative source records remain independent and accessible as before.
func CollapsePublicQuestionVariants(assets []Asset) []Asset {
	out := []Asset{}
	index := map[string]int{}
	for _, a := range assets {
		key := a.Payload.FamilyID
		if !a.IsPublic || a.Kind != "question" || a.Status != "ready" || !strings.HasPrefix(key, PublicQuestionFamilyPrefix) {
			out = append(out, a)
			continue
		}
		// Recheck content and grade at read time; stale family metadata cannot hide a divergent edit.
		if QuestionContentFamily(a) != key {
			out = append(out, a)
			continue
		}
		key += ":" + a.PointID + ":" + strconv.Itoa(a.Payload.Answer) + ":" + a.Payload.AnswerOrigin
		if i, ok := index[key]; ok {
			out[i].Payload.VariantSources = append(out[i].Payload.VariantSources, Citation{DocumentID: a.Payload.DocumentID, DocumentTitle: a.Title, Page: a.Payload.Page, RevisionID: a.Payload.RevisionID, LocationQuality: "page"})
		} else {
			index[key] = len(out)
			out = append(out, a)
		}
	}
	return out
}
