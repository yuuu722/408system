package repository

import (
	"context"
	"encoding/json"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"time"
)

func planDate() string {
	return time.Now().In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02")
}
func (p *Postgres) DailyPlan(ctx context.Context, uid string) (domain.DailyPlan, error) {
	out := domain.DailyPlan{Date: planDate(), Strategy: "408-rules-v1", BudgetMinutes: 60}
	profile, e := p.Profile(ctx, uid)
	if e != nil {
		return out, e
	}
	if profile != nil {
		out.BudgetMinutes = profile.DailyMinutes
	}
	e = p.DB.QueryRow(ctx, `SELECT id,budget_minutes,strategy FROM daily_plans WHERE user_id=$1 AND plan_date=$2`, uid, out.Date).Scan(&out.ID, &out.BudgetMinutes, &out.Strategy)
	if e != nil && nf(e) == ErrNotFound {
		return out, nil
	}
	if e != nil {
		return out, e
	}
	e = p.DB.QueryRow(ctx, `SELECT COALESCE(sum(estimated_minutes),0) FROM study_tasks WHERE plan_id=$1`, out.ID).Scan(&out.AllocatedMinutes)
	return out, e
}

func (p *Postgres) GenerateTasks(ctx context.Context, uid string) error {
	tx, e := p.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,409))`, uid); e != nil {
		return e
	}
	profile := domain.Profile{DailyMinutes: 60, Stage: "beginner", Preference: "mixed"}
	saved, e := p.Profile(ctx, uid)
	if e != nil {
		return e
	}
	if saved != nil {
		profile = *saved
	}
	var pid string
	var budget int
	e = tx.QueryRow(ctx, `INSERT INTO daily_plans(user_id,plan_date,budget_minutes) VALUES($1,$2,$3) ON CONFLICT(user_id,plan_date) DO UPDATE SET strategy=daily_plans.strategy RETURNING id,budget_minutes`, uid, planDate(), profile.DailyMinutes).Scan(&pid, &budget)
	if e != nil {
		return e
	}
	var allocated int
	if e = tx.QueryRow(ctx, `SELECT COALESCE(sum(estimated_minutes),0) FROM study_tasks WHERE plan_id=$1`, pid).Scan(&allocated); e != nil {
		return e
	}
	excluded := map[string]bool{}
	rows, e := tx.Query(ctx, `SELECT a.id,a.payload FROM study_tasks t JOIN learning_assets a ON a.id=t.asset_id WHERE t.plan_id=$1`, pid)
	if e != nil {
		return e
	}
	for rows.Next() {
		var a domain.Asset
		var raw []byte
		if e = rows.Scan(&a.ID, &raw); e != nil {
			rows.Close()
			return e
		}
		if e = json.Unmarshal(raw, &a.Payload); e != nil {
			rows.Close()
			return e
		}
		excluded[domain.Family(a)] = true
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	history := []domain.ReviewEvidence{}
	rows, e = tx.Query(ctx, `SELECT result,submitted_at FROM assessments WHERE user_id=$1 AND result IS NOT NULL ORDER BY submitted_at,id`, uid)
	if e != nil {
		return e
	}
	for rows.Next() {
		var raw []byte
		var at time.Time
		var result domain.AssessmentResult
		if e = rows.Scan(&raw, &at); e != nil {
			rows.Close()
			return e
		}
		if e = json.Unmarshal(raw, &result); e != nil {
			rows.Close()
			return e
		}
		for _, item := range result.Items {
			if item.Counted {
				family := item.FamilyID
				if family == "" {
					family = item.ID
				}
				history = append(history, domain.ReviewEvidence{FamilyID: family, PointID: item.PointID, Correct: item.Correct, At: at})
			}
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	assets, e := p.Assets(ctx, uid)
	if e != nil {
		return e
	}
	for _, candidate := range domain.PlanCandidates(assets, history, profile, time.Now()) {
		if len(excluded) >= 20 {
			break
		}
		a := candidate.Asset
		family := domain.Family(a)
		if excluded[family] || candidate.Minutes > budget-allocated {
			continue
		}
		tag, err := tx.Exec(ctx, `INSERT INTO study_tasks(user_id,asset_id,reason,plan_id,estimated_minutes,duration_source,priority) SELECT $1,id,$3,$4,$5,$6,$7 FROM learning_assets WHERE id=$2 AND (user_id=$1 OR is_public=true) AND status='ready' AND revision=$8 ON CONFLICT(user_id,asset_id) WHERE completed_at IS NULL DO UPDATE SET plan_id=EXCLUDED.plan_id,reason=EXCLUDED.reason,estimated_minutes=EXCLUDED.estimated_minutes,duration_source=EXCLUDED.duration_source,priority=EXCLUDED.priority WHERE study_tasks.plan_id IS DISTINCT FROM EXCLUDED.plan_id`, uid, a.ID, candidate.Reason, pid, candidate.Minutes, candidate.DurationSource, candidate.Priority, a.Revision)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 1 {
			allocated += candidate.Minutes
			excluded[family] = true
		}
	}
	return tx.Commit(ctx)
}

func (p *Postgres) PlanRecommendations(ctx context.Context, uid string) ([]domain.Recommendation, error) {
	tasks, e := p.Tasks(ctx, uid)
	if e != nil {
		return nil, e
	}
	out := []domain.Recommendation{}
	seen := map[string]bool{}
	points, e := p.ListKnowledgePoints(ctx)
	if e != nil {
		return nil, e
	}
	byID := map[string]domain.KnowledgePoint{}
	for _, point := range points {
		byID[point.ID] = point
	}
	for _, task := range tasks {
		if task.Completed || task.PlanDate != planDate() || seen[task.PointID] {
			continue
		}
		seen[task.PointID] = true
		point := byID[task.PointID]
		out = append(out, domain.Recommendation{UserID: uid, KnowledgePointID: task.PointID, KnowledgePointName: point.Name, SubjectName: point.SubjectName, Score: 1, Reason: task.Reason, RecommendedAction: "follow_daily_plan", Status: "active", GeneratedAt: time.Now(), ExpiresAt: time.Now().Add(24 * time.Hour)})
	}
	if e = p.ReplaceRecommendations(ctx, uid, out); e != nil {
		return nil, e
	}
	return p.ListRecommendations(ctx, uid)
}
