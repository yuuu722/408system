package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/local/exam408-assistant/backend/internal/domain"
)

const QuestionViewVersion = "question-layout-v1"

type QuestionView struct {
	RevisionID, BaseFingerprint, SourceHash, ParserVersion string
	Pages                                                  []domain.StructuredPage
	Attempted                                              []int
	Failed                                                 []map[string]any
}

func mergeQuestionView(base domain.ParsedDocument, v QuestionView) domain.ParsedDocument {
	if v.ParserVersion != QuestionViewVersion || v.BaseFingerprint != ParsedFingerprint(base) {
		return base
	}
	out := base
	out.Pages = append([]domain.StructuredPage(nil), base.Pages...)
	key := func(p domain.StructuredPage) string { return fmt.Sprintf("%d/%d", p.Number, p.Segment) }
	replacement := map[string]domain.StructuredPage{}
	for _, p := range v.Pages {
		replacement[key(p)] = p
	}
	for i, p := range out.Pages {
		if q, ok := replacement[key(p)]; ok {
			out.Pages[i] = q
		}
	}
	return out
}
func questionStructureTx(ctx context.Context, tx pgx.Tx, id string) (domain.ParsedDocument, error) {
	var raw []byte
	if e := tx.QueryRow(ctx, `SELECT payload FROM document_parses WHERE document_id=$1 FOR SHARE`, id).Scan(&raw); e != nil {
		return domain.ParsedDocument{}, e
	}
	var base domain.ParsedDocument
	if e := json.Unmarshal(raw, &base); e != nil {
		return base, e
	}
	var v QuestionView
	e := tx.QueryRow(ctx, `SELECT v.revision_id,v.base_fingerprint,v.source_hash,v.parser_version,v.payload FROM document_question_views v JOIN documents d ON d.id=v.document_id WHERE v.document_id=$1 AND v.revision_id=COALESCE(d.active_revision_id::text,'') FOR SHARE OF v`, id).Scan(&v.RevisionID, &v.BaseFingerprint, &v.SourceHash, &v.ParserVersion, &raw)
	if e == pgx.ErrNoRows {
		return base, nil
	}
	if e != nil {
		return base, e
	}
	var body struct {
		Pages     []domain.StructuredPage
		Attempted []int
		Failed    []map[string]any
	}
	if e = json.Unmarshal(raw, &body); e != nil {
		return base, e
	}
	v.Pages = body.Pages
	v.Attempted = body.Attempted
	v.Failed = body.Failed
	return mergeQuestionView(base, v), nil
}
func (p *Postgres) QuestionStructure(ctx context.Context, uid, id string) (domain.ParsedDocument, error) {
	if _, e := p.Document(ctx, uid, id); e != nil {
		return domain.ParsedDocument{}, e
	}
	tx, e := p.DB.Begin(ctx)
	if e != nil {
		return domain.ParsedDocument{}, e
	}
	defer tx.Rollback(ctx)
	var locked string
	if e = tx.QueryRow(ctx, `SELECT id FROM documents WHERE id=$1 FOR SHARE`, id).Scan(&locked); e != nil {
		return domain.ParsedDocument{}, e
	}
	parsed, e := questionStructureTx(ctx, tx, id)
	if e != nil {
		return parsed, e
	}
	e = tx.Commit(ctx)
	return parsed, e
}
func (p *Postgres) QuestionView(ctx context.Context, uid string, d domain.Document, base domain.ParsedDocument) (QuestionView, error) {
	if _, e := p.Document(ctx, uid, d.ID); e != nil {
		return QuestionView{}, e
	}
	var v QuestionView
	var raw []byte
	e := p.DB.QueryRow(ctx, `SELECT revision_id,base_fingerprint,source_hash,parser_version,payload FROM document_question_views WHERE document_id=$1`, d.ID).Scan(&v.RevisionID, &v.BaseFingerprint, &v.SourceHash, &v.ParserVersion, &raw)
	if e == pgx.ErrNoRows {
		return QuestionView{RevisionID: d.ActiveRevisionID, BaseFingerprint: ParsedFingerprint(base), ParserVersion: QuestionViewVersion}, nil
	}
	if e != nil {
		return v, e
	}
	if v.RevisionID != d.ActiveRevisionID || v.BaseFingerprint != ParsedFingerprint(base) || v.ParserVersion != QuestionViewVersion {
		return QuestionView{RevisionID: d.ActiveRevisionID, BaseFingerprint: ParsedFingerprint(base), ParserVersion: QuestionViewVersion}, nil
	}
	var body struct {
		Pages     []domain.StructuredPage
		Attempted []int
		Failed    []map[string]any
	}
	e = json.Unmarshal(raw, &body)
	v.Pages = body.Pages
	v.Attempted = body.Attempted
	v.Failed = body.Failed
	return v, e
}
func (p *Postgres) SaveQuestionView(ctx context.Context, uid string, d domain.Document, base domain.ParsedDocument, v QuestionView) error {
	if e := p.CanMaintainDocument(ctx, uid, d.ID); e != nil {
		return e
	}
	tx, e := p.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var status, rid string
	if e = tx.QueryRow(ctx, `SELECT status,COALESCE(active_revision_id::text,'') FROM documents WHERE id=$1 FOR UPDATE`, d.ID).Scan(&status, &rid); e != nil {
		return e
	}
	stale := &domain.RequestError{Status: 409, Message: "原文件或解析已变化，停止保存题库版面视图"}
	if status != "ready" || rid != d.ActiveRevisionID || v.RevisionID != rid {
		return stale
	}
	var raw []byte
	if e = tx.QueryRow(ctx, `SELECT payload FROM document_parses WHERE document_id=$1 FOR SHARE`, d.ID).Scan(&raw); e != nil {
		return e
	}
	var current domain.ParsedDocument
	if e = json.Unmarshal(raw, &current); e != nil {
		return e
	}
	if ParsedFingerprint(current) != ParsedFingerprint(base) || v.BaseFingerprint != ParsedFingerprint(base) || v.ParserVersion != QuestionViewVersion {
		return stale
	}
	if rid != "" {
		var expected string
		if e = tx.QueryRow(ctx, `SELECT file_hash FROM document_revisions WHERE id=$1`, rid).Scan(&expected); e != nil {
			return e
		}
		if expected != "legacy" && expected != "" && expected != v.SourceHash {
			return stale
		}
	}
	var previousRaw []byte
	var previousHash string
	err := tx.QueryRow(ctx, `SELECT source_hash,payload FROM document_question_views WHERE document_id=$1 AND revision_id=$2 AND base_fingerprint=$3 AND parser_version=$4`, d.ID, v.RevisionID, v.BaseFingerprint, v.ParserVersion).Scan(&previousHash, &previousRaw)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	if err == nil {
		if previousHash != v.SourceHash {
			return stale
		}
		var old struct {
			Pages     []domain.StructuredPage
			Attempted []int
			Failed    []map[string]any
		}
		if e = json.Unmarshal(previousRaw, &old); e != nil {
			return e
		}
		found := map[int]bool{}
		for _, p := range v.Pages {
			found[p.Number] = true
		}
		for _, p := range old.Pages {
			if !found[p.Number] {
				v.Pages = append(v.Pages, p)
			}
		}
		seen := map[int]bool{}
		all := []int{}
		for _, n := range append(v.Attempted, old.Attempted...) {
			if !seen[n] {
				all = append(all, n)
				seen[n] = true
			}
		}
		v.Attempted = all
		failures := map[string]map[string]any{}
		for _, f := range append(old.Failed, v.Failed...) {
			failures[fmt.Sprint(f["Page"])] = f
		}
		for _, p := range v.Pages {
			delete(failures, fmt.Sprint(p.Number))
		}
		v.Failed = nil
		for _, f := range failures {
			v.Failed = append(v.Failed, f)
		}
	}
	raw, e = json.Marshal(struct {
		Pages     []domain.StructuredPage
		Attempted []int
		Failed    []map[string]any
	}{v.Pages, v.Attempted, v.Failed})
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO document_question_views(document_id,revision_id,base_fingerprint,source_hash,parser_version,payload) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(document_id) DO UPDATE SET revision_id=EXCLUDED.revision_id,base_fingerprint=EXCLUDED.base_fingerprint,source_hash=EXCLUDED.source_hash,parser_version=EXCLUDED.parser_version,payload=EXCLUDED.payload,updated_at=now()`, d.ID, v.RevisionID, v.BaseFingerprint, v.SourceHash, v.ParserVersion, raw)
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
