package repository

import (
	"context"
	"encoding/json"
	"github.com/local/exam408-assistant/backend/internal/domain"
)

// Adjacent original textbook chunks recover explanations split across chunks.
// The anchor is retrieval context only; citations always point to actual text.
func (p *Postgres) ExpandTextbookContext(ctx context.Context, uid, kid string, anchors []domain.Chunk) ([]domain.Chunk, error) {
	chosen, e := p.KnowledgeBase(ctx, uid, kid)
	if e != nil {
		return nil, e
	}
	ids := []string{}
	scores := map[string]float64{}
	for _, c := range anchors {
		if len(ids) >= 12 {
			break
		}
		ids = append(ids, c.ID)
		scores[c.ID] = c.Score
	}
	if len(ids) == 0 {
		return nil, nil
	}
	rows, e := p.DB.Query(ctx, `SELECT DISTINCT ON(c.id) c.id,c.document_id,c.knowledge_base_id,c.user_id,c.content,c.chunk_index,c.source_location,
 c.metadata || jsonb_build_object('resource_type','textbook','is_public',kb.is_public,'retrieval_anchor',left(a.content,600)),d.original_filename,a.id::text
 FROM document_chunks a JOIN document_chunks c ON c.document_id=a.document_id AND c.id<>a.id AND abs(c.chunk_index-a.chunk_index)<=2
 JOIN documents d ON d.id=c.document_id JOIN knowledge_bases kb ON kb.id=d.knowledge_base_id
 LEFT JOIN document_catalog cat ON cat.document_id=d.id
 WHERE a.id::text=ANY($3::text[]) AND (kb.id=$2 OR ($4 AND kb.is_public)) AND (kb.is_public OR kb.user_id=$1)
 AND COALESCE(cat.resource_type,d.resource_type)='textbook' AND COALESCE(cat.purpose,'knowledge')<>'questions'
 AND (d.status='ready' OR d.active_revision_id IS NOT NULL)
 AND c.metadata->>'embedding_space'=a.metadata->>'embedding_space'
 AND (COALESCE(c.metadata->>'heading_path','')='' OR COALESCE(a.metadata->>'heading_path','')='' OR c.metadata->>'heading_path'=a.metadata->>'heading_path')
 ORDER BY c.id,abs(c.chunk_index-a.chunk_index) LIMIT 48`, uid, kid, ids, !chosen.IsPublic)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.Chunk{}
	for rows.Next() {
		var c domain.Chunk
		var raw []byte
		var anchor string
		if e = rows.Scan(&c.ID, &c.DocumentID, &c.KnowledgeBaseID, &c.UserID, &c.Content, &c.ChunkIndex, &c.SourceLocation, &raw, &c.DocumentTitle, &anchor); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(raw, &c.Metadata); e != nil {
			return nil, e
		}
		c.Score = scores[anchor] * .98
		out = append(out, c)
	}
	return out, rows.Err()
}
