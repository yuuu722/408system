package repository

import (
	"context"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"strings"
)

// SaveKnowledgePoint keeps canonical node identity stable when editors rename a point.
func (p *Postgres) SaveKnowledgePoint(ctx context.Context, uid string, point domain.KnowledgePoint) (domain.KnowledgePoint, error) {
	u, err := p.UserByID(ctx, uid)
	if err != nil {
		return point, err
	}
	if u.Role != "admin" {
		return point, domain.ErrForbidden
	}
	if point.Keywords == nil {
		point.Keywords = []string{}
	}
	if point.Level < 1 {
		point.Level = 1
		if point.ParentID != "" {
			point.Level = 2
		}
	}
	point.Name = strings.TrimSpace(point.Name)
	point.Code = strings.TrimSpace(point.Code)
	if len([]rune(point.Name)) < 2 || len([]rune(point.Name)) > 120 || point.Code == "" || len(point.Code) > 64 || len([]rune(point.Description)) > 8000 {
		return point, &domain.RequestError{Status: 400, Message: "知识点代码、名称或说明不合法"}
	}
	tx, err := p.DB.Begin(ctx)
	if err != nil {
		return point, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(4082101)`); err != nil {
		return point, err
	}
	if point.ParentID != "" {
		var subject string
		err = tx.QueryRow(ctx, `SELECT subject_id::text FROM exam408_knowledge_points WHERE id=$1`, point.ParentID).Scan(&subject)
		if err != nil {
			return point, nf(err)
		}
		if subject != point.SubjectID {
			return point, &domain.RequestError{Status: 400, Message: "上级目录必须属于同一学科"}
		}
	}
	if point.ID == "" {
		point.ID = id()
		_, err = tx.Exec(ctx, `INSERT INTO exam408_knowledge_points(id,subject_id,parent_id,code,name,description,keywords,level,sort_order) VALUES($1,$2,NULLIF($3,'')::uuid,$4,$5,$6,$7,$8,$9)`, point.ID, point.SubjectID, point.ParentID, point.Code, point.Name, point.Description, point.Keywords, point.Level, point.SortOrder)
	} else {
		// Parent/subject/code are immutable on edit: avoid accidental cycles or breaking stable curriculum keys.
		tag, e := tx.Exec(ctx, `UPDATE exam408_knowledge_points SET name=$2,description=$3,keywords=$4,updated_at=now() WHERE id=$1`, point.ID, point.Name, point.Description, point.Keywords)
		err = e
		if e == nil && tag.RowsAffected() == 0 {
			return point, ErrNotFound
		}
	}
	if err != nil {
		return point, err
	}
	// Update mapped canonical entities in place, then fill the shared graph for newly authored points.
	if _, err = tx.Exec(ctx, `UPDATE knowledge_entities e SET name=p.name,description=p.description FROM exam408_entity_mappings m JOIN exam408_knowledge_points p ON p.id=m.knowledge_point_id WHERE m.entity_id=e.id AND p.id=$1 AND e.source='canonical'`, point.ID); err != nil {
		return point, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO knowledge_entities(user_id,knowledge_base_id,name,entity_type,description,source,confidence) SELECT kb.user_id,kb.id,p.name,'knowledge_point',p.description,'canonical',1 FROM knowledge_bases kb CROSS JOIN exam408_knowledge_points p WHERE kb.is_system AND p.id=$1 ON CONFLICT DO NOTHING`, point.ID); err != nil {
		return point, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO exam408_entity_mappings(user_id,knowledge_point_id,entity_id,confidence) SELECT e.user_id,p.id,e.id,1 FROM knowledge_entities e JOIN knowledge_bases kb ON kb.id=e.knowledge_base_id AND kb.is_system JOIN exam408_knowledge_points p ON p.name=e.name WHERE p.id=$1 AND e.entity_type='knowledge_point' ON CONFLICT DO NOTHING`, point.ID); err != nil {
		return point, err
	}
	if _, err = tx.Exec(ctx, `UPDATE knowledge_entities e SET pinned=true FROM knowledge_bases kb WHERE kb.id=e.knowledge_base_id AND kb.is_system AND e.source='canonical'`); err != nil {
		return point, err
	}
	if err = tx.Commit(ctx); err != nil {
		return point, err
	}
	return p.KnowledgePoint(ctx, point.ID)
}
