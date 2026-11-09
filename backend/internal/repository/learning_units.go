package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"regexp"
	"sort"
	"strings"
	"time"
)

var textbookNameAcronyms = regexp.MustCompile(`[A-Za-z][A-Za-z0-9-]+`)

func resourceKey(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:])
}

// Textbook-only, owner/public scoped references; these are excerpts, not generated explanations.
func (p *Postgres) unitKnowledge(ctx context.Context, uid, name string, extra ...string) ([]domain.Citation, error) {
	terms := []string{name}
	for _, v := range strings.FieldsFunc(name, func(r rune) bool { return r == '与' || r == '、' }) {
		if len([]rune(v)) >= 2 {
			terms = append(terms, v)
		}
	}
	primary := append([]string{}, terms...)
	focus := textbookNameAcronyms.FindAllString(name, -1)
	for _, v := range extra {
		v = strings.Join(strings.Fields(v), "")
		if len([]rune(v)) >= 2 && v != "基本概念" && v != "原理" && v != "基础" && len(terms) < 12 {
			terms = append(terms, v)
		}
	}
	rows, e := p.DB.Query(ctx, `SELECT c.document_id,d.original_filename,c.id,c.source_location,c.content,COALESCE(c.metadata->>'page','0'),COALESCE(c.metadata->>'revision_id','') FROM document_chunks c JOIN documents d ON d.id=c.document_id JOIN knowledge_bases kb ON kb.id=c.knowledge_base_id LEFT JOIN document_catalog dc ON dc.document_id=d.id WHERE (kb.is_public OR kb.user_id=$1) AND COALESCE(dc.resource_type,d.resource_type)='textbook' AND COALESCE(dc.purpose,'knowledge')<>'questions' AND (d.status='ready' OR d.active_revision_id IS NOT NULL) AND EXISTS(SELECT 1 FROM unnest($2::text[]) term WHERE strpos(lower(translate(c.content,' '||chr(9)||chr(10)||chr(13)||chr(160)||chr(12288),'')),lower(term))>0) AND (COALESCE(cardinality($4::text[]),0)=0 OR EXISTS(SELECT 1 FROM unnest($4::text[]) term WHERE strpos(lower(translate(c.content,' '||chr(9)||chr(10)||chr(13)||chr(160)||chr(12288),'')),lower(term))>0)) AND COALESCE(c.metadata->>'heading_path','') !~ '(习题|选择题|参考答案|答案解析)' ORDER BY (strpos(lower(translate(c.content,' '||chr(9)||chr(10)||chr(13)||chr(160)||chr(12288),'')),lower($3))>0) DESC,(SELECT count(*) FROM unnest($5::text[]) term WHERE strpos(lower(translate(c.content,' '||chr(9)||chr(10)||chr(13)||chr(160)||chr(12288),'')),lower(term))>0) DESC,(SELECT count(*) FROM unnest($2::text[]) term WHERE strpos(lower(translate(c.content,' '||chr(9)||chr(10)||chr(13)||chr(160)||chr(12288),'')),lower(term))>0) DESC,kb.is_public DESC,c.document_id,c.chunk_index LIMIT 2`, uid, terms, strings.Join(strings.Fields(name), ""), focus, primary)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.Citation{}
	for rows.Next() {
		var c domain.Citation
		var page string
		if e = rows.Scan(&c.DocumentID, &c.DocumentTitle, &c.ChunkID, &c.Location, &c.Excerpt, &page, &c.RevisionID); e != nil {
			return nil, e
		}
		fmt.Sscan(page, &c.Page)
		out = append(out, c)
	}
	return out, rows.Err()
}

