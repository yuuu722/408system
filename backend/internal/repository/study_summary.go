package repository

import (
	"context"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"time"
)

type StudyAttempt struct {
	ID          string
	Kind        string
	CreatedAt   time.Time
	SubmittedAt *time.Time
	Total       int
	Counted     int
	Correct     int
}
type StudySummary struct {
	Assessments       int
	Submitted         int
	Unsubmitted       int
	UncountedAttempts int
	ValidFamilies     int
	CorrectFamilies   int
	CoveredPoints     int
	TotalPoints       int
	Accuracy          *float64
	Subjects          []domain.RadarSubject
	Recent            []StudyAttempt
	Notice            string
}

// All counts are scoped to the authenticated member. Radar uses only the latest
// effective answer per question family; raw attempt totals are named separately.
func (p *Postgres) StudySummary(ctx context.Context, uid string) (StudySummary, error) {
	out := StudySummary{Recent: []StudyAttempt{}, Notice: "正确率按题族最近有效作答计算，不等于考试成绩预测。浏览、提问和自评不计入掌握度；少量题目仅供观察，未测科目保持待测。覆盖率仅针对当前已录入知识点目录。"}
	err := p.DB.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE result IS NOT NULL),count(*) FILTER(WHERE result IS NULL),COALESCE(sum((SELECT count(*) FROM jsonb_array_elements(COALESCE(a.result->'Items','[]'::jsonb)) item WHERE COALESCE((item->>'Counted')::boolean,false)=false)),0) FROM assessments a WHERE user_id=$1`, uid).Scan(&out.Assessments, &out.Submitted, &out.Unsubmitted, &out.UncountedAttempts)
	if err != nil {
		return out, err
	}
	out.Subjects, err = p.Radar(ctx, uid)
	if err != nil {
		return out, err
	}
	for _, s := range out.Subjects {
		out.ValidFamilies += s.Tested
		out.CorrectFamilies += s.Correct
		out.CoveredPoints += s.CoveredPoints
		out.TotalPoints += s.TotalPoints
	}
	if out.ValidFamilies > 0 {
		v := float64(out.CorrectFamilies) / float64(out.ValidFamilies)
		out.Accuracy = &v
	}
	rows, err := p.DB.Query(ctx, `SELECT id,kind,created_at,submitted_at,COALESCE((result->>'Total')::int,0),COALESCE((result->>'Counted')::int,0),COALESCE((result->>'Correct')::int,0) FROM assessments WHERE user_id=$1 ORDER BY created_at DESC,id DESC LIMIT 10`, uid)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var a StudyAttempt
		if err = rows.Scan(&a.ID, &a.Kind, &a.CreatedAt, &a.SubmittedAt, &a.Total, &a.Counted, &a.Correct); err != nil {
			return out, err
		}
		out.Recent = append(out.Recent, a)
	}
	return out, rows.Err()
}
