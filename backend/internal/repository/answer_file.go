package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"sort"
	"strings"
)

// Batch updates are optimistic and never overwrite already published questions.
func (p *Postgres) ApplyAnswerFile(ctx context.Context, uid string, qdoc, adoc domain.Document, updates []domain.Asset, qhash, ahash string) (int, error) {
	if e := p.CanMaintainDocument(ctx, uid, qdoc.ID); e != nil {
		return 0, e
	}
	if _, e := p.Document(ctx, uid, adoc.ID); e != nil {
		return 0, e
	}
	tx, e := p.DB.Begin(ctx)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback(ctx)
	docs := []domain.Document{qdoc, adoc}
	sort.Slice(docs, func(i, j int) bool { return docs[i].ID < docs[j].ID })
	for _, d := range docs {
		var state, revision string
		if e = tx.QueryRow(ctx, `SELECT status,COALESCE(active_revision_id::text,'') FROM documents WHERE id=$1 FOR SHARE`, d.ID).Scan(&state, &revision); e != nil {
			return 0, e
		}
		if state != "ready" || revision != d.ActiveRevisionID {
			return 0, &domain.RequestError{Status: 409, Message: "文件版本已变化或正在处理，请重新预览配对"}
		}
	}
	for _, check := range []struct{ id, hash string }{{qdoc.ID, qhash}, {adoc.ID, ahash}} {
		parsed, err := questionStructureTx(ctx, tx, check.id)
		if err != nil {
			return 0, err
		}
		if ParsedFingerprint(parsed) != check.hash {
			return 0, &domain.RequestError{Status: 409, Message: "解析结果已变化，请重新预览"}
		}
	}
	sort.Slice(updates, func(i, j int) bool { return updates[i].ID < updates[j].ID })
	count := 0
	for _, a := range updates {
		raw, e := json.Marshal(a.Payload)
		if e != nil {
			return 0, e
		}
		tag, e := tx.Exec(ctx, `UPDATE learning_assets SET payload=$4,status='unavailable',revision=revision+1,updated_at=now() WHERE id=$1 AND document_id=$2 AND revision=$3 AND COALESCE(payload->>'ReviewStatus','')<>'published'`, a.ID, qdoc.ID, a.Revision, raw)
		if e != nil {
			return 0, e
		}
		if tag.RowsAffected() != 1 {
			return 0, &domain.RequestError{Status: 409, Message: "题目已被其他操作修改，请重新预览"}
		}
		count++
	}
	if e = tx.Commit(ctx); e != nil {
		return 0, e
	}
	return count, nil
}
func LinkSourceAnswer(a domain.Asset, d domain.Document, page, answer int, quote string) domain.Asset {
	if answer >= 0 && answer < len(a.Payload.Options) {
		a.Payload.Answer = answer
		a.Payload.AnswerOrigin = "source"
	}
	a.Payload.Explanation = quote
	a.Payload.ReviewStatus = "pending"
	a.Payload.Source = strings.Split(a.Payload.Source, "；配对解析：")[0] + fmt.Sprintf("；配对解析：%s，第%d页", d.OriginalFilename, page)
	a.Payload.Evidence = []domain.Citation{{DocumentID: d.ID, DocumentTitle: d.OriginalFilename, RevisionID: d.ActiveRevisionID, Page: page, Location: fmt.Sprintf("第%d页", page), Excerpt: quote, LocationQuality: "page"}}
	return a
}

func (p *Postgres) DocumentQuestionAssets(ctx context.Context, uid, did string) ([]domain.Asset, error) {
	if _, e := p.Document(ctx, uid, did); e != nil {
		return nil, e
	}
	rows, e := p.DB.Query(ctx, `SELECT `+assetCols+` FROM learning_assets WHERE document_id=$1 AND kind='question' AND EXISTS(SELECT 1 FROM document_catalog WHERE document_id=$1 AND purpose IN ('questions','both')) ORDER BY id`, did)
	if e != nil {
		return nil, e
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

func ParsedFingerprint(parsed domain.ParsedDocument) string {
	raw, _ := json.Marshal(parsed)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
