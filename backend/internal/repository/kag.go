package repository

import (
	"context"
	"encoding/json"
	"github.com/local/exam408-assistant/backend/internal/domain"
)

func (p *Postgres) RetrieveGraphEvidence(ctx context.Context, uid, kid string, terms []string) (domain.GraphRetrieval, error) {
	out := domain.GraphRetrieval{Paths: []domain.KnowledgePath{}, Chunks: []domain.Chunk{}, Terms: []string{}}
	if _, e := p.KnowledgeBase(ctx, uid, kid); e != nil {
		return out, e
	}
	if p.Neo4j == nil {
		return out, graphUnavailable()
	}
	if e := p.SyncKnowledgeGraph(ctx, kid); e != nil {
		return out, e
	}
	paths, e := p.Neo4j.Paths(ctx, kid, terms)
	if e != nil {
		return out, e
	}
	out.Paths = paths
	ids := []string{}
	seen := map[string]bool{}
	names := map[string]bool{}
	for _, path := range paths {
		for _, n := range path.Nodes {
			if n.EvidenceChunkID != "" && !seen[n.EvidenceChunkID] {
				ids = append(ids, n.EvidenceChunkID)
				seen[n.EvidenceChunkID] = true
			}
			if !names[n.Name] && len(out.Terms) < 12 {
				out.Terms = append(out.Terms, n.Name)
				names[n.Name] = true
			}
		}
		for _, r := range path.Edges {
			if r.EvidenceChunkID != "" && !seen[r.EvidenceChunkID] {
				ids = append(ids, r.EvidenceChunkID)
				seen[r.EvidenceChunkID] = true
			}
		}
	}
	if len(ids) > 40 {
		ids = ids[:40]
	}
	if len(ids) == 0 {
		return out, nil
	}
	// Neo4j is navigation, not an authority for source text. Reauthorize and fetch
	// current chunks from the selected KB; stale/deleted/other-KB IDs cannot enter.
	rows, e := p.DB.Query(ctx, `SELECT c.id,c.document_id,c.knowledge_base_id,c.user_id,c.content,c.chunk_index,c.source_location,c.metadata,d.original_filename,c.created_at FROM document_chunks c JOIN documents d ON d.id=c.document_id JOIN knowledge_bases kb ON kb.id=c.knowledge_base_id WHERE c.knowledge_base_id=$2 AND (kb.is_public OR kb.user_id=$1) AND c.id::text=ANY($3::text[]) AND (d.status='ready' OR d.active_revision_id IS NOT NULL) AND NOT EXISTS(SELECT 1 FROM document_catalog cat WHERE cat.document_id=d.id AND cat.purpose='questions') ORDER BY c.chunk_index LIMIT 40`, uid, kid, ids)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var c domain.Chunk
		var meta []byte
		if e = rows.Scan(&c.ID, &c.DocumentID, &c.KnowledgeBaseID, &c.UserID, &c.Content, &c.ChunkIndex, &c.SourceLocation, &meta, &c.DocumentTitle, &c.CreatedAt); e != nil {
			return out, e
		}
		if e = json.Unmarshal(meta, &c.Metadata); e != nil {
			return out, e
		}
		c.Score = 1
		out.Chunks = append(out.Chunks, c)
	}
	return out, rows.Err()
}
