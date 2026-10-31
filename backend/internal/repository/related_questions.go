package repository

import (
	"context"
	"github.com/local/exam408-assistant/backend/internal/domain"
)

// Only reviewed, usable questions from the selected scope. Retrieval never
// publishes extraction candidates or exposes another member's exercises.
func (p *Postgres) RelatedQuestions(ctx context.Context, uid, kid string, pointIDs []string) ([]domain.Asset, error) {
	chosen, e := p.KnowledgeBase(ctx, uid, kid)
	if e != nil {
		return nil, e
	}
	out := []domain.Asset{}
	if len(pointIDs) == 0 {
		return out, nil
	}
	rows, e := p.DB.Query(ctx, `SELECT a.id,a.kind,COALESCE(a.point_id::text,''),a.title,a.status,a.revision,a.payload,a.is_public
 FROM learning_assets a JOIN documents d ON d.id=a.document_id JOIN knowledge_bases kb ON kb.id=d.knowledge_base_id
 WHERE a.kind='question' AND a.status='ready' AND a.payload->>'ReviewStatus'='published'
 AND a.point_id::text=ANY($3::text[]) AND (d.status='ready' OR d.active_revision_id IS NOT NULL)
 AND ((kb.id=$2 AND (kb.is_public OR kb.user_id=$1)) OR ($4 AND kb.is_public))
 AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(a.payload->'Evidence')='array' THEN a.payload->'Evidence' ELSE '[]'::jsonb END) ev
 WHERE NOT EXISTS(SELECT 1 FROM documents sd JOIN knowledge_bases sk ON sk.id=sd.knowledge_base_id
 WHERE sd.id::text=ev->>'DocumentID' AND (sk.is_public OR (sk.id=$2 AND sk.user_id=$1))
 AND (sd.status='ready' OR sd.active_revision_id IS NOT NULL)
 AND (COALESCE(ev->>'RevisionID','')='' OR sd.active_revision_id::text=ev->>'RevisionID')))
 ORDER BY (d.resource_type='past_exam') DESC,a.updated_at DESC LIMIT 3`, uid, kid, pointIDs, !chosen.IsPublic)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		a, e := scanAsset(rows)
		if e != nil {
			return out, e
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
