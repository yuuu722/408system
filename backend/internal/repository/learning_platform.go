package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"time"
)

func (p *Postgres) Profile(ctx context.Context, uid string) (*domain.Profile, error) {
	var raw []byte
	err := p.DB.QueryRow(ctx, `SELECT payload FROM learning_profiles WHERE user_id=$1`, uid).Scan(&raw)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var v domain.Profile
	err = json.Unmarshal(raw, &v)
	return &v, err
}
func (p *Postgres) SaveProfile(ctx context.Context, uid string, v domain.Profile) error {
	raw, _ := json.Marshal(v)
	_, err := p.DB.Exec(ctx, `INSERT INTO learning_profiles(user_id,payload) VALUES($1,$2) ON CONFLICT(user_id) DO UPDATE SET payload=$2,updated_at=now()`, uid, raw)
	return err
}

func scanAsset(row pgx.Row) (a domain.Asset, err error) {
	var raw []byte
	err = row.Scan(&a.ID, &a.Kind, &a.PointID, &a.Title, &a.Status, &a.Revision, &raw, &a.IsPublic)
	if err != nil {
		return a, nf(err)
	}
	err = json.Unmarshal(raw, &a.Payload)
	return
}

const assetCols = `id,kind,COALESCE(point_id::text,''),title,status,revision,payload,is_public`

