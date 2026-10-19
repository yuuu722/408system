package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/local/exam408-assistant/backend/internal/domain"
)

func (p *Postgres) BeginRevision(ctx context.Context, d domain.Document, s domain.RevisionSpec) (domain.Revision, error) {
	out := domain.Revision{}
	tx, e := p.executionTx(ctx, d)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	var fresh bool
	e = tx.QueryRow(ctx, `INSERT INTO document_revisions(document_id,fingerprint,file_hash,parse_key,chunk_key,embedding_space) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(document_id,fingerprint) DO NOTHING RETURNING id`, d.ID, s.Fingerprint, s.FileHash, s.ParseKey, s.ChunkKey, s.EmbeddingSpace).Scan(&out.ID)
	if e == pgx.ErrNoRows {
		e = tx.QueryRow(ctx, `SELECT id FROM document_revisions WHERE document_id=$1 AND fingerprint=$2`, d.ID, s.Fingerprint).Scan(&out.ID)
	} else if e == nil {
		fresh = true
	}
	if e != nil {
		return out, e
	}
	if fresh {
		var source string
		e = tx.QueryRow(ctx, `SELECT id FROM document_revisions WHERE document_id=$1 AND parse_key=$2 AND parsed_done ORDER BY created_at DESC LIMIT 1`, d.ID, s.ParseKey).Scan(&source)
		if e != nil && e != pgx.ErrNoRows {
			return out, e
		}
		if e == nil {
			if _, e = tx.Exec(ctx, `INSERT INTO revision_pages SELECT $1,ordinal,payload FROM revision_pages WHERE revision_id=$2`, out.ID, source); e != nil {
				return out, e
			}
			if _, e = tx.Exec(ctx, `UPDATE document_revisions SET parsed_done=true,total_pages=(SELECT total_pages FROM document_revisions WHERE id=$2) WHERE id=$1`, out.ID, source); e != nil {
				return out, e
			}
		}
		e = tx.QueryRow(ctx, `SELECT id FROM document_revisions WHERE document_id=$1 AND chunk_key=$2 AND chunked ORDER BY created_at DESC LIMIT 1`, d.ID, s.ChunkKey).Scan(&source)
		if e != nil && e != pgx.ErrNoRows {
			return out, e
		}
		if e == nil {
			if _, e = tx.Exec(ctx, `INSERT INTO revision_chunks SELECT $1::uuid,ordinal,content,metadata || jsonb_build_object('revision_id',$1::text,'embedding_space',$3::text),CASE WHEN r.embedding_space=$3 THEN c.embedding ELSE NULL END FROM revision_chunks c JOIN document_revisions r ON r.id=c.revision_id WHERE r.id=$2`, out.ID, source, s.EmbeddingSpace); e != nil {
				return out, e
			}
			if _, e = tx.Exec(ctx, `UPDATE document_revisions SET chunked=true WHERE id=$1`, out.ID); e != nil {
				return out, e
			}
		}
	}
	if e = tx.QueryRow(ctx, `SELECT parsed_done,chunked,total_pages FROM document_revisions WHERE id=$1`, out.ID).Scan(&out.ParsedDone, &out.Chunked, &out.TotalPages); e != nil {
		return out, e
	}
	return out, tx.Commit(ctx)
}

