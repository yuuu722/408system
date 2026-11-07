package repository

import (
	"context"
	"encoding/json"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"strings"
)

type QuestionDedupeReport struct {
	ReadyRecords, UniqueGroups, DuplicateRecords, Changed, Quarantined int
	Conflicts                                                          []map[string]any
	ManualGroups, PointLabelGroups                                     int
}

// Records and source references are retained. Only the learning family metadata
// is unified; conflicting auto-published source grades are quarantined, never
// picked by majority vote or overwritten with a guessed answer.
func (p *Postgres) DeduplicatePublicQuestions(ctx context.Context, uid, kid string) (QuestionDedupeReport, error) {
	report := QuestionDedupeReport{Conflicts: []map[string]any{}}
	user, e := p.UserByID(ctx, uid)
	if e != nil {
		return report, e
	}
	if user.Role != "admin" {
		return report, domain.ErrForbidden
	}
	kb, e := p.KnowledgeBase(ctx, uid, kid)
	if e != nil {
		return report, e
	}
	if !kb.IsPublic {
		return report, domain.ErrForbidden
	}
	tx, e := p.DB.Begin(ctx)
	if e != nil {
		return report, e
	}
	defer tx.Rollback(ctx)
	rows, e := tx.Query(ctx, `SELECT a.id,a.kind,COALESCE(a.point_id::text,''),a.title,a.status,a.revision,a.payload,a.is_public FROM learning_assets a JOIN documents d ON d.id=a.document_id WHERE d.knowledge_base_id=$1 AND a.is_public=true AND a.kind='question' AND a.status='ready' ORDER BY a.id FOR UPDATE OF a`, kid)
	if e != nil {
		return report, e
	}
	groups := map[string][]domain.Asset{}
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			rows.Close()
			return report, err
		}
		report.ReadyRecords++
		key := domain.QuestionContentFamily(a)
		if key == "" {
			key = "unmerged:" + a.ID
		}
		groups[key] = append(groups[key], a)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return report, e
	}
	for key, group := range groups {
		conflict, manual, pointConflict := false, false, false
		for _, a := range group {
			manual = manual || a.Payload.DifficultyOrigin == "user" || a.Payload.QuestionOrigin != "extracted"
			if a.Payload.Answer != group[0].Payload.Answer || a.Payload.AnswerOrigin != "source" {
				conflict = true
			}
			pointConflict = pointConflict || a.PointID != group[0].PointID
		}
		if manual {
			report.ManualGroups++
			report.UniqueGroups += len(group)
			continue
		}
		if pointConflict && !conflict {
			report.PointLabelGroups++
			report.UniqueGroups += len(group)
			continue
		}
		if !strings.HasPrefix(key, domain.PublicQuestionFamilyPrefix) {
			report.UniqueGroups += len(group)
			continue
		}
		if conflict {
			ids := []string{}
			for _, a := range group {
				ids = append(ids, a.ID)
				a.Payload.FamilyID = "conflict:" + a.ID
				a.Payload.ReviewStatus = "source_conflict"
				a.Payload.Issue = "同题来源的答案或知识点冲突，暂停自动判分，保留全部来源"
				raw, _ := json.Marshal(a.Payload)
				if _, e = tx.Exec(ctx, `UPDATE learning_assets SET payload=$2,status='unavailable',revision=revision+1,updated_at=now() WHERE id=$1`, a.ID, raw); e != nil {
					return report, e
				}
				report.Quarantined++
			}
			report.Conflicts = append(report.Conflicts, map[string]any{"AssetIDs": ids, "Reason": "答案或知识点不一致，未合并未猜选"})
			continue
		}
		report.UniqueGroups++
		report.DuplicateRecords += len(group) - 1
		for _, a := range group {
			legacy := a.Payload.FamilyID
			if legacy == "" {
				legacy = a.ID
			}
			if legacy != key && !strings.HasPrefix(legacy, domain.PublicQuestionFamilyPrefix) {
				if _, e = tx.Exec(ctx, `INSERT INTO question_family_aliases(legacy_family_id,canonical_family_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, legacy, key); e != nil {
					return report, e
				}
			}
			if a.Payload.FamilyID == key {
				continue
			}
			if _, e = tx.Exec(ctx, `UPDATE learning_assets SET payload=jsonb_set(payload,'{FamilyID}',to_jsonb($2::text)),revision=revision+1,updated_at=now() WHERE id=$1`, a.ID, key); e != nil {
				return report, e
			}
			report.Changed++
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return report, e
	}
	return report, nil
}