func (p *Postgres) Assets(ctx context.Context, uid string) ([]domain.Asset, error) {
	rows, err := p.DB.Query(ctx, `SELECT `+assetCols+` FROM learning_assets WHERE (user_id=$1 OR is_public=true) AND (COALESCE(payload->>'ExtractionKey','')='' OR NOT EXISTS(SELECT 1 FROM document_catalog dc WHERE dc.document_id=learning_assets.document_id AND dc.purpose='knowledge')) ORDER BY is_public DESC,updated_at DESC`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Asset{}
	for rows.Next() {
		a, e := scanAsset(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (p *Postgres) SaveAsset(ctx context.Context, uid string, a domain.Asset) (domain.Asset, error) {
	if a.ID != "" {
		old, err := p.Asset(ctx, uid, a.ID)
		if err != nil {
			return a, err
		}
		if old.IsPublic != a.IsPublic {
			return a, domain.ErrForbidden
		}
	}
	if a.IsPublic {
		u, err := p.UserByID(ctx, uid)
		if err != nil {
			return a, err
		}
		if u.Role != "admin" {
			return a, domain.ErrForbidden
		}
	}
	if a.Payload.DocumentID != "" {
		d, e := p.Document(ctx, uid, a.Payload.DocumentID)
		if e != nil {
			return a, e
		}
		if a.Payload.ReviewStatus == "published" && a.Kind == "question" {
			var purpose string
			if e = p.DB.QueryRow(ctx, `SELECT COALESCE((SELECT purpose FROM document_catalog WHERE document_id=$1),'knowledge')`, d.ID).Scan(&purpose); e != nil {
				return a, e
			}
			if purpose == "knowledge" {
				return a, &domain.RequestError{Status: 409, Message: "请先将原文件用途设置为题库或知识库与题库"}
			}
		}
		if a.IsPublic && a.Kind == "material" && a.Payload.Stem != "" && d.Status == "processing" {
			return a, &domain.RequestError{Status: 409, Message: "资料正在处理，请完成后再编辑"}
		}
		if a.IsPublic {
			kb, e := p.KnowledgeBase(ctx, uid, d.KnowledgeBaseID)
			if e != nil {
				return a, e
			}
			if !kb.IsPublic {
				return a, domain.ErrForbidden
			}
		}
	}
	if a.PointID != "" {
		if _, e := p.KnowledgePoint(ctx, a.PointID); e != nil {
			return a, e
		}
	}
	domain.PrepareAsset(&a)
	raw, _ := json.Marshal(a.Payload)
	if a.ID == "" {
		return scanAsset(p.DB.QueryRow(ctx, `INSERT INTO learning_assets(user_id,kind,point_id,title,payload,status,document_id,is_public) VALUES($1,$2,NULLIF($3,'')::uuid,$4,$5,$6,NULLIF($7,'')::uuid,$8) RETURNING `+assetCols, uid, a.Kind, a.PointID, a.Title, raw, a.Status, a.Payload.DocumentID, a.IsPublic))
	}
	return scanAsset(p.DB.QueryRow(ctx, `UPDATE learning_assets SET kind=$3,point_id=NULLIF($4,'')::uuid,title=$5,payload=$6,status=$8,document_id=NULLIF($9,'')::uuid,revision=revision+1,updated_at=now() WHERE id=$1 AND (user_id=$2 AND is_public=false OR is_public=true AND EXISTS(SELECT 1 FROM users WHERE id=$2 AND role='admin')) AND revision=$7 AND is_public=$10 RETURNING `+assetCols, a.ID, uid, a.Kind, a.PointID, a.Title, raw, a.Revision, a.Status, a.Payload.DocumentID, a.IsPublic))
}
func (p *Postgres) Asset(ctx context.Context, uid, aid string) (domain.Asset, error) {
	return scanAsset(p.DB.QueryRow(ctx, `SELECT `+assetCols+` FROM learning_assets WHERE id=$1 AND (user_id=$2 OR is_public=true)`, aid, uid))
}
func (p *Postgres) DeleteAsset(ctx context.Context, uid, aid string) error {
	tag, err := p.DB.Exec(ctx, `DELETE FROM learning_assets WHERE id=$1 AND (user_id=$2 AND is_public=false OR is_public=true AND EXISTS(SELECT 1 FROM users WHERE id=$2 AND role='admin'))`, aid, uid)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func RedactAssessment(a domain.Assessment) domain.Assessment {
	if a.Result == nil {
		for i := range a.Questions {
			a.Questions[i].Payload.Answer = -1
			a.Questions[i].Payload.Explanation = ""
		}
	}
	return a
}
func (p *Postgres) CreateAssessment(ctx context.Context, uid, kind, point string, assetIDs ...string) (domain.Assessment, error) {
	a := domain.Assessment{Kind: kind, Questions: []domain.Asset{}}
	assetID := ""
	if len(assetIDs) > 0 {
		assetID = assetIDs[0]
	}
	rows, err := p.DB.Query(ctx, `WITH eligible AS (SELECT a.*,k.subject_id,row_number() OVER(PARTITION BY COALESCE(NULLIF(a.payload->>'FamilyID',''),a.id::text) ORDER BY a.created_at,a.id) family_rank FROM learning_assets a JOIN exam408_knowledge_points k ON k.id=a.point_id WHERE (a.user_id=$1 OR a.is_public=true) AND a.kind='question' AND a.status='ready' AND ($2='' OR a.point_id::text=$2) AND ($4='' OR a.id::text=$4) AND ($3<>'retest' OR NOT EXISTS(SELECT 1 FROM assessments t,jsonb_array_elements(t.questions) q WHERE t.user_id=$1 AND t.result IS NOT NULL AND COALESCE(NULLIF(q->'Payload'->>'FamilyID',''),q->>'ID')=COALESCE(NULLIF(a.payload->>'FamilyID',''),a.id::text)))) SELECT a.id,a.kind,COALESCE(a.point_id::text,''),a.title,a.status,a.revision,a.payload,a.is_public FROM eligible a WHERE family_rank=1 ORDER BY row_number() OVER(PARTITION BY a.subject_id ORDER BY a.created_at,a.id),a.subject_id,a.id LIMIT 12`, uid, point, kind, assetID)
	if err != nil {
		return a, err
	}
	for rows.Next() {
		q, e := scanAsset(rows)
		if e != nil {
			rows.Close()
			return a, e
		}
		a.Questions = append(a.Questions, q)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return a, err
	}
	if len(a.Questions) == 0 {
		return a, fmt.Errorf("暂无符合条件的可用题目；复测需要未作答的新题（No available questions）")
	}
	raw, _ := json.Marshal(a.Questions)
	err = p.DB.QueryRow(ctx, `INSERT INTO assessments(user_id,kind,questions) VALUES($1,$2,$3) RETURNING id,created_at`, uid, kind, raw).Scan(&a.ID, &a.CreatedAt)
	return RedactAssessment(a), err
}
func scanAssessment(row pgx.Row) (a domain.Assessment, err error) {
	var q, r []byte
	err = row.Scan(&a.ID, &a.Kind, &q, &r, &a.CreatedAt)
	if err != nil {
		return a, nf(err)
	}
	if err = json.Unmarshal(q, &a.Questions); err != nil {
		return
	}
	if len(r) > 0 {
		err = json.Unmarshal(r, &a.Result)
	}
	return
}
func (p *Postgres) Assessments(ctx context.Context, uid string) ([]domain.Assessment, error) {
	rows, err := p.DB.Query(ctx, `SELECT id,kind,questions,result,created_at FROM assessments WHERE user_id=$1 ORDER BY created_at DESC LIMIT 50`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Assessment{}
	for rows.Next() {
		a, e := scanAssessment(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, RedactAssessment(a))
	}
	return out, rows.Err()
}

func Grade(questions []domain.Asset, answers map[string]int) (domain.AssessmentResult, error) {
	r := domain.AssessmentResult{Total: len(questions), Items: []domain.QuestionResult{}}
	if len(answers) != len(questions) {
		return r, fmt.Errorf("请完成全部题目，不确定时选择未掌握（Incomplete answers）")
	}
	for _, q := range questions {
		v, ok := answers[q.ID]
		if !ok || v < -1 || v >= len(q.Payload.Options) {
			return r, fmt.Errorf("答案格式不正确（Invalid answer）")
		}
		origin := q.Payload.AnswerOrigin
		if origin == "" {
			origin = "source"
		}
		counted := origin == "source"
		correct := counted && v >= 0 && v == q.Payload.Answer
		if counted {
			r.Counted++
		}
		if correct {
			r.Correct++
		}
		r.Items = append(r.Items, domain.QuestionResult{ID: q.ID, FamilyID: domain.Family(q), RevisionID: q.Payload.RevisionID, PointID: q.PointID, Selected: v, Answer: q.Payload.Answer, Correct: correct, Explanation: q.Payload.Explanation, AnswerOrigin: origin, Counted: counted, Confirmation: "uncertain"})
	}
	return r, nil
}
func (p *Postgres) SubmitAssessment(ctx context.Context, uid, aid string, answers map[string]int) (domain.AssessmentResult, error) {
	tx, err := p.DB.Begin(ctx)
	if err != nil {
		return domain.AssessmentResult{}, err
	}
	defer tx.Rollback(ctx)
	a, err := scanAssessment(tx.QueryRow(ctx, `SELECT id,kind,questions,result,created_at FROM assessments WHERE id=$1 AND user_id=$2 FOR UPDATE`, aid, uid))
	if err != nil {
		return domain.AssessmentResult{}, err
	}
	if a.Result != nil {
		return *a.Result, nil
	}
	result, err := Grade(a.Questions, answers)
	if err != nil {
		return result, err
	}
	raw, _ := json.Marshal(result)
	_, err = tx.Exec(ctx, `UPDATE assessments SET result=$2,submitted_at=now() WHERE id=$1`, aid, raw)
	if err != nil {
		return result, err
	}
	if err = syncAssessmentEvents(ctx, tx, uid, aid, result); err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
func (p *Postgres) Radar(ctx context.Context, uid string) ([]domain.RadarSubject, error) {
	rows, err := p.DB.Query(ctx, `WITH latest AS (SELECT DISTINCT ON(COALESCE(NULLIF(metadata->>'family_id',''),metadata->>'question_id')) subject_id,knowledge_point_id,event_type FROM exam408_learning_events WHERE user_id=$1 AND event_type IN ('quiz_correct','quiz_wrong') AND metadata ? 'question_id' ORDER BY COALESCE(NULLIF(metadata->>'family_id',''),metadata->>'question_id'),created_at DESC,metadata->>'assessment_id' DESC,metadata->>'question_id' DESC) SELECT s.id,s.name,(SELECT count(*) FROM exam408_knowledge_points WHERE subject_id=s.id),count(l.event_type),count(*) FILTER(WHERE l.event_type='quiz_correct'),count(DISTINCT l.knowledge_point_id) FROM exam408_subjects s LEFT JOIN latest l ON l.subject_id=s.id GROUP BY s.id ORDER BY s.sort_order`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.RadarSubject{}
	for rows.Next() {
		var v domain.RadarSubject
		if err = rows.Scan(&v.ID, &v.Name, &v.TotalPoints, &v.Tested, &v.Correct, &v.CoveredPoints); err != nil {
			return nil, err
		}
		if v.Tested > 0 {
			x := float64(v.Correct) / float64(v.Tested)
			v.Accuracy = &x
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (p *Postgres) Tasks(ctx context.Context, uid string) ([]domain.StudyTask, error) {
	rows, err := p.DB.Query(ctx, `SELECT t.id,t.asset_id,a.title,a.kind,a.point_id,t.reason,a.payload,t.completed_at,COALESCE(dp.plan_date::text,''),t.estimated_minutes,t.duration_source FROM study_tasks t JOIN learning_assets a ON a.id=t.asset_id LEFT JOIN daily_plans dp ON dp.id=t.plan_id WHERE t.user_id=$1 AND (a.user_id=$1 OR a.is_public=true) AND a.status='ready' AND (t.plan_id IS NULL OR dp.plan_date=$2::date) ORDER BY t.priority DESC,t.created_at LIMIT 100`, uid, planDate())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.StudyTask{}
	for rows.Next() {
		var t domain.StudyTask
		var raw []byte
		var at *time.Time
		if err = rows.Scan(&t.ID, &t.AssetID, &t.Title, &t.Kind, &t.PointID, &t.Reason, &raw, &at, &t.PlanDate, &t.EstimatedMinutes, &t.DurationSource); err != nil {
			return nil, err
		}
		var payload domain.AssetPayload
		if err = json.Unmarshal(raw, &payload); err != nil {
			return nil, err
		}
		t.Completed = at != nil
		t.URL = payload.URL
		t.DocumentID = payload.DocumentID
		t.Page = payload.Page
		t.StartSeconds = payload.StartSeconds
		t.EndSeconds = payload.EndSeconds
		out = append(out, t)
	}
	return out, rows.Err()
}
func (p *Postgres) CompleteTask(ctx context.Context, uid, tid string) error {
	tag, err := p.DB.Exec(ctx, `UPDATE study_tasks SET completed_at=COALESCE(completed_at,now()) WHERE id=$1 AND user_id=$2`, tid, uid)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