func (p *Postgres) SaveRevisionPage(ctx context.Context, d domain.Document, rid string, ordinal, total int, page domain.StructuredPage) error {
	tx, e := p.revisionTx(ctx, d, rid)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	raw, e := json.Marshal(page)
	if e != nil {
		return e
	}
	var used int64
	if e = tx.QueryRow(ctx, `SELECT COALESCE(sum(octet_length(payload::text)),0) FROM revision_pages WHERE revision_id=$1`, rid).Scan(&used); e != nil {
		return e
	}
	if used+int64(len(raw)) > 64<<20 {
		return fmt.Errorf("结构化结果超过64MiB，请按章节拆分文件（Parse result too large）")
	}
	if _, e = tx.Exec(ctx, `INSERT INTO revision_pages(revision_id,ordinal,payload) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, rid, ordinal, raw); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE document_revisions SET total_pages=$2 WHERE id=$1 AND (total_pages=0 OR total_pages=$2)`, rid, total); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (p *Postgres) RevisionPages(ctx context.Context, uid, did, rid string) ([]domain.StructuredPage, error) {
	rows, e := p.DB.Query(ctx, `SELECT p.payload FROM revision_pages p JOIN document_revisions r ON r.id=p.revision_id JOIN documents d ON d.id=r.document_id WHERE r.id=$1 AND d.id=$2 AND (d.user_id=$3 OR EXISTS(SELECT 1 FROM knowledge_bases kb WHERE kb.id=d.knowledge_base_id AND kb.is_public=true)) ORDER BY p.ordinal`, rid, did, uid)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.StructuredPage{}
	for rows.Next() {
		var raw []byte
		var page domain.StructuredPage
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(raw, &page); e != nil {
			return nil, e
		}
		out = append(out, page)
	}
	return out, rows.Err()
}
func (p *Postgres) CompleteRevisionParse(ctx context.Context, d domain.Document, rid string) error {
	tx, e := p.revisionTx(ctx, d, rid)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	tag, e := tx.Exec(ctx, `UPDATE document_revisions SET parsed_done=true WHERE id=$1 AND total_pages>0 AND total_pages=(SELECT count(*) FROM revision_pages WHERE revision_id=$1)`, rid)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("页面成果不完整（Incomplete pages）")
	}
	return tx.Commit(ctx)
}
func (p *Postgres) SaveRevisionChunks(ctx context.Context, d domain.Document, rid string, chunks []domain.Chunk) error {
	tx, e := p.revisionTx(ctx, d, rid)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `DELETE FROM revision_chunks WHERE revision_id=$1`, rid); e != nil {
		return e
	}
	for i, c := range chunks {
		raw, e := json.Marshal(c.Metadata)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO revision_chunks(revision_id,ordinal,content,metadata) VALUES($1,$2,$3,$4)`, rid, i, c.Content, raw); e != nil {
			return e
		}
	}
	if _, e = tx.Exec(ctx, `UPDATE document_revisions SET chunked=true WHERE id=$1 AND parsed_done`, rid); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (p *Postgres) PendingRevisionChunks(ctx context.Context, d domain.Document, rid string, limit int) ([]domain.Chunk, error) {
	rows, e := p.DB.Query(ctx, `SELECT ordinal,content,metadata FROM revision_chunks c JOIN document_revisions r ON r.id=c.revision_id WHERE r.id=$1 AND r.document_id=$2 AND c.embedding IS NULL ORDER BY ordinal LIMIT $3`, rid, d.ID, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.Chunk{}
	for rows.Next() {
		c := domain.Chunk{}
		var raw []byte
		if e = rows.Scan(&c.ChunkIndex, &c.Content, &raw); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(raw, &c.Metadata); e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (p *Postgres) SaveRevisionVectors(ctx context.Context, d domain.Document, rid string, chunks []domain.Chunk, vectors [][]float32) error {
	tx, e := p.revisionTx(ctx, d, rid)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	for i, c := range chunks {
		tag, e := tx.Exec(ctx, `UPDATE revision_chunks SET embedding=$4::vector WHERE revision_id=$1 AND ordinal=$2 AND content=$3 AND embedding IS NULL`, rid, c.ChunkIndex, c.Content, vectorLiteral(vectors[i]))
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("分块已改变（Chunk changed）")
		}
	}
	return tx.Commit(ctx)
}
func (p *Postgres) RevisionCounts(ctx context.Context, rid string) (int, int, error) {
	var total, done int
	e := p.DB.QueryRow(ctx, `SELECT count(*),count(embedding) FROM revision_chunks WHERE revision_id=$1`, rid).Scan(&total, &done)
	return total, done, e
}
func (p *Postgres) RevisionProgress(ctx context.Context, d domain.Document, stage string, percent, done, total int) error {
	tx, e := p.executionTx(ctx, d)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `UPDATE documents SET processing_stage=$2,progress=$3,processed_chunks=$4,total_chunks=$5,updated_at=now() WHERE id=$1 AND user_id=$6`, d.ID, stage, percent, done, total, d.UserID); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

func (p *Postgres) ActivateRevision(ctx context.Context, d domain.Document, rid string, parsed domain.ParsedDocument, assets []domain.Asset) error {
	tx, e := p.revisionTx(ctx, d, rid)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var valid bool
	e = tx.QueryRow(ctx, `SELECT parsed_done AND chunked AND (SELECT count(*)>0 AND count(*)=count(embedding) AND count(DISTINCT vector_dims(embedding))=1 FROM revision_chunks WHERE revision_id=$1) FROM document_revisions WHERE id=$1`, rid).Scan(&valid)
	if e != nil {
		return e
	}
	if !valid {
		return fmt.Errorf("处理成果不完整（Incomplete revision）")
	}
	if _, e = tx.Exec(ctx, `DELETE FROM knowledge_relations WHERE evidence_chunk_id IN (SELECT id FROM document_chunks WHERE document_id=$1 AND user_id=$2)`, d.ID, d.UserID); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `DELETE FROM exam408_resource_mappings WHERE document_id=$1 AND source='auto'`, d.ID); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `DELETE FROM document_chunks WHERE document_id=$1 AND user_id=$2`, d.ID, d.UserID); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO document_chunks(id,document_id,knowledge_base_id,user_id,content,chunk_index,source_location,metadata,embedding) SELECT gen_random_uuid(),$2,$3,$4,content,ordinal,'分块 '||(ordinal+1)::text,metadata,embedding FROM revision_chunks WHERE revision_id=$1`, rid, d.ID, d.KnowledgeBaseID, d.UserID); e != nil {
		return e
	}
	graphRows, e := tx.Query(ctx, `SELECT id,content,chunk_index,source_location FROM document_chunks WHERE document_id=$1 AND user_id=$2`, d.ID, d.UserID)
	if e != nil {
		return e
	}
	indexedChunks := []domain.Chunk{}
	for graphRows.Next() {
		var ch domain.Chunk
		ch.DocumentID, ch.KnowledgeBaseID, ch.UserID = d.ID, d.KnowledgeBaseID, d.UserID
		if e = graphRows.Scan(&ch.ID, &ch.Content, &ch.ChunkIndex, &ch.SourceLocation); e != nil {
			graphRows.Close()
			return e
		}
		indexedChunks = append(indexedChunks, ch)
	}
	if e = graphRows.Err(); e != nil {
		graphRows.Close()
		return e
	}
	graphRows.Close()
	if e = p.buildKnowledgeGraphFromChunks(ctx, tx, d.UserID, d.KnowledgeBaseID, indexedChunks); e != nil {
		return e
	}
	if e = p.pruneGraph(ctx, tx, d.KnowledgeBaseID); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO document_revision_files(revision_id,document_id,storage_path,original_filename,mime_type) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, rid, d.ID, d.StoragePath, d.OriginalFilename, d.MimeType); e != nil {
		return e
	}
	parsed.RevisionID = rid
	parsed.State = "complete"
	for i := range assets {
		assets[i].Payload.RevisionID = rid
	}
	if e = saveDocumentStructureTx(ctx, tx, d.UserID, d.ID, parsed, assets); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE document_revisions SET state='complete' WHERE id=$1`, rid); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE documents SET active_revision_id=$2,status='ready',progress=100,processing_stage='completed',processed_chunks=total_chunks,error_message='',updated_at=now() WHERE id=$1`, d.ID, rid); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE document_jobs SET state='completed',lease_until=NULL,last_error='',updated_at=now() WHERE document_id=$1 AND execution=$2`, d.ID, d.Execution); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (p *Postgres) revisionTx(ctx context.Context, d domain.Document, rid string) (pgx.Tx, error) {
	tx, e := p.executionTx(ctx, d)
	if e != nil {
		return nil, e
	}
	var valid bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM document_revisions WHERE id=$1 AND document_id=$2)`, rid, d.ID).Scan(&valid)
	if e != nil || !valid {
		tx.Rollback(ctx)
		if e != nil {
			return nil, e
		}
		return nil, ErrNotFound
	}
	return tx, nil
}