// Internal history is not capped by the UI's latest-50 list. Older exposure
// still excludes a family from a new-question check.
func (p *Postgres) unitHistory(ctx context.Context, uid string) ([]domain.Assessment, map[string]time.Time, error) {
	rows, e := p.DB.Query(ctx, `SELECT id,kind,questions,result,created_at,submitted_at FROM assessments WHERE user_id=$1 ORDER BY created_at DESC`, uid)
	if e != nil {
		return nil, nil, e
	}
	defer rows.Close()
	out := []domain.Assessment{}
	submitted := map[string]time.Time{}
	for rows.Next() {
		var a domain.Assessment
		var questions, result []byte
		var at *time.Time
		if e = rows.Scan(&a.ID, &a.Kind, &questions, &result, &a.CreatedAt, &at); e != nil {
			return nil, nil, e
		}
		if e = json.Unmarshal(questions, &a.Questions); e != nil {
			return nil, nil, e
		}
		if len(result) > 0 {
			if e = json.Unmarshal(result, &a.Result); e != nil {
				return nil, nil, e
			}
		}
		if at != nil {
			submitted[a.ID] = *at
		}
		out = append(out, a)
	}
	if e = rows.Err(); e != nil {
		return nil, nil, e
	}
	rows.Close()
	aliasRows, e := p.DB.Query(ctx, `SELECT legacy_family_id,canonical_family_id FROM question_family_aliases`)
	if e != nil {
		return nil, nil, e
	}
	defer aliasRows.Close()
	aliases := map[string]string{}
	for aliasRows.Next() {
		var a, b string
		if e = aliasRows.Scan(&a, &b); e != nil {
			return nil, nil, e
		}
		aliases[a] = b
	}
	if e = aliasRows.Err(); e != nil {
		return nil, nil, e
	}
	for i := range out {
		for j := range out[i].Questions {
			q := &out[i].Questions[j]
			family := domain.Family(*q)
			for n := 0; n < 8 && aliases[family] != ""; n++ {
				family = aliases[family]
			}
			q.Payload.FamilyID = family
		}
	}
	return out, submitted, nil
}

func (p *Postgres) unitExampleFamilies(ctx context.Context, uid string) (map[string]bool, error) {
	rows, e := p.DB.Query(ctx, `SELECT COALESCE(a.canonical_family_id,x.family_id) FROM learning_unit_examples x LEFT JOIN question_family_aliases a ON a.legacy_family_id=x.family_id WHERE x.user_id=$1`, uid)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var f string
		if e = rows.Scan(&f); e != nil {
			return nil, e
		}
		out[f] = true
	}
	return out, rows.Err()
}

