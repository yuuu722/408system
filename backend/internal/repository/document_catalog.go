package repository

import (
	"context"
	"strings"

	"github.com/local/exam408-assistant/backend/internal/domain"
)

func (p *Postgres) Catalogs(ctx context.Context, uid, kid string) ([]domain.DocumentCatalog, error) {
	if _, e := p.KnowledgeBase(ctx, uid, kid); e != nil {
		return nil, e
	}
	rows, e := p.DB.Query(ctx, `SELECT c.document_id,c.resource_type,c.subject_code,c.purpose,c.review_state,c.reason FROM document_catalog c JOIN documents d ON d.id=c.document_id WHERE d.knowledge_base_id=$1`, kid)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.DocumentCatalog{}
	for rows.Next() {
		var c domain.DocumentCatalog
		if e = rows.Scan(&c.DocumentID, &c.ResourceType, &c.SubjectCode, &c.Purpose, &c.ReviewState, &c.Reason); e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (p *Postgres) SaveCatalog(ctx context.Context, uid string, c domain.DocumentCatalog) error {
	if e := p.CanMaintainDocument(ctx, uid, c.DocumentID); e != nil {
		return e
	}
	if c.Purpose != "knowledge" && c.Purpose != "questions" && c.Purpose != "both" {
		return catalogInputError("请选择知识库、题库或两者用途")
	}
	valid := map[string]bool{"textbook": true, "past_exam": true, "solution": true, "courseware": true, "notes": true, "outline": true, "other": true}
	if !valid[c.ResourceType] {
		return catalogInputError("不支持的资料类型")
	}
	if c.SubjectCode != "" && c.SubjectCode != "ds" && c.SubjectCode != "co" && c.SubjectCode != "os" && c.SubjectCode != "cn" && c.SubjectCode != "general" {
		return catalogInputError("不支持的学科")
	}
	if c.ReviewState != "confirmed" && c.ReviewState != "pending" {
		return catalogInputError("分类状态不正确")
	}
	if c.ReviewState == "confirmed" && (c.ResourceType == "other" || c.SubjectCode == "") {
		return catalogInputError("确认前请选择明确类型与学科；跨科资料可选择综合")
	}
	tx, e := p.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	// Do not change source/usage while a worker is extracting it.
	var status string
	if e = tx.QueryRow(ctx, `SELECT status FROM documents WHERE id=$1 FOR UPDATE`, c.DocumentID).Scan(&status); e != nil {
		return e
	}
	if status == "processing" || status == "uploaded" {
		return &domain.RequestError{Status: 409, Message: "资料正在处理，完成后再整理分类"}
	}
	_, e = tx.Exec(ctx, `INSERT INTO document_catalog(document_id,resource_type,subject_code,purpose,review_state,reason) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(document_id) DO UPDATE SET resource_type=EXCLUDED.resource_type,subject_code=EXCLUDED.subject_code,purpose=EXCLUDED.purpose,review_state=EXCLUDED.review_state,reason=EXCLUDED.reason,updated_at=now()`, c.DocumentID, c.ResourceType, c.SubjectCode, c.Purpose, c.ReviewState, c.Reason)
	if e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE documents SET resource_type=$2 WHERE id=$1`, c.DocumentID, c.ResourceType); e != nil {
		return e
	}
	if c.Purpose == "knowledge" {
		if _, e = tx.Exec(ctx, `UPDATE learning_assets SET status='unavailable',payload=jsonb_set(payload,'{ReviewStatus}','"pending"'::jsonb),revision=revision+1 WHERE document_id=$1 AND COALESCE(payload->>'ExtractionKey','')<>''`, c.DocumentID); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}

// Rebuild only from the still-active parse. Lock the document against a worker
// activation/replacement so a stale browser request cannot overwrite new data.
func (p *Postgres) RefreshQuestionCandidates(ctx context.Context, actor string, d domain.Document, parsed domain.ParsedDocument, assets []domain.Asset) error {
	if e := p.CanMaintainDocument(ctx, actor, d.ID); e != nil {
		return e
	}
	tx, e := p.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var status, rid string
	if e = tx.QueryRow(ctx, `SELECT status,COALESCE(active_revision_id::text,'') FROM documents WHERE id=$1 FOR UPDATE`, d.ID).Scan(&status, &rid); e != nil {
		return e
	}
	if status != "ready" || rid != d.ActiveRevisionID {
		return &domain.RequestError{Status: 409, Message: "资料版本已变化或正在处理，请刷新后再整理"}
	}
	if e = saveDocumentStructureTx(ctx, tx, d.UserID, d.ID, parsed, assets); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

func catalogInputError(message string) error {
	return &domain.RequestError{Status: 400, Message: message}
}

func (p *Postgres) UpdatePersonalKnowledgeBase(ctx context.Context, uid, kid, name, description string) (domain.KnowledgeBase, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 100 {
		return domain.KnowledgeBase{}, catalogInputError("知识库名称须为1至100字")
	}
	return scanKB(p.DB.QueryRow(ctx, `UPDATE knowledge_bases SET name=$3,description=$4,updated_at=now() WHERE id=$1 AND user_id=$2 AND is_public=false RETURNING id,user_id,name,description,created_at,updated_at,is_public`, kid, uid, name, description))
}
