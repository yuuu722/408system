package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/local/exam408-assistant/backend/internal/domain"
)

// BootstrapAdmin promotes an existing account exactly once and creates its shared
// public knowledge base in the same transaction. It never creates credentials.
func (p *Postgres) BootstrapAdmin(ctx context.Context, email string) (domain.User, domain.KnowledgeBase, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return domain.User{}, domain.KnowledgeBase{}, fmt.Errorf("email is required")
	}
	tx, err := p.DB.Begin(ctx)
	if err != nil {
		return domain.User{}, domain.KnowledgeBase{}, err
	}
	defer tx.Rollback(ctx)

	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(4082099)`); err != nil {
		return domain.User{}, domain.KnowledgeBase{}, err
	}
	var hasAdmin bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE role='admin' AND id<>'40800000-0000-4000-8000-000000000001')`).Scan(&hasAdmin); err != nil {
		return domain.User{}, domain.KnowledgeBase{}, err
	}
	if hasAdmin {
		return domain.User{}, domain.KnowledgeBase{}, errors.New("an administrator already exists; bootstrap is disabled")
	}

	var u domain.User
	err = tx.QueryRow(ctx, `UPDATE users SET role='admin',updated_at=now() WHERE lower(email)=lower($1) AND disabled=false RETURNING id,username,email,password_hash,created_at,role`, email).
		Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt, &u.Role)
	if err != nil {
		return domain.User{}, domain.KnowledgeBase{}, nf(err)
	}

	var kb domain.KnowledgeBase
	err = tx.QueryRow(ctx, `SELECT id,user_id,name,description,created_at,updated_at,is_public FROM knowledge_bases WHERE is_system=true`).
		Scan(&kb.ID, &kb.UserID, &kb.Name, &kb.Description, &kb.CreatedAt, &kb.UpdatedAt, &kb.IsPublic)
	if err != nil {
		return domain.User{}, domain.KnowledgeBase{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.User{}, domain.KnowledgeBase{}, err
	}
	return u, kb, nil
}
