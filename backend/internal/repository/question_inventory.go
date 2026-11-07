package repository

import (
	"context"
	"github.com/local/exam408-assistant/backend/internal/domain"
)

// PublicQuestionInventory avoids transferring stored question image bytes and
// answer-file excerpts. It is an admin-only inventory, never a student quiz API.
func (p *Postgres) PublicQuestionInventory(ctx context.Context, uid, kid string) ([]domain.Asset, error) {
	u, err := p.UserByID(ctx, uid)
	if err != nil {
		return nil, err
	}
	if u.Role != "admin" {
		return nil, domain.ErrForbidden
	}
	kb, err := p.KnowledgeBase(ctx, uid, kid)
	if err != nil {
		return nil, err
	}
	if !kb.IsPublic {
		return nil, domain.ErrForbidden
	}
	rows, err := p.DB.Query(ctx, `SELECT a.id,a.kind,COALESCE(a.point_id::text,''),a.title,a.status,a.revision,
 (a.payload - 'Images' - 'Evidence') || jsonb_build_object('DocumentID',d.id::text,'Images',
 CASE WHEN jsonb_typeof(a.payload->'Images')='array' AND a.payload->'Images' <> '[]'::jsonb
 THEN '["present"]'::jsonb ELSE '[]'::jsonb END),a.is_public
 FROM learning_assets a JOIN documents d ON d.id=a.document_id
 WHERE d.knowledge_base_id=$1 AND a.is_public=true AND a.kind='question' ORDER BY a.id`, kid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Asset{}
	for rows.Next() {
		a, e := scanAsset(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
