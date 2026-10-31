package repository

import (
	"context"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"time"
)

func (p *Postgres) SyncKnowledgeGraph(ctx context.Context, kid string) error {
	if p.Neo4j == nil {
		return nil
	}
	tx, e := p.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	// API and worker share a journal; only one may publish a scope at a time.
	var acquired bool
	if e = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,40821))`, kid).Scan(&acquired); e != nil {
		return e
	}
	if !acquired {
		return nil
	}
	// Do not hold the journal row lock during Neo4j network I/O: document
	// writers must remain responsive. A changed revision stays dirty for replay.
	var version, synced int64
	var deleted bool
	if e = tx.QueryRow(ctx, `SELECT revision,synced_revision,deleted FROM graph_sync_state WHERE knowledge_base_id=$1`, kid).Scan(&version, &synced, &deleted); e != nil {
		return nf(e)
	}
	if version == synced {
		stored, check := p.Neo4j.Version(ctx, kid)
		if check != nil {
			return check
		}
		if stored == version {
			return tx.Commit(ctx)
		}
	}
	g := domain.KnowledgeGraph{KnowledgeBaseID: kid}
	if !deleted {
		var owner string
		if e = tx.QueryRow(ctx, `SELECT user_id FROM knowledge_bases WHERE id=$1`, kid).Scan(&owner); e != nil {
			return nf(e)
		}
		g, e = p.graphProvenance(ctx, owner, kid)
		if e != nil {
			return e
		}
	}
	var currentVersion int64
	if e = tx.QueryRow(ctx, `SELECT revision FROM graph_sync_state WHERE knowledge_base_id=$1`, kid).Scan(&currentVersion); e != nil {
		return e
	}
	if currentVersion != version {
		return nil
	}
	if e = p.Neo4j.Replace(ctx, kid, g, version); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE graph_sync_state SET synced_revision=$2,updated_at=now() WHERE knowledge_base_id=$1`, kid, version); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (p *Postgres) SyncPendingGraphs(ctx context.Context) error {
	if p.Neo4j == nil {
		return nil
	}
	rows, e := p.DB.Query(ctx, `SELECT knowledge_base_id FROM graph_sync_state WHERE revision<>synced_revision ORDER BY updated_at LIMIT 8`)
	if e != nil {
		return e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, id := range ids {
		if e = p.SyncKnowledgeGraph(ctx, id); e != nil {
			return e
		}
	}
	return nil
}
func (p *Postgres) RunGraphSync(ctx context.Context, report func(error)) {
	if p.Neo4j == nil {
		return
	}
	for ctx.Err() == nil {
		attempt, cancel := context.WithTimeout(ctx, 120*time.Second)
		e := p.Neo4j.Initialize(attempt)
		if e == nil {
			e = p.SyncPendingGraphs(attempt)
		}
		cancel()
		if e != nil && report != nil {
			report(e)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}
func (p *Postgres) KnowledgeGraph(ctx context.Context, uid, kid string) (domain.KnowledgeGraph, error) {
	kb, e := p.KnowledgeBase(ctx, uid, kid)
	if e != nil {
		return domain.KnowledgeGraph{}, e
	}
	if p.Neo4j == nil {
		g, e := p.graphProvenance(ctx, uid, kid)
		g.Storage = "postgres"
		return g, e
	}
	graphCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var g domain.KnowledgeGraph
	g, e = p.Neo4j.Read(graphCtx, kid)
	if e == nil {
		var pending bool
		e = p.DB.QueryRow(ctx, `SELECT revision<>synced_revision FROM graph_sync_state WHERE knowledge_base_id=$1`, kid).Scan(&pending)
		if e == nil && pending {
			g.Warning = "图谱正在后台更新，当前为上次完成的快照。"
			if len(g.Nodes) == 0 {
				e = graphUnavailable()
			}
		}
	}
	if e != nil {
		if ctx.Err() != nil {
			return domain.KnowledgeGraph{}, ctx.Err()
		}
		g, sourceErr := p.graphProvenance(ctx, uid, kid)
		if sourceErr != nil {
			return g, sourceErr
		}
		g.Storage = "postgres-recovery"
		g.Warning = "Neo4j图谱服务暂不可用，当前显示已保存的图谱与来源；同步任务会自动重试。此状态不代表已完成Neo4j查询。"
		return g, nil
	}
	// Empty storage is replayed asynchronously; the page never replaces snapshots.
	if len(g.Nodes) == 0 {
		saved, err := p.graphProvenance(ctx, uid, kid)
		if err != nil {
			return saved, err
		}
		if len(saved.Nodes) > 0 {
			_, err = p.DB.Exec(ctx, `UPDATE graph_sync_state SET revision=revision+1,updated_at=now() WHERE knowledge_base_id=$1 AND revision=synced_revision`, kid)
			if err != nil {
				return saved, err
			}
			saved.Storage = "postgres-recovery"
			saved.Warning = "图谱快照尚未就绪，当前显示已保存的图谱；后台正在重建Neo4j快照。"
			return saved, nil
		}
	}
	g.IsPublic = kb.IsPublic
	g.Storage = "neo4j"
	return g, nil
}
