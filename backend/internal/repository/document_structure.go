package repository

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/local/exam408-assistant/backend/internal/domain"
)

func (p *Postgres) SaveDocumentStructure(ctx context.Context, uid, did string, parsed domain.ParsedDocument, assets []domain.Asset) error {
	if _, e := p.Document(ctx, uid, did); e != nil {
		return e
	}
	tx, e := p.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if e = saveDocumentStructureTx(ctx, tx, uid, did, parsed, assets); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func saveDocumentStructureTx(ctx context.Context, tx pgx.Tx, uid, did string, parsed domain.ParsedDocument, assets []domain.Asset) error {
	raw, _ := json.Marshal(parsed)
	if _, e := tx.Exec(ctx, `INSERT INTO document_parses(document_id,payload) VALUES($1,$2) ON CONFLICT(document_id) DO UPDATE SET payload=$2,updated_at=now()`, did, raw); e != nil {
		return e
	}
	var purpose string
	if e := tx.QueryRow(ctx, `SELECT COALESCE((SELECT purpose FROM document_catalog WHERE document_id=$1),'knowledge')`, did).Scan(&purpose); e != nil {
		return e
	}
	if purpose == "knowledge" {
		return nil
	}
	ids := []string{}
	for _, a := range assets {
		a.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(did+":"+a.Payload.ExtractionKey)).String()
		ids = append(ids, a.ID)
		a.Payload.ReviewStatus = "pending"
		domain.PrepareAsset(&a)
		payload, _ := json.Marshal(a.Payload)
		if _, e := tx.Exec(ctx, `INSERT INTO learning_assets(id,user_id,kind,point_id,title,status,payload,document_id,is_public) VALUES($1,$2,'question',NULLIF($3,'')::uuid,$4,$5,$6,$7,(SELECT kb.is_public FROM documents d JOIN knowledge_bases kb ON kb.id=d.knowledge_base_id WHERE d.id=$7)) ON CONFLICT(id) DO UPDATE SET point_id=EXCLUDED.point_id,title=EXCLUDED.title,status=EXCLUDED.status,payload=EXCLUDED.payload,revision=learning_assets.revision+1,updated_at=now() WHERE COALESCE(learning_assets.payload->>'ReviewStatus','')<>'published'`, a.ID, uid, a.PointID, a.Title, a.Status, payload, did); e != nil {
			return e
		}
	}
	if _, e := tx.Exec(ctx, `DELETE FROM learning_assets WHERE user_id=$1 AND payload->>'DocumentID'=$2 AND COALESCE(payload->>'ExtractionKey','')<>'' AND NOT(id::text=ANY($3::text[]))`, uid, did, ids); e != nil {
		return e
	}
	return nil
}
func (p *Postgres) DocumentStructure(ctx context.Context, uid, did string) (domain.ParsedDocument, error) {
	var out domain.ParsedDocument
	if _, e := p.Document(ctx, uid, did); e != nil {
		return out, e
	}
	var raw []byte
	e := p.DB.QueryRow(ctx, `SELECT payload FROM document_parses WHERE document_id=$1`, did).Scan(&raw)
	if e != nil {
		return out, nf(e)
	}
	e = json.Unmarshal(raw, &out)
	return out, e
}

func (p *Postgres) RevisionStructure(ctx context.Context, uid, did, rid string) (domain.ParsedDocument, error) {
	if rid == "" {
		return p.DocumentStructure(ctx, uid, did)
	}
	if _, e := p.Document(ctx, uid, did); e != nil {
		return domain.ParsedDocument{}, e
	}
	if rid == "latest-attempt" {
		var state string
		e := p.DB.QueryRow(ctx, `SELECT id,state FROM document_revisions WHERE document_id=$1 ORDER BY created_at DESC LIMIT 1`, did).Scan(&rid, &state)
		if e != nil {
			return domain.ParsedDocument{}, nf(e)
		}
		pages, e := p.RevisionPages(ctx, uid, did, rid)
		return domain.ParsedDocument{RevisionID: rid, State: state, Pages: pages}, e
	}
	var valid bool
	e := p.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM document_revisions WHERE id=$1 AND document_id=$2 AND state='complete')`, rid, did).Scan(&valid)
	if e != nil {
		return domain.ParsedDocument{}, e
	}
	if !valid {
		return domain.ParsedDocument{}, ErrNotFound
	}
	pages, e := p.RevisionPages(ctx, uid, did, rid)
	return domain.ParsedDocument{RevisionID: rid, Pages: pages}, e
}
