package repository

import (
	"context"

	"github.com/local/exam408-assistant/backend/internal/domain"
)

type AssetPageFilter struct {
	Kind, Point, Search, Review string
	Children, Admin, Personal   bool
	Page, Limit                 int
}

type AssetCounts struct {
	All, Ready, UniqueReady, Repair, NoAnswer, Pending int
}

type AssetPage struct {
	Items  []domain.Asset
	Total  int
	Counts AssetCounts
}

const visibleAssetsSQL = `COALESCE(seed_key,'') NOT LIKE '408-diagnostic-2026:%'
	AND (is_public OR ($3 AND NOT $2 AND user_id=$1))
	AND (NOT is_public OR status='ready' OR $2)
	AND (COALESCE(payload->>'ExtractionKey','')='' OR NOT EXISTS(
		SELECT 1 FROM document_catalog dc WHERE dc.document_id=a.document_id AND dc.purpose='knowledge'))`

const assetPageScopeSQL = `WITH RECURSIVE point_tree(id) AS (
	SELECT id FROM exam408_knowledge_points WHERE id::text=$5
	UNION SELECT p.id FROM exam408_knowledge_points p JOIN point_tree t ON p.parent_id=t.id WHERE $6
), searchable(id) AS (
	SELECT p.id FROM exam408_knowledge_points p JOIN exam408_subjects s ON s.id=p.subject_id
	WHERE $7<>'' AND strpos(lower(concat_ws(' ',p.name,p.code,s.name,array_to_string(p.keywords,' '))),lower($7))>0
	UNION SELECT p.id FROM exam408_knowledge_points p JOIN searchable t ON p.parent_id=t.id
), matching AS (
	SELECT a.*, row_number() OVER (PARTITION BY CASE WHEN NOT $2 AND kind='question' AND is_public
		AND status='ready' AND payload->>'FamilyID' LIKE 'public-question-v1:%'
		THEN concat_ws(':',payload->>'FamilyID',point_id,payload->>'Answer',payload->>'AnswerOrigin',md5(jsonb_build_array(payload->>'Stem',payload->'Options')::text))
		ELSE a.id::text END ORDER BY updated_at DESC,a.id) AS variant
	FROM learning_assets a WHERE ` + visibleAssetsSQL + `
	AND ($4='' OR kind=$4)
	AND ($5='' OR ($5='unassigned' AND point_id IS NULL) OR point_id IN (SELECT id FROM point_tree))
	AND ($7='' OR point_id IN (SELECT id FROM searchable) OR
		strpos(lower(concat_ws(' ',title,payload->>'Stem',payload->>'Source')),lower($7))>0)
	AND ($8='' OR CASE
		WHEN $8='published' THEN status='ready'
		WHEN NOT $2 THEN true
		WHEN $8='repair' THEN status<>'ready' AND (COALESCE(payload->>'Issue','')<>'' OR (kind='question' AND
			CASE WHEN jsonb_typeof(payload->'Options')='array' THEN jsonb_array_length(payload->'Options') ELSE 0 END<2))
		ELSE status<>'ready' AND COALESCE(payload->>'Issue','')='' AND (kind<>'question' OR
			CASE WHEN jsonb_typeof(payload->'Options')='array' THEN jsonb_array_length(payload->'Options') ELSE 0 END>=2)
	END)
) `

func (p *Postgres) ListAssetPage(ctx context.Context, uid string, filter AssetPageFilter) (AssetPage, error) {
	out := AssetPage{Items: []domain.Asset{}}
	args := []any{uid, filter.Admin, filter.Personal, filter.Kind, filter.Point, filter.Children, filter.Search, filter.Review}
	if err := p.DB.QueryRow(ctx, assetPageScopeSQL+`SELECT count(*) FROM matching WHERE variant=1`, args...).Scan(&out.Total); err != nil {
		return out, err
	}
	if filter.Admin {
		if err := p.DB.QueryRow(ctx, `SELECT count(*),
			count(*) FILTER(WHERE status='ready'),
			count(DISTINCT COALESCE(NULLIF(payload->>'FamilyID',''),id::text)) FILTER(WHERE status='ready'),
			count(*) FILTER(WHERE COALESCE(payload->>'Issue','')<>''),
			count(*) FILTER(WHERE COALESCE(payload->>'AnswerOrigin','')<>'source' OR COALESCE(payload->>'Answer','-1') LIKE '-%'),
			count(*) FILTER(WHERE status<>'ready' AND COALESCE(payload->>'Issue','')='')
			FROM learning_assets a WHERE kind='question' AND `+visibleAssetsSQL, args[:3]...).Scan(
			&out.Counts.All, &out.Counts.Ready, &out.Counts.UniqueReady, &out.Counts.Repair, &out.Counts.NoAnswer, &out.Counts.Pending); err != nil {
			return out, err
		}
	}
	rows, err := p.DB.Query(ctx, assetPageScopeSQL+`SELECT `+assetCols+` FROM matching
		WHERE variant=1 ORDER BY is_public DESC,updated_at DESC,id LIMIT $9 OFFSET $10`,
		append(args, filter.Limit, (filter.Page-1)*filter.Limit)...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		asset, err := scanAsset(rows)
		if err != nil {
			return out, err
		}
		domain.MarkQuestionTextQuality(&asset)
		if !filter.Admin && asset.IsPublic && asset.Status != "ready" {
			continue
		}
		out.Items = append(out.Items, asset)
	}
	return out, rows.Err()
}
