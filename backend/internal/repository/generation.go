package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/local/exam408-assistant/backend/internal/domain"
)

func (p *Postgres) ReserveGeneration(ctx context.Context, uid, rid, hash string) ([]domain.Asset, bool, error) {
	if _, e := uuid.Parse(rid); e != nil {
		return nil, false, &domain.RequestError{Status: 400, Message: "生成请求标识无效（Invalid request ID）"}
	}
	tx, e := p.DB.Begin(ctx)
	if e != nil {
		return nil, false, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,411))`, uid); e != nil {
		return nil, false, e
	}
	var owner, state, previous string
	var raw []byte
	e = tx.QueryRow(ctx, `SELECT user_id,state,request_hash,assets FROM question_generations WHERE id=$1`, rid).Scan(&owner, &state, &previous, &raw)
	if e == nil {
		if owner != uid || previous != hash {
			return nil, false, &domain.RequestError{Status: 409, Message: "生成请求冲突（Conflicting request）"}
		}
		if state != "complete" {
			return nil, false, &domain.RequestError{Status: 409, Message: "该请求正在处理或已中断，请新建生成请求（Request already used）"}
		}
		var out []domain.Asset
		e = json.Unmarshal(raw, &out)
		return out, true, e
	}
	if e != pgx.ErrNoRows {
		return nil, false, e
	}
	var count int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM question_generations WHERE user_id=$1 AND created_at>=now()-interval '24 hours'`, uid).Scan(&count); e != nil {
		return nil, false, e
	}
	if count >= 5 {
		return nil, false, &domain.RequestError{Status: 429, Message: "24小时生成次数已达5次，请稍后再试（Generation limit reached）"}
	}
	_, e = tx.Exec(ctx, `INSERT INTO question_generations(id,user_id,request_hash) VALUES($1,$2,$3)`, rid, uid, hash)
	if e != nil {
		return nil, false, e
	}
	return nil, false, tx.Commit(ctx)
}
func (p *Postgres) FailGeneration(ctx context.Context, uid, rid string) error {
	_, e := p.DB.Exec(ctx, `UPDATE question_generations SET state='failed' WHERE id=$1 AND user_id=$2 AND state='running'`, rid, uid)
	return e
}
func (p *Postgres) CompleteGeneration(ctx context.Context, uid, rid string, assets []domain.Asset) ([]domain.Asset, error) {
	tx, e := p.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,411))`, uid); e != nil {
		return nil, e
	}
	var state string
	e = tx.QueryRow(ctx, `SELECT state FROM question_generations WHERE id=$1 AND user_id=$2 FOR UPDATE`, rid, uid).Scan(&state)
	if e != nil {
		return nil, nf(e)
	}
	if state != "running" {
		return nil, fmt.Errorf("生成请求已结束（Generation finished）")
	}
	for i := range assets {
		a := &assets[i]
		if a.Payload.ContentHash != "" {
			existing, e := scanAsset(tx.QueryRow(ctx, `SELECT `+assetCols+` FROM learning_assets WHERE user_id=$1 AND payload->>'ContentHash'=$2 LIMIT 1`, uid, a.Payload.ContentHash))
			if e == nil {
				if existing.PointID != a.PointID {
					return nil, &domain.RequestError{Status: 409, Message: "同内容题目已关联其他知识点，请调整生成目标（Conflicting question）"}
				}
				*a = existing
				continue
			}
			if e != ErrNotFound {
				return nil, e
			}
		}
		a.ID = uuid.NewString()
		a.Revision = 1
		raw, e := json.Marshal(a.Payload)
		if e != nil {
			return nil, e
		}
		_, e = tx.Exec(ctx, `INSERT INTO learning_assets(id,user_id,kind,point_id,title,status,payload) VALUES($1,$2,'question',$3,$4,'ready',$5)`, a.ID, uid, a.PointID, a.Title, raw)
		if e != nil {
			return nil, e
		}
	}
	raw, e := json.Marshal(assets)
	if e != nil {
		return nil, e
	}
	_, e = tx.Exec(ctx, `UPDATE question_generations SET state='complete',assets=$3 WHERE id=$1 AND user_id=$2`, rid, uid, raw)
	if e != nil {
		return nil, e
	}
	return assets, tx.Commit(ctx)
}
