package repository

import (
	"context"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"strings"
	"time"
)

type ManagedUser struct {
	ID, Username, Email, Role        string
	PersonalKBLimit, PersonalKBCount int
	StorageBytes                     int64
	Disabled                         bool
	CreatedAt                        time.Time
}
type UserPage struct {
	Users                []ManagedUser
	Total, Offset, Limit int
}

func (p *Postgres) ManageUsers(ctx context.Context, actor, query string, offset int) (UserPage, error) {
	out := UserPage{Users: []ManagedUser{}, Offset: offset, Limit: 50}
	u, e := p.UserByID(ctx, actor)
	if e != nil {
		return out, e
	}
	if u.Role != "admin" || u.Disabled {
		return out, domain.ErrForbidden
	}
	if offset < 0 {
		offset = 0
	}
	out.Offset = offset
	pattern := "%" + strings.TrimSpace(query) + "%"
	if e = p.DB.QueryRow(ctx, `SELECT count(*) FROM users WHERE id<>'40800000-0000-4000-8000-000000000001' AND (username ILIKE $1 OR email ILIKE $1)`, pattern).Scan(&out.Total); e != nil {
		return out, e
	}
	rows, e := p.DB.Query(ctx, `SELECT id,username,email,role,disabled,created_at,personal_kb_limit,(SELECT count(*) FROM knowledge_bases k WHERE k.user_id=users.id AND NOT k.is_public),(SELECT COALESCE(sum(d.size),0) FROM documents d JOIN knowledge_bases k ON k.id=d.knowledge_base_id WHERE k.user_id=users.id AND NOT k.is_public) FROM users WHERE id<>'40800000-0000-4000-8000-000000000001' AND (username ILIKE $1 OR email ILIKE $1) ORDER BY created_at DESC,id LIMIT 50 OFFSET $2`, pattern, offset)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var u ManagedUser
		if e = rows.Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.Disabled, &u.CreatedAt, &u.PersonalKBLimit, &u.PersonalKBCount, &u.StorageBytes); e != nil {
			return out, e
		}
		out.Users = append(out.Users, u)
	}
	return out, rows.Err()
}
func (p *Postgres) UpdateManagedUser(ctx context.Context, actor, target, role string, disabled bool) error {
	if role != "admin" && role != "student" {
		return &domain.RequestError{Status: 400, Message: "角色必须为管理员或普通用户"}
	}
	if target == "40800000-0000-4000-8000-000000000001" {
		return domain.ErrForbidden
	}
	if target == actor && (disabled || role != "admin") {
		return &domain.RequestError{Status: 409, Message: "不能停用自己或取消自己的管理员身份"}
	}
	tx, e := p.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(4082099)`); e != nil {
		return e
	}
	var allowed bool
	if e = tx.QueryRow(ctx, `SELECT role='admin' AND NOT disabled FROM users WHERE id=$1`, actor).Scan(&allowed); e != nil {
		return nf(e)
	}
	if !allowed {
		return domain.ErrForbidden
	}
	var last bool
	if e = tx.QueryRow(ctx, `SELECT role='admin' AND NOT disabled AND (SELECT count(*) FROM users WHERE role='admin' AND NOT disabled AND id<>'40800000-0000-4000-8000-000000000001')<=1 FROM users WHERE id=$1`, target).Scan(&last); e != nil {
		return nf(e)
	}
	if last && (role != "admin" || disabled) {
		return &domain.RequestError{Status: 409, Message: "至少保留一位可用管理员"}
	}
	tag, e := tx.Exec(ctx, `UPDATE users SET role=$2,disabled=$3,updated_at=now() WHERE id=$1`, target, role, disabled)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}
func (p *Postgres) CreatePublicKnowledgeBase(ctx context.Context, actor, name, description string) (domain.KnowledgeBase, error) {
	var kb domain.KnowledgeBase
	u, e := p.UserByID(ctx, actor)
	if e != nil {
		return kb, e
	}
	if u.Role != "admin" || u.Disabled {
		return kb, domain.ErrForbidden
	}
	e = p.DB.QueryRow(ctx, `INSERT INTO knowledge_bases(user_id,name,description,is_public) VALUES($1,$2,$3,true) RETURNING id,user_id,name,description,created_at,updated_at,is_public`, actor, name, description).Scan(&kb.ID, &kb.UserID, &kb.Name, &kb.Description, &kb.CreatedAt, &kb.UpdatedAt, &kb.IsPublic)
	return kb, e
}

func (p *Postgres) UpdatePublicKnowledgeBase(ctx context.Context, actor, kid, name, description string) (domain.KnowledgeBase, error) {
	var kb domain.KnowledgeBase
	u, e := p.UserByID(ctx, actor)
	if e != nil {
		return kb, e
	}
	if u.Role != "admin" || u.Disabled {
		return kb, domain.ErrForbidden
	}
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 120 {
		return kb, &domain.RequestError{Status: 400, Message: "知识库名称须为1至120字"}
	}
	e = p.DB.QueryRow(ctx, `UPDATE knowledge_bases SET name=$2,description=$3,updated_at=now() WHERE id=$1 AND is_public RETURNING id,user_id,name,description,created_at,updated_at,is_public`, kid, name, description).Scan(&kb.ID, &kb.UserID, &kb.Name, &kb.Description, &kb.CreatedAt, &kb.UpdatedAt, &kb.IsPublic)
	return kb, nf(e)
}

func (p *Postgres) RetryFailedPublicDocuments(ctx context.Context, actor, kid string) (int64, error) {
	u, e := p.UserByID(ctx, actor)
	if e != nil {
		return 0, e
	}
	if u.Role != "admin" || u.Disabled {
		return 0, domain.ErrForbidden
	}
	tag, e := p.DB.Exec(ctx, `WITH targets AS (
 UPDATE documents d SET status='processing',processing_stage='queued',progress=0,error_message='',updated_at=now()
 FROM knowledge_bases kb WHERE kb.id=d.knowledge_base_id AND kb.is_public AND d.status='failed' AND ($1='' OR kb.id::text=$1) RETURNING d.id
 ) INSERT INTO document_jobs(document_id) SELECT id FROM targets ON CONFLICT(document_id) DO UPDATE SET state='queued',execution=document_jobs.execution+1,attempts=0,lease_until=NULL,last_error='',available_at=now(),updated_at=now()`, kid)
	if e != nil {
		return 0, e
	}
	return tag.RowsAffected(), nil
}

// Quotas govern future creation; lowering one never removes private data.
func (p *Postgres) SetPersonalQuota(ctx context.Context, actor, target string, limit int) error {
	if limit < 0 || limit > 100 {
		return catalogInputError("个人知识库配额须为0至100")
	}
	u, e := p.UserByID(ctx, actor)
	if e != nil {
		return e
	}
	if u.Role != "admin" || u.Disabled {
		return domain.ErrForbidden
	}
	tag, e := p.DB.Exec(ctx, `UPDATE users SET personal_kb_limit=$2,updated_at=now() WHERE id=$1 AND id<>'40800000-0000-4000-8000-000000000001'`, target, limit)
	if e == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return e
}