func (p *Postgres) LearningUnits(ctx context.Context, uid string) ([]domain.LearningUnit, error) {
	plan, e := p.DailyPlan(ctx, uid)
	if e != nil {
		return nil, e
	}
	out := []domain.LearningUnit{}
	if plan.ID == "" {
		return out, nil
	}
	tasks, e := p.Tasks(ctx, uid)
	if e != nil {
		return nil, e
	}
	assets, e := p.Assets(ctx, uid)
	if e != nil {
		return nil, e
	}
	points, e := p.ListKnowledgePoints(ctx)
	if e != nil {
		return nil, e
	}
	names := map[string]string{}
	for _, v := range points {
		names[v.ID] = v.Name
	}
	byID := map[string]domain.Asset{}
	for _, a := range assets {
		byID[a.ID] = a
	}
	indexes := map[string]int{}
	for _, t := range tasks {
		if t.PlanDate != plan.Date || t.PointID == "" {
			continue
		}
		i, ok := indexes[t.PointID]
		if !ok {
			i = len(out)
			indexes[t.PointID] = i
			out = append(out, domain.LearningUnit{PointID: t.PointID, Name: names[t.PointID], Goal: "梳理定义、适用条件和易错点，能说明解题依据并完成新题检查。", Reason: t.Reason, Date: plan.Date, PlanID: plan.ID, Practice: []domain.Asset{}, Materials: []domain.Asset{}})
		}
		out[i].ScheduledMinutes += t.EstimatedMinutes
		a, ok := byID[t.AssetID]
		if ok && a.Kind != "question" {
			out[i].Materials = append(out[i].Materials, a)
		}
		if ok && a.Kind == "question" && a.Payload.AnswerOrigin == "source" && a.Status == "ready" {
			out[i].Practice = append(out[i].Practice, a)
		}
	}
	sort.SliceStable(assets, func(i, j int) bool { return assets[i].ID < assets[j].ID })
	assessments, submitted, e := p.unitHistory(ctx, uid)
	if e != nil {
		return nil, e
	}
	pendingFamilies := map[string]bool{}
	for _, a := range assessments {
		if a.Result == nil {
			for _, q := range a.Questions {
				pendingFamilies[domain.Family(q)] = true
			}
		}
	}
	var assignedAt time.Time
	if e = p.DB.QueryRow(ctx, `SELECT created_at FROM daily_plans WHERE id=$1 AND user_id=$2`, plan.ID, uid).Scan(&assignedAt); e != nil {
		return nil, e
	}
	for i := range out {
		u := &out[i]
		families := map[string]bool{}
		for _, a := range u.Practice {
			families[domain.Family(a)] = true
		}
		for _, a := range assets {
			if a.PointID == u.PointID && a.Kind == "question" && a.Status == "ready" && a.Payload.AnswerOrigin == "source" && a.Payload.Explanation != "" && !families[domain.Family(a)] && !pendingFamilies[domain.Family(a)] {
				copy := a
				u.Example = &copy
				break
			}
		}
		if u.Example != nil {
			if _, e = p.DB.Exec(ctx, `INSERT INTO learning_unit_examples(user_id,family_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, uid, domain.Family(*u.Example)); e != nil {
				return nil, e
			}
		}
		u.Knowledge, e = p.unitKnowledge(ctx, uid, u.Name)
		if e != nil {
			return nil, e
		}
		knowledgeParts := []string{}
		for _, a := range u.Materials {
			knowledgeParts = append(knowledgeParts, a.ID, fmt.Sprint(a.Revision))
		}
		for _, c := range u.Knowledge {
			knowledgeParts = append(knowledgeParts, c.DocumentID, c.ChunkID, c.RevisionID, c.Excerpt)
		}
		exampleKey := ""
		if u.Example != nil {
			exampleKey = resourceKey(u.Example.ID, fmt.Sprint(u.Example.Revision))
		}
		u.Steps = []domain.LearningUnitStep{{Key: "knowledge", Title: "知识复习", Minutes: 8, Available: len(u.Knowledge) > 0 || len(u.Materials) > 0, ResourceKey: resourceKey(knowledgeParts...)}, {Key: "example", Title: "典型例题", Minutes: 7, Available: u.Example != nil, ResourceKey: exampleKey}, {Key: "practice", Title: "独立练习", Minutes: 5 * len(u.Practice), Available: len(u.Practice) > 0}, {Key: "check", Title: "新题复测与总结", Minutes: 5, Available: false}}
		u.EstimatedMinutes = max(20+5*len(u.Practice), u.ScheduledMinutes)
		for j := 0; j < 2; j++ {
			step := &u.Steps[j]
			if step.Available {
				if e = p.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM learning_unit_steps WHERE user_id=$1 AND point_id=$2 AND step=$3 AND resource_key=$4)`, uid, u.PointID, step.Key, step.ResourceKey).Scan(&step.Completed); e != nil {
					return nil, e
				}
			} else {
				step.Notice = "缺少可核对资料，暂不标记完成；可补充资料或联系管理员。"
			}
		}
		done := map[string]bool{}
		for _, a := range assessments {
			if a.Result == nil || submitted[a.ID].Before(assignedAt) {
				continue
			}
			for _, r := range a.Result.Items {
				if r.Counted && r.PointID == u.PointID {
					done[r.ID] = true
					for _, q := range a.Questions {
						if q.ID == r.ID {
							done[domain.Family(q)] = true
						}
					}
				}
			}
		}
		all := len(u.Practice) > 0
		for _, a := range u.Practice {
			all = all && (done[a.ID] || done[domain.Family(a)])
		}
		u.Steps[2].Completed = all
		if !u.Steps[2].Available {
			u.Steps[2].Notice = "缺少有可靠答案的独立练习，暂不标记完成。"
		}
		var checkID string
		e = p.DB.QueryRow(ctx, `SELECT a.id FROM learning_unit_checks l JOIN assessments a ON a.id=l.assessment_id WHERE l.plan_id=$1 AND l.point_id=$2 AND a.user_id=$3 ORDER BY a.created_at DESC LIMIT 1`, plan.ID, u.PointID, uid).Scan(&checkID)
		if e != nil && nf(e) != ErrNotFound {
			return nil, e
		}
		u.CheckAssessmentID = checkID
		if checkID != "" {
			u.Steps[3].Available = true
			for _, a := range assessments {
				if a.ID == checkID && a.Result != nil && a.Result.Counted > 0 {
					u.Steps[3].Completed = true
					if a.Result.Correct < a.Result.Counted {
						u.Steps[3].Notice = "复测已提交，仍有错误；返回知识与例题环节补学，不视为已掌握。"
					}
				}
			}
		}
		// Look for a source-graded, unexposed and unanswered family for the check.
		used, e := p.unitExampleFamilies(ctx, uid)
		if e != nil {
			return nil, e
		}
		for f := range families {
			used[f] = true
		}
		if u.Example != nil {
			used[domain.Family(*u.Example)] = true
		}
		for _, a := range assessments {
			for _, q := range a.Questions {
				used[domain.Family(q)] = true
			}
		}
		for _, a := range assets {
			if a.PointID == u.PointID && a.Kind == "question" && a.Status == "ready" && a.Payload.AnswerOrigin == "source" && !used[domain.Family(a)] {
				u.Steps[3].Available = true
				break
			}
		}
		if !u.Steps[3].Available {
			u.Steps[3].Notice = "暂无同知识点未接触的新题族；不重复例题冒充复测。"
		}
		for j := range u.Practice {
			u.Practice[j].Payload.Answer = -1
			u.Practice[j].Payload.Explanation = ""
		}
	}
	return out, nil
}

