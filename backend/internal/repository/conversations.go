package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"strings"
)

func (p *Postgres) ListConversations(ctx context.Context, uid string) ([]domain.Conversation, error) {
	out := []domain.Conversation{}
	rows, err := p.DB.Query(ctx, `SELECT id,knowledge_base_id,title,updated_at FROM conversations WHERE user_id=$1 ORDER BY updated_at DESC LIMIT 100`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c domain.Conversation
		if err = rows.Scan(&c.ID, &c.KnowledgeBaseID, &c.Title, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (p *Postgres) ConversationMessages(ctx context.Context, uid, cid, kid string) ([]domain.Message, error) {
	var exists bool
	err := p.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM conversations WHERE id=$1 AND user_id=$2 AND ($3='' OR knowledge_base_id::text=$3))`, cid, uid, kid).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := p.DB.Query(ctx, `SELECT id,role,content,citations,created_at FROM (SELECT * FROM messages WHERE conversation_id=$1 AND user_id=$2 ORDER BY sequence DESC LIMIT 200) m ORDER BY sequence`, cid, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Message{}
	for rows.Next() {
		var m domain.Message
		var raw []byte
		if err = rows.Scan(&m.ID, &m.Role, &m.Content, &raw, &m.CreatedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &m.Citations); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (p *Postgres) RenameConversation(ctx context.Context, uid, cid, title string) error {
	title = strings.TrimSpace(title)
	if title == "" || len([]rune(title)) > 100 {
		return fmt.Errorf("invalid title")
	}
	tag, err := p.DB.Exec(ctx, `UPDATE conversations SET title=$3,updated_at=now() WHERE id=$1 AND user_id=$2`, cid, uid, title)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
func (p *Postgres) DeleteConversation(ctx context.Context, uid, cid string) error {
	tag, err := p.DB.Exec(ctx, `DELETE FROM conversations WHERE id=$1 AND user_id=$2`, cid, uid)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
func (p *Postgres) LockConversation(ctx context.Context, uid, cid string) (func(), error) {
	c, err := p.DB.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	var ok bool
	err = c.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,408))`, uid+cid).Scan(&ok)
	if err != nil || !ok {
		c.Release()
		return nil, fmt.Errorf("conversation is already processing")
	}
	return func() {
		_, _ = c.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtextextended($1,408))`, uid+cid)
		c.Release()
	}, nil
}
func (p *Postgres) SaveTurn(ctx context.Context, uid, cid, question, answer string, citations []domain.Citation) error {
	tx, err := p.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE conversations SET updated_at=now() WHERE id=$1 AND user_id=$2`, cid, uid)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	raw, _ := json.Marshal(citations)
	_, err = tx.Exec(ctx, `INSERT INTO messages(conversation_id,user_id,role,content,citations) VALUES($1,$2,'user',$3,'[]'),($1,$2,'assistant',$4,$5)`, cid, uid, question, answer, raw)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
