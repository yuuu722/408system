package repository

import (
	"context"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"sort"
	"strings"
)

// Reuse validated resources for every directory point, including parent nodes.
// Neither catalog text nor question options are invented as teaching evidence.
func (p *Postgres) PointStudy(ctx context.Context, uid, pointID string) (domain.PointStudy, error) {
	out := domain.PointStudy{Ancestors: []domain.KnowledgePoint{}, Children: []domain.KnowledgePoint{}, Knowledge: []domain.Citation{}, Exercises: []domain.Asset{}, Mistakes: []domain.PointMistake{}, Missing: []string{}}
	points, e := p.ListKnowledgePoints(ctx)
	if e != nil {
		return out, e
	}
	byID := map[string]domain.KnowledgePoint{}
	for _, v := range points {
		byID[v.ID] = v
		if v.ID == pointID {
			out.Point = v
		}
		if v.ParentID == pointID {
			out.Children = append(out.Children, v)
		}
	}
	if out.Point.ID == "" {
		return out, ErrNotFound
	}
	seen := map[string]bool{pointID: true}
	for id := out.Point.ParentID; id != "" && !seen[id]; id = byID[id].ParentID {
		seen[id] = true
		v, ok := byID[id]
		if !ok {
			break
		}
		out.Ancestors = append([]domain.KnowledgePoint{v}, out.Ancestors...)
	}
	scope := domain.PointScope(points, pointID)
	out.PointNames = map[string]string{}
	for id := range scope {
		out.PointNames[id] = byID[id].Name
	}
	out.Knowledge, e = p.unitKnowledge(ctx, uid, out.Point.Name, out.Point.Keywords...)
	if e != nil {
		return out, e
	}
	// Try a few direct child concepts when an overview heading has no literal paragraph.
	if len(out.Knowledge) == 0 {
		for _, child := range out.Children {
			refs, err := p.unitKnowledge(ctx, uid, child.Name, child.Keywords...)
			if err != nil {
				return out, err
			}
			out.Knowledge = append(out.Knowledge, refs...)
			if len(out.Knowledge) >= 2 {
				break
			}
		}
	}
	if len(out.Knowledge) > 0 {
		e = p.DB.QueryRow(ctx, `SELECT knowledge_base_id FROM documents WHERE id=$1`, out.Knowledge[0].DocumentID).Scan(&out.PreferredKnowledgeBaseID)
		if e != nil && nf(e) != ErrNotFound {
			return out, e
		}
	}
	bank, e := p.Assets(ctx, uid)
	if e != nil {
		return out, e
	}
	questions := []domain.Asset{}
	families := map[string]bool{}
	byAsset := map[string]domain.Asset{}
	visibleFamilies := map[string]bool{}
	for _, a := range bank {
		if a.Kind == "question" && a.Status == "ready" && scope[a.PointID] {
			byAsset[a.ID] = a
			visibleFamilies[domain.Family(a)] = true
		}
		if a.Kind != "question" || a.Status != "ready" || a.Payload.AnswerOrigin != "source" || !scope[a.PointID] || families[domain.Family(a)] {
			continue
		}
		families[domain.Family(a)] = true
		questions = append(questions, a)
		byAsset[a.ID] = a
		out.SubtreeQuestions++
		if a.PointID == pointID {
			out.DirectQuestions++
		}
	}
	history, submitted, e := p.unitHistory(ctx, uid)
	if e != nil {
		return out, e
	}
	sort.SliceStable(history, func(i, j int) bool {
		if submitted[history[i].ID].Equal(submitted[history[j].ID]) {
			return history[i].ID > history[j].ID
		}
		return submitted[history[i].ID].After(submitted[history[j].ID])
	})
	used := map[string]bool{}
	pending := map[string]bool{}
	latest := map[string]bool{}
	for _, a := range history {
		for _, q := range a.Questions {
			family := domain.Family(q)
			used[family] = true
			if a.Result == nil {
				pending[family] = true
				continue
			}
			for _, r := range a.Result.Items {
				if r.ID != q.ID || !r.Counted || latest[family] {
					continue
				}
				latest[family] = true
				if !r.Correct && scope[q.PointID] && len(out.Mistakes) < 5 {
					if _, ok := byAsset[q.ID]; ok || visibleFamilies[family] {
						out.Mistakes = append(out.Mistakes, domain.PointMistake{AssessmentID: a.ID, Question: q, Result: r, SubmittedAt: submitted[a.ID]})
					}
				}
			}
		}
	}
	// Prefer known exam provenance and a substantive source explanation; this is
	// a transparent resource heuristic, not expert certification of typicality.
	score := func(a domain.Asset) int {
		n := min(10, len([]rune(a.Payload.Explanation))/30)
		if strings.Contains(a.Payload.Source, "统考真题") || a.Payload.QuestionOrigin == "past_exam" {
			n += 20
		}
		if a.PointID == pointID {
			n += 10
		}
		return n
	}
	sort.SliceStable(questions, func(i, j int) bool {
		if score(questions[i]) != score(questions[j]) {
			return score(questions[i]) > score(questions[j])
		}
		return questions[i].ID < questions[j].ID
	})
	preferred, e := p.PreferredExampleID(ctx, uid, pointID)
	if e != nil {
		return out, e
	}
	for _, a := range questions {
		if a.ID == preferred {
			copy := a
			out.Example = &copy
			break
		}
	}
	for _, a := range questions {
		if out.Example != nil {
			break
		}
		if !pending[domain.Family(a)] && a.Payload.Explanation != "" {
			copy := a
			out.Example = &copy
			break
		}
	}
	examples, e := p.unitExampleFamilies(ctx, uid)
	if e != nil {
		return out, e
	}
	if out.Example != nil {
		family := domain.Family(*out.Example)
		examples[family] = true
		if _, e = p.DB.Exec(ctx, `INSERT INTO learning_unit_examples(user_id,family_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, uid, family); e != nil {
			return out, e
		}
	}
	for _, a := range questions {
		if !used[domain.Family(a)] && !examples[domain.Family(a)] && len(out.Exercises) < 3 {
			a.Payload.Answer = -1
			a.Payload.Explanation = ""
			a.Payload.Evidence = nil
			a.Payload.VariantSources = nil
			out.Exercises = append(out.Exercises, a)
		}
	}
	if len(out.Knowledge) == 0 {
		out.Missing = append(out.Missing, "尚未定位到可核对教材段落，不能把目录描述当成完整讲解。")
	}
	if out.Example == nil {
		if len(questions) > 0 {
			out.Missing = append(out.Missing, "可选例题正处于未提交测评中，为避免提前显示答案，请先完成已有测评。 ")
		} else {
			out.Missing = append(out.Missing, "暂无有可靠答案与解析的参考例题。")
		}
	}
	if len(out.Exercises) == 0 {
		out.Missing = append(out.Missing, "暂无未接触的独立练习题组，可先复查已有错题或补充资料。")
	}
	if out.Example != nil {
		out.Example.Payload.Answer = -1
		out.Example.Payload.Explanation = ""
	}
	return out, nil
}
