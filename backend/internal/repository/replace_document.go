package repository

import (
	"context"
	"github.com/local/exam408-assistant/backend/internal/domain"
)

// ReplaceDocument preserves document identity and its old active revision until
// new processing commits. A new execution fences any worker still reading the old file.
func (p *Postgres) ReplaceDocument(ctx context.Context, actor string, d domain.Document) (domain.Document, error) {
	if e := p.CanMaintainDocument(ctx, actor, d.ID); e != nil {
		return d, e
	}
	tx, e := p.DB.Begin(ctx)
	if e != nil {
		return d, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT document_id FROM document_jobs WHERE document_id=$1 FOR UPDATE`, d.ID); e != nil {
		return d, e
	}
	tag, e := tx.Exec(ctx, `UPDATE documents SET filename=$2,original_filename=$3,mime_type=$4,size=$5,storage_path=$6,resource_type=$7,status='processing',processing_stage='queued',progress=0,error_message='',updated_at=now() WHERE id=$1`, d.ID, d.Filename, d.OriginalFilename, d.MimeType, d.Size, d.StoragePath, d.ResourceType)
	if e != nil {
		return d, e
	}
	if tag.RowsAffected() != 1 {
		return d, ErrNotFound
	}
	if d.Purpose != "" {
		if _, e = tx.Exec(ctx, `INSERT INTO document_catalog(document_id,resource_type,purpose) VALUES($1,$2,$3) ON CONFLICT(document_id) DO UPDATE SET resource_type=EXCLUDED.resource_type,purpose=EXCLUDED.purpose,review_state='pending',updated_at=now()`, d.ID, d.ResourceType, d.Purpose); e != nil {
			return d, e
		}
	}
	if _, e = tx.Exec(ctx, `INSERT INTO document_jobs(document_id) VALUES($1) ON CONFLICT(document_id) DO UPDATE SET state='queued',execution=document_jobs.execution+1,attempts=0,lease_until=NULL,last_error='',available_at=now(),updated_at=now()`, d.ID); e != nil {
		return d, e
	}
	if e = tx.Commit(ctx); e != nil {
		return d, e
	}
	return p.Document(ctx, actor, d.ID)
}

func (p *Postgres) RevisionSource(ctx context.Context, actor, did, rid string) (domain.Document, error) {
	d, e := p.Document(ctx, actor, did)
	if e != nil {
		return d, e
	}
	e = p.DB.QueryRow(ctx, `SELECT storage_path,original_filename,mime_type FROM document_revision_files WHERE document_id=$1 AND revision_id=$2`, did, rid).Scan(&d.StoragePath, &d.OriginalFilename, &d.MimeType)
	return d, nf(e)
}
