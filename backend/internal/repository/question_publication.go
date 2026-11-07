package repository

import (
	"context"
	"encoding/json"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"sort"
)

type QuestionSourceSnapshot struct {
	Document domain.Document
	Parsed   domain.ParsedDocument
}

// Commit only server-verified questions. Lock source revisions and stored parses,
// then compare candidate revisions so concurrent edits can never be overwritten.
func (p *Postgres) PublishVerifiedQuestions(ctx context.Context, uid string, qdoc domain.Document, updates []domain.Asset, sources map[string]QuestionSourceSnapshot) (int, error) {
	if e := p.CanMaintainDocument(ctx, uid, qdoc.ID); e != nil {
		return 0, e
	}
	tx, e := p.DB.Begin(ctx)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback(ctx)
	ids := []string{}
	for id := range sources {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	stale := func() error {
		return &domain.RequestError{Status: 409, Message: "题目或来源已变化，请重试自动整理"}
	}
	for _, id := range ids {
		source := sources[id]
		var status, revision string
		e = tx.QueryRow(ctx, `SELECT status,COALESCE(active_revision_id::text,'') FROM documents WHERE id=$1 FOR SHARE`, id).Scan(&status, &revision)
		if e != nil {
			return 0, e
		}
		if status != "ready" || revision != source.Document.ActiveRevisionID {
			return 0, stale()
		}
		parsed, err := questionStructureTx(ctx, tx, id)
		if err != nil {
			return 0, err
		}
		if ParsedFingerprint(parsed) != ParsedFingerprint(source.Parsed) {
			return 0, stale()
		}
	}
	var purpose string
	if e = tx.QueryRow(ctx, `SELECT purpose FROM document_catalog WHERE document_id=$1 FOR SHARE`, qdoc.ID).Scan(&purpose); e != nil {
		return 0, e
	}
	if purpose != "questions" && purpose != "both" {
		return 0, stale()
	}
	sort.Slice(updates, func(i, j int) bool { return updates[i].ID < updates[j].ID })
	for _, a := range updates {
		domain.PrepareAsset(&a)
		if a.Status != "ready" || a.Payload.ReviewStatus != "published" || !domain.HasQuestionExplanation(a.Payload.Explanation) {
			return 0, domain.ErrForbidden
		}
		raw, _ := json.Marshal(a.Payload)
		tag, e := tx.Exec(ctx, `UPDATE learning_assets SET payload=$4,point_id=$5::uuid,status='ready',revision=revision+1,updated_at=now() WHERE id=$1 AND document_id=$2 AND revision=$3 AND COALESCE(payload->>'ReviewStatus','')<>'published' AND is_public=true`, a.ID, qdoc.ID, a.Revision, raw, a.PointID)
		if e != nil {
			return 0, e
		}
		if tag.RowsAffected() != 1 {
			return 0, stale()
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return 0, e
	}
	return len(updates), nil
}
