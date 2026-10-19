package repository

import (
	"context"
	"github.com/jackc/pgx/v5"
)

func (p *Postgres) rememberGraphEvidence(ctx context.Context, tx pgx.Tx, kid string) error {
	_, err := tx.Exec(ctx, `INSERT INTO knowledge_entity_evidence(entity_id,chunk_id) SELECT e.id,e.evidence_chunk_id FROM knowledge_entities e WHERE e.knowledge_base_id=$1 AND e.evidence_chunk_id IS NOT NULL ON CONFLICT DO NOTHING`, kid)
	return err
}
func (p *Postgres) pruneGraph(ctx context.Context, tx pgx.Tx, kid string) error {
	_, err := tx.Exec(ctx, `DELETE FROM knowledge_entities e WHERE e.knowledge_base_id=$1 AND NOT e.pinned AND NOT EXISTS(SELECT 1 FROM knowledge_entity_evidence evidence WHERE evidence.entity_id=e.id)`, kid)
	return err
}
