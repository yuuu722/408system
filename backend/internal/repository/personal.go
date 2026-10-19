package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/local/exam408-assistant/backend/internal/domain"
)

// Replaces this attempt's contribution, preserving attempt time when an older report is edited.
func syncAssessmentEvents(ctx context.Context, tx pgx.Tx, uid, aid string, result domain.AssessmentResult) error {
	if _, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,409))`, uid); e != nil {
		return e
	}
	if _, e := tx.Exec(ctx, `DELETE FROM exam408_learning_events WHERE user_id=$1 AND metadata->>'assessment_id'=$2 AND event_type IN ('quiz_correct','quiz_wrong')`, uid, aid); e != nil {
		return e
	}
	for _, item := range result.Items {
		snapshot, _ := json.Marshal(item)
		if _, e := tx.Exec(ctx, `INSERT INTO assessment_result_changes(user_id,assessment_id,question_id,occurred_at,snapshot) SELECT $1,$2,$3,a.submitted_at,$4 FROM assessments a WHERE a.id=$2 AND NOT EXISTS(SELECT 1 FROM (SELECT snapshot FROM assessment_result_changes WHERE assessment_id=$2 AND question_id=$3 ORDER BY sequence DESC LIMIT 1) old WHERE old.snapshot=$4::jsonb)`, uid, aid, item.ID, snapshot); e != nil {
			return e
		}
		if !item.Counted {
			continue
		}
		event := "quiz_wrong"
		if item.Correct {
			event = "quiz_correct"
		}
		family := item.FamilyID
		if family == "" {
			family = item.ID
		}
		meta, _ := json.Marshal(map[string]any{"assessment_id": aid, "question_id": item.ID, "family_id": family, "selected": item.Selected, "answer_origin": item.AnswerOrigin, "confirmation": item.Confirmation})
		if _, e := tx.Exec(ctx, `INSERT INTO exam408_learning_events(user_id,subject_id,knowledge_point_id,event_type,metadata,created_at) SELECT $1,k.subject_id,k.id,$3,$4,a.submitted_at FROM exam408_knowledge_points k,assessments a WHERE k.id=$2 AND a.id=$5`, uid, item.PointID, event, meta, aid); e != nil {
			return e
		}
	}
	// Clear stale projections only for points touched by this attempt, including a withdrawn confirmation.
	for _, item := range result.Items {
		if _, e := tx.Exec(ctx, `DELETE FROM exam408_user_mastery WHERE user_id=$1 AND knowledge_point_id=$2`, uid, item.PointID); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `WITH latest AS (SELECT DISTINCT ON(COALESCE(NULLIF(metadata->>'family_id',''),metadata->>'question_id')) event_type,created_at FROM exam408_learning_events WHERE user_id=$1 AND knowledge_point_id=$2 AND event_type IN ('quiz_correct','quiz_wrong') AND metadata ? 'question_id' ORDER BY COALESCE(NULLIF(metadata->>'family_id',''),metadata->>'question_id'),created_at DESC,metadata->>'assessment_id' DESC,metadata->>'question_id' DESC),score AS (SELECT avg((event_type='quiz_correct')::int)::real value,count(*) n,max(created_at) last_at FROM latest) INSERT INTO exam408_user_mastery(user_id,knowledge_point_id,mastery_score,weakness_score,confidence,interaction_count,last_interaction_at) SELECT $1,$2,value,1-value,LEAST(1,n/10.0),n,last_at FROM score WHERE n>0`, uid, item.PointID)
		if e != nil {
			return e
		}
	}
	return nil
}
func recount(r *domain.AssessmentResult) {
	r.Correct = 0
	r.Counted = 0
	for _, v := range r.Items {
		if v.Counted {
			r.Counted++
			if v.Correct {
				r.Correct++
			}
		}
	}
}

func (p *Postgres) Assessment(ctx context.Context, uid, aid string) (domain.Assessment, error) {
	return scanAssessment(p.DB.QueryRow(ctx, `SELECT id,kind,questions,result,created_at FROM assessments WHERE id=$1 AND user_id=$2`, aid, uid))
}

func (p *Postgres) ConfirmAnswer(ctx context.Context, uid, aid, qid, decision string) (domain.AssessmentResult, error) {
	return p.updateResult(ctx, uid, aid, qid, func(item *domain.QuestionResult) error {
		if decision != "correct" && decision != "wrong" && decision != "uncertain" {
			return fmt.Errorf("选择正确、错误或不确定（Invalid decision）")
		}
		item.Confirmation = decision
		item.Counted = decision != "uncertain"
		item.Correct = decision == "correct"
		return nil
	})
}
func (p *Postgres) SaveAIAnswer(ctx context.Context, uid, aid, qid string, answer int, explanation string) (domain.AssessmentResult, error) {
	return p.updateResult(ctx, uid, aid, qid, func(item *domain.QuestionResult) error {
		item.Answer = answer
		item.Explanation = explanation
		item.AnswerOrigin = "ai"
		if !item.Counted {
			item.Correct = answer >= 0 && answer == item.Selected
		}
		return nil
	})
}
func (p *Postgres) updateResult(ctx context.Context, uid, aid, qid string, change func(*domain.QuestionResult) error) (domain.AssessmentResult, error) {
	var empty domain.AssessmentResult
	tx, e := p.DB.Begin(ctx)
	if e != nil {
		return empty, e
	}
	defer tx.Rollback(ctx)
	a, e := scanAssessment(tx.QueryRow(ctx, `SELECT id,kind,questions,result,created_at FROM assessments WHERE id=$1 AND user_id=$2 FOR UPDATE`, aid, uid))
	if e != nil {
		return empty, e
	}
	if a.Result == nil {
		return empty, fmt.Errorf("请先提交作答（Incomplete assessment）")
	}
	found := false
	for i := range a.Result.Items {
		v := &a.Result.Items[i]
		if v.ID == qid {
			if v.AnswerOrigin == "source" || v.AnswerOrigin == "" {
				return empty, fmt.Errorf("资料答案题目不接受AI判分确认（Invalid confirmation）")
			}
			if e = change(v); e != nil {
				return empty, e
			}
			found = true
			break
		}
	}
	if !found {
		return empty, ErrNotFound
	}
	recount(a.Result)
	raw, _ := json.Marshal(a.Result)
	if _, e = tx.Exec(ctx, `UPDATE assessments SET result=$2 WHERE id=$1`, aid, raw); e != nil {
		return empty, e
	}
	if e = syncAssessmentEvents(ctx, tx, uid, aid, *a.Result); e != nil {
		return empty, e
	}
	return *a.Result, tx.Commit(ctx)
}

// Re-evaluate legacy resources on startup, without enabling malformed old drafts.
func (p *Postgres) RefreshAssetAvailability(ctx context.Context) error {
	rows, e := p.DB.Query(ctx, `SELECT `+assetCols+` FROM learning_assets`)
	if e != nil {
		return e
	}
	var assets []domain.Asset
	for rows.Next() {
		a, e := scanAsset(rows)
		if e != nil {
			rows.Close()
			return e
		}
		assets = append(assets, a)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, a := range assets {
		domain.PrepareAsset(&a)
		raw, _ := json.Marshal(a.Payload)
		if _, e = p.DB.Exec(ctx, `UPDATE learning_assets SET status=$2,payload=$3 WHERE id=$1 AND revision=$4`, a.ID, a.Status, raw, a.Revision); e != nil {
			return e
		}
	}
	return nil
}