func (p *Postgres) MarkUnitStep(ctx context.Context, uid, point, step, key string, complete bool) error {
	if step != "knowledge" && step != "example" {
		return fmt.Errorf("练习和复测只能通过实际作答记录完成")
	}
	units, e := p.LearningUnits(ctx, uid)
	if e != nil {
		return e
	}
	for _, u := range units {
		if u.PointID == point {
			for _, s := range u.Steps {
				if s.Key == step && s.Available && s.ResourceKey == key {
					if complete {
						_, e = p.DB.Exec(ctx, `INSERT INTO learning_unit_steps(user_id,point_id,step,resource_key) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, uid, point, step, key)
					} else {
						_, e = p.DB.Exec(ctx, `DELETE FROM learning_unit_steps WHERE user_id=$1 AND point_id=$2 AND step=$3 AND resource_key=$4`, uid, point, step, key)
					}
					return e
				}
			}
		}
	}
	return ErrNotFound
}

func (p *Postgres) CreateUnitCheck(ctx context.Context, uid, point string) (domain.Assessment, error) {
	tx, e := p.DB.Begin(ctx)
	if e != nil {
		return domain.Assessment{}, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,409))`, uid); e != nil {
		return domain.Assessment{}, e
	}

	units, e := p.LearningUnits(ctx, uid)
	if e != nil {
		return domain.Assessment{}, e
	}
	var unit *domain.LearningUnit
	for i := range units {
		if units[i].PointID == point {
			unit = &units[i]
			break
		}
	}
	if unit == nil {
		return domain.Assessment{}, ErrNotFound
	}
	history, _, e := p.unitHistory(ctx, uid)
	if e != nil {
		return domain.Assessment{}, e
	}
	used, e := p.unitExampleFamilies(ctx, uid)
	if e != nil {
		return domain.Assessment{}, e
	}
	for _, a := range history {
		if a.ID == unit.CheckAssessmentID && a.Result == nil {
			return RedactAssessment(a), nil
		}
		for _, q := range a.Questions {
			used[domain.Family(q)] = true
		}
	}
	for _, a := range unit.Practice {
		used[domain.Family(a)] = true
	}
	if unit.Example != nil {
		used[domain.Family(*unit.Example)] = true
	}
	assets, e := p.Assets(ctx, uid)
	if e != nil {
		return domain.Assessment{}, e
	}
	for _, a := range assets {
		if a.PointID != point || a.Kind != "question" || a.Status != "ready" || a.Payload.AnswerOrigin != "source" || used[domain.Family(a)] {
			continue
		}
		raw, _ := json.Marshal([]domain.Asset{a})
		result := domain.Assessment{Kind: "retest", Questions: []domain.Asset{a}}
		e = tx.QueryRow(ctx, `INSERT INTO assessments(user_id,kind,questions) VALUES($1,'retest',$2) RETURNING id,created_at`, uid, raw).Scan(&result.ID, &result.CreatedAt)
		if e != nil {
			return result, e
		}
		_, e = tx.Exec(ctx, `INSERT INTO learning_unit_checks(plan_id,point_id,assessment_id) VALUES($1,$2,$3)`, unit.PlanID, point, result.ID)
		if e != nil {
			return result, e
		}
		if e = tx.Commit(ctx); e != nil {
			return result, e
		}
		return RedactAssessment(result), nil
	}
	return domain.Assessment{}, fmt.Errorf("暂无未接触的新题族可复测；请补充可靠题目")
}
