package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"strings"
	"time"
)

// Only retain lessons whose cited textbooks are still readable and unchanged.
func (p *Postgres) lessonFingerprint(ctx context.Context, uid string, result domain.ChatResult, notAfter ...time.Time) (string, error) {
	ids := []string{}
	seen := map[string]bool{}
	for _, c := range result.Sources {
		if c.DocumentID != "" && !seen[c.DocumentID] {
			ids = append(ids, c.DocumentID)
			seen[c.DocumentID] = true
		}
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("讲解缺少可核对的教材来源")
	}
	var fingerprint string
	var count int
	var latest time.Time
	e := p.DB.QueryRow(ctx, `SELECT count(*),COALESCE(string_agg(d.id::text||':'||d.updated_at::text||':'||COALESCE(d.active_revision_id::text,'')||':'||COALESCE(dc.resource_type,d.resource_type)||':'||COALESCE(dc.purpose,'knowledge')||':'||COALESCE(dc.updated_at::text,''),'|' ORDER BY d.id),''),COALESCE(max(GREATEST(d.updated_at,COALESCE(dc.updated_at,d.updated_at))),'epoch'::timestamptz) FROM documents d JOIN knowledge_bases kb ON kb.id=d.knowledge_base_id LEFT JOIN document_catalog dc ON dc.document_id=d.id WHERE d.id=ANY($2::uuid[]) AND (kb.is_public OR kb.user_id=$1) AND (d.status='ready' OR d.active_revision_id IS NOT NULL) AND COALESCE(dc.resource_type,d.resource_type)='textbook' AND COALESCE(dc.purpose,'knowledge')<>'questions'`, uid, ids).Scan(&count, &fingerprint, &latest)
	if e != nil {
		return "", e
	}
	if len(notAfter) > 0 && latest.After(notAfter[0]) {
		return "", fmt.Errorf("教材在讲解生成期间发生更新，请重新整理")
	}
	if count != len(ids) {
		return "", fmt.Errorf("部分教材来源已不可访问或分类发生变化，请重新整理讲解")
	}
	return fingerprint, nil
}
func (p *Postgres) UnitLesson(ctx context.Context, uid, point, key string) (*domain.ChatResult, error) {
	var raw []byte
	var saved string
	e := p.DB.QueryRow(ctx, `SELECT payload,source_fingerprint FROM learning_unit_lessons WHERE user_id=$1 AND point_id=$2 AND resource_key=$3`, uid, point, key).Scan(&raw, &saved)
	if e != nil {
		if nf(e) == ErrNotFound {
			return nil, nil
		}
		return nil, e
	}
	var result domain.ChatResult
	if e = json.Unmarshal(raw, &result); e != nil {
		return nil, e
	}
	current, e := p.lessonFingerprint(ctx, uid, result)
	if e != nil || current != saved {
		return nil, nil
	}
	return &result, nil
}
func (p *Postgres) BuildUnitLesson(ctx context.Context, uid, point, key string, build func() (domain.ChatResult, error)) (domain.ChatResult, error) {
	tx, e := p.DB.Begin(ctx)
	if e != nil {
		return domain.ChatResult{}, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,410))`, uid+point+key); e != nil {
		return domain.ChatResult{}, e
	}
	existing, e := p.UnitLesson(ctx, uid, point, key)
	if e != nil {
		return domain.ChatResult{}, e
	}
	if existing != nil {
		return *existing, nil
	}
	var started time.Time
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&started); e != nil {
		return domain.ChatResult{}, e
	}
	result, e := build()
	if e != nil {
		return result, e
	}
	if result.Generation == nil || !result.Generation.Verified || strings.TrimSpace(result.Answer) == "" {
		return domain.ChatResult{}, fmt.Errorf("知识讲解尚未通过原文核验，请重试；不会用分块摘录替代完整讲解")
	}
	fingerprint, e := p.lessonFingerprint(ctx, uid, result, started)
	if e != nil {
		return domain.ChatResult{}, e
	}
	raw, e := json.Marshal(result)
	if e != nil {
		return result, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO learning_unit_lessons(user_id,point_id,resource_key,payload,source_fingerprint) VALUES($1,$2,$3,$4,$5) ON CONFLICT(user_id,point_id,resource_key) DO UPDATE SET payload=EXCLUDED.payload,source_fingerprint=EXCLUDED.source_fingerprint,created_at=now()`, uid, point, key, raw, fingerprint)
	if e != nil {
		return result, e
	}
	return result, tx.Commit(ctx)
}

