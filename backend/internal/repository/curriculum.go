package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/local/exam408-assistant/backend/internal/domain"
)

// EnsurePublicDocuments is retained for compatibility. Public libraries start
// empty; only explicit administrator publication may materialize a resource.
func (p *Postgres) EnsurePublicDocuments(ctx context.Context, storageRoot string) error {
	return nil // Public resources are uploaded or explicitly published by administrators.
}

func (p *Postgres) SyncPublicMaterial(ctx context.Context, storageRoot, assetID string) error {
	return p.ensurePublicDocuments(ctx, storageRoot, assetID)
}

func (p *Postgres) ensurePublicDocuments(ctx context.Context, storageRoot, assetID string) error {
	tx, err := p.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(4082100)`); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT a.id,a.user_id,kb.id,a.point_id,a.title,a.payload,COALESCE(a.document_id::text,''),a.revision FROM learning_assets a CROSS JOIN knowledge_bases kb WHERE kb.is_system AND a.is_public AND a.kind='material' AND COALESCE(a.payload->>'Stem','')<>'' AND a.id::text=$1`, assetID)
	if err != nil {
		return err
	}
	type lesson struct {
		id, uid, kid, point, title, did string
		revision                        int
		payload                         domain.AssetPayload
	}
	lessons := []lesson{}
	for rows.Next() {
		var l lesson
		var raw []byte
		if err = rows.Scan(&l.id, &l.uid, &l.kid, &l.point, &l.title, &raw, &l.did, &l.revision); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal(raw, &l.payload); err != nil {
			rows.Close()
			return err
		}
		lessons = append(lessons, l)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, l := range lessons {
		did := l.did
		if did == "" {
			did = id()
		} else {
			var running bool
			if err = tx.QueryRow(ctx, `SELECT state='running' FROM document_jobs WHERE document_id=$1 FOR UPDATE`, did).Scan(&running); err != nil {
				return err
			}
			if running {
				return &domain.RequestError{Status: 409, Message: "资料正在处理，请完成后再编辑"}
			}
		}
		name := fmt.Sprintf("%s-r%d.md", l.id, l.revision)
		dir := filepath.Join(storageRoot, l.uid, l.kid)
		if err = os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		content := fmt.Sprintf("# %s\n\n%s\n\n%s\n\n来源：%s\n", l.title, l.payload.Stem, l.payload.Explanation, l.payload.Source)
		path := filepath.Join(dir, name)
		// A transaction retry may encounter a previously materialized identical file.
		if old, e := os.ReadFile(path); e == nil {
			if string(old) != content {
				return fmt.Errorf("curriculum file already exists with different content: %s", name)
			}
		} else if os.IsNotExist(e) {
			if err = os.WriteFile(path, []byte(content), 0600); err != nil {
				return err
			}
		} else {
			return e
		}
		if _, err = tx.Exec(ctx, `INSERT INTO documents(id,user_id,knowledge_base_id,filename,original_filename,mime_type,size,storage_path,resource_type,status) VALUES($1,$2,$3,$4,$5,'text/markdown',$6,$7,'notes','processing') ON CONFLICT(id) DO UPDATE SET storage_path=EXCLUDED.storage_path,size=EXCLUDED.size,original_filename=EXCLUDED.original_filename,status='processing',processing_stage='queued',progress=0,error_message=''`, did, l.uid, l.kid, name, l.title+".md", len(content), path); err != nil {
			return err
		}
		l.payload.DocumentID = did
		raw, _ := json.Marshal(l.payload)
		if _, err = tx.Exec(ctx, `UPDATE learning_assets SET document_id=$2,payload=$3 WHERE id=$1`, l.id, did, raw); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO exam408_resource_mappings(user_id,knowledge_point_id,document_id,confidence,source) VALUES($1,$2,$3,1,'manual') ON CONFLICT DO NOTHING`, l.uid, l.point, did); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO document_jobs(document_id) VALUES($1) ON CONFLICT(document_id) DO UPDATE SET state='queued',attempts=0,last_error='',available_at=now(),lease_until=NULL,execution=document_jobs.execution+1`, did); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
