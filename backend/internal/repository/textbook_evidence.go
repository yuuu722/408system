package repository

import (
	"context"
	"encoding/json"
	"github.com/local/exam408-assistant/backend/internal/domain"
)

// Supplement vectors with literal textbook evidence for the requested intent.
// Scope and document lifecycle checks are identical to ordinary retrieval.
func (p *Postgres) SearchTextbookEvidence(ctx context.Context, uid, kid string, topics, needs []string, limit int) ([]domain.Chunk, error) {
	if _, e := p.KnowledgeBase(ctx, uid, kid); e != nil {
		return nil, e
	}
	if len(topics) == 0 || len(needs) == 0 {
		return []domain.Chunk{}, nil
	}
	limit = min(24, max(1, limit))
	rows, e := p.DB.Query(ctx, `WITH eligible AS MATERIALIZED (
 SELECT c.*,d.original_filename,kb.is_public,
 replace(translate(lower(c.content),' '||chr(9)||chr(10)||chr(13)||chr(160)||chr(12288),''),' ','') body,
 replace(translate(lower(c.content||' '||COALESCE(c.metadata->>'heading_path','')),' '||chr(9)||chr(10)||chr(13)||chr(160)||chr(12288),''),' ','') scope
 FROM document_chunks c JOIN documents d ON d.id=c.document_id JOIN knowledge_bases kb ON kb.id=c.knowledge_base_id
 LEFT JOIN document_catalog dc ON dc.document_id=d.id
 WHERE c.knowledge_base_id=$2 AND (kb.is_public OR kb.user_id=$1)
 AND COALESCE(dc.resource_type,d.resource_type)='textbook'
 AND COALESCE(dc.purpose,'knowledge')<>'questions'
 AND (d.status='ready' OR d.active_revision_id IS NOT NULL)
 AND c.metadata->>'embedding_space'=$6
 ), scored AS (
 SELECT e.*,(SELECT count(*) FROM unnest($4::text[]) term WHERE strpos(e.body,lower(term))>0) hits,
 (SELECT count(*) FROM unnest($3::text[]) topic WHERE strpos(e.body,lower(topic))>0) topic_hits
 FROM eligible e WHERE EXISTS(SELECT 1 FROM unnest($3::text[]) topic WHERE strpos(e.scope,lower(topic))>0)
 ) SELECT id,document_id,knowledge_base_id,user_id,content,chunk_index,source_location,
 metadata||jsonb_build_object('resource_type','textbook','is_public',is_public,'retrieval_method','textbook_intent'),original_filename
 FROM scored WHERE hits>0 ORDER BY CASE WHEN '比较'=ANY($4::text[]) OR '区别'=ANY($4::text[]) THEN topic_hits ELSE 0 END DESC,hits DESC,is_public DESC,document_id,chunk_index LIMIT $5`, uid, kid, topics, needs, limit, p.EmbeddingSpace)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.Chunk{}
	for rows.Next() {
		var c domain.Chunk
		var raw []byte
		if e = rows.Scan(&c.ID, &c.DocumentID, &c.KnowledgeBaseID, &c.UserID, &c.Content, &c.ChunkIndex, &c.SourceLocation, &raw, &c.DocumentTitle); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(raw, &c.Metadata); e != nil {
			return nil, e
		}
		c.Score = 1
		out = append(out, c)
	}
	return out, rows.Err()
}