func (p *Postgres) ExampleAttempt(ctx context.Context, uid, point, key string) (*domain.Assessment, error) {
	a, e := scanAssessment(p.DB.QueryRow(ctx, `SELECT a.id,a.kind,a.questions,a.result,a.created_at FROM learning_example_attempts x JOIN assessments a ON a.id=x.assessment_id WHERE x.user_id=$1 AND x.point_id=$2 AND x.resource_key=$3 AND a.user_id=$1 ORDER BY a.created_at DESC LIMIT 1`, uid, point, key))
	if e != nil {
		if nf(e) == ErrNotFound {
			return nil, nil
		}
		return nil, e
	}
	a = RedactAssessment(a)
	return &a, nil
}
func (p *Postgres) ExampleResource(ctx context.Context, uid, point, id string, revision int) (domain.Asset, string, error) {
	points, e := p.ListKnowledgePoints(ctx)
	if e != nil {
		return domain.Asset{}, "", e
	}
	exists := false
	for _, v := range points {
		exists = exists || v.ID == point
	}
	if !exists {
		return domain.Asset{}, "", ErrNotFound
	}
	q, e := p.Asset(ctx, uid, id)
	if e != nil {
		return q, "", e
	}
	scope := domain.PointScope(points, point)
	if !scope[q.PointID] || q.Kind != "question" || q.Status != "ready" || q.Payload.AnswerOrigin != "source" || domain.QuestionTextIssue(q) != "" {
		return q, "", domain.ErrForbidden
	}
	if q.Revision != revision {
		return q, "", &domain.RequestError{Status: 409, Message: "例题资料已更新，请刷新后重新作答"}
	}
	return q, resourceKey(q.ID, fmt.Sprint(q.Revision)), nil
}
func (p *Postgres) CreateExampleAttempt(ctx context.Context, uid, point, id string, revision int, retry bool) (domain.Assessment, error) {
	tx, e := p.DB.Begin(ctx)
	if e != nil {
		return domain.Assessment{}, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,409))`, uid); e != nil {
		return domain.Assessment{}, e
	}
	q, key, e := p.ExampleResource(ctx, uid, point, id, revision)
	if e != nil {
		return domain.Assessment{}, e
	}
	old, e := p.ExampleAttempt(ctx, uid, point, key)
	if e != nil {
		return domain.Assessment{}, e
	}
	if old != nil && (!retry || old.Result == nil) {
		return *old, nil
	}
	a := domain.Assessment{Kind: "practice", Questions: []domain.Asset{q}}
	raw, _ := json.Marshal(a.Questions)
	e = tx.QueryRow(ctx, `INSERT INTO assessments(user_id,kind,questions) VALUES($1,'practice',$2) RETURNING id,created_at`, uid, raw).Scan(&a.ID, &a.CreatedAt)
	if e != nil {
		return a, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO learning_example_attempts(user_id,point_id,resource_key,assessment_id) VALUES($1,$2,$3,$4)`, uid, point, key, a.ID)
	if e != nil {
		return a, e
	}
	if e = tx.Commit(ctx); e != nil {
		return a, e
	}
	return RedactAssessment(a), nil
}
func (p *Postgres) ExampleFeedback(ctx context.Context, uid, assessment string) (*domain.ChatResult, error) {
	var raw []byte
	e := p.DB.QueryRow(ctx, `SELECT feedback FROM learning_example_attempts WHERE user_id=$1 AND assessment_id=$2`, uid, assessment).Scan(&raw)
	if e != nil {
		return nil, nf(e)
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var result domain.ChatResult
	e = json.Unmarshal(raw, &result)
	return &result, e
}
func (p *Postgres) SaveExampleFeedback(ctx context.Context, uid, assessment string, result domain.ChatResult) error {
	raw, _ := json.Marshal(result)
	tag, e := p.DB.Exec(ctx, `UPDATE learning_example_attempts SET feedback=$3 WHERE user_id=$1 AND assessment_id=$2`, uid, assessment, raw)
	if e == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return e
}

func (p *Postgres) PreferredExampleID(ctx context.Context, uid, point string) (string, error) {
	var id string
	e := p.DB.QueryRow(ctx, `SELECT a.questions->0->>'ID' FROM learning_example_attempts x JOIN assessments a ON a.id=x.assessment_id WHERE x.user_id=$1 AND x.point_id=$2 AND a.user_id=$1 ORDER BY a.created_at DESC LIMIT 1`, uid, point).Scan(&id)
	if nf(e) == ErrNotFound {
		return "", nil
	}
	return id, e
}
