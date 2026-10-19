package repository

import (
	"context"
	"github.com/local/exam408-assistant/backend/internal/domain"
)

// Public reads never imply permission to mutate the shared corpus.
func (p *Postgres) CanMaintainDocument(ctx context.Context, uid, did string) error {
	var allowed bool
	err := p.DB.QueryRow(ctx, `SELECT (NOT kb.is_public AND d.user_id=$2) OR (kb.is_public AND EXISTS(SELECT 1 FROM users WHERE id=$2 AND role='admin')) FROM documents d JOIN knowledge_bases kb ON kb.id=d.knowledge_base_id WHERE d.id=$1`, did, uid).Scan(&allowed)
	if err != nil {
		return nf(err)
	}
	if !allowed {
		return domain.ErrForbidden
	}
	return nil
}
