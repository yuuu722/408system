package repository

import (
	"context"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/local/exam408-assistant/backend/internal/domain"
)

var definitionPattern = regexp.MustCompile(`(?m)(?:^|\n)[# \t]*([\p{Han}A-Za-z][\p{Han}A-Za-z0-9 +/_-]{1,23})[：:]([^\n]{12,300})`)

type graphPoint struct {
	id, name, description string
	keywords              []string
}
type graphRelation struct {
	from, to, kind string
}

// buildKnowledgeGraphFromChunks conservatively links only canonical 408 knowledge
// points explicitly mentioned in a processed chunk. It does not invent new
// concepts or relations; every generated edge keeps the chunk as evidence.
func (p *Postgres) buildKnowledgeGraphFromChunks(ctx context.Context, tx pgx.Tx, uid, kid string, chunks []domain.Chunk) error {
	rows, err := tx.Query(ctx, `SELECT id,name,description,keywords FROM exam408_knowledge_points`)
	if err != nil {
		return err
	}
	points := []graphPoint{}
	for rows.Next() {
		var x graphPoint
		if err = rows.Scan(&x.id, &x.name, &x.description, &x.keywords); err != nil {
			rows.Close()
			return err
		}
		points = append(points, x)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	relRows, err := tx.Query(ctx, `SELECT from_point_id,to_point_id,relation_type FROM exam408_knowledge_point_relations`)
	if err != nil {
		return err
	}
	relations := []graphRelation{}
	for relRows.Next() {
		var x graphRelation
		if err = relRows.Scan(&x.from, &x.to, &x.kind); err != nil {
			relRows.Close()
			return err
		}
		relations = append(relations, x)
	}
	if err = relRows.Err(); err != nil {
		relRows.Close()
		return err
	}
	relRows.Close()

	for _, chunk := range chunks {
		text := strings.ToLower(chunk.Content)
		matched := map[string]string{}
		for _, point := range points {
			terms := append([]string{point.name}, point.keywords...)
			found := false
			for _, term := range terms {
				term = strings.ToLower(strings.TrimSpace(term))
				if len([]rune(term)) >= 2 && strings.Contains(text, term) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
			var entityID string
			err = tx.QueryRow(ctx, `INSERT INTO knowledge_entities(id,user_id,knowledge_base_id,name,entity_type,description,confidence,source,evidence_chunk_id)
				VALUES($1,$2,$3,$4,'knowledge_point',$5,1.0,'canonical',NULLIF($6,'')::uuid)
				ON CONFLICT(knowledge_base_id,name,entity_type) DO UPDATE SET description=EXCLUDED.description,confidence=1.0,source='canonical',evidence_chunk_id=EXCLUDED.evidence_chunk_id
				RETURNING id`, id(), uid, kid, point.name, point.description, chunk.ID).Scan(&entityID)
			if err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO exam408_entity_mappings(id,user_id,knowledge_point_id,entity_id,confidence,evidence_chunk_id)
				VALUES($1,$2,$3,$4,0.9,$5) ON CONFLICT(user_id,knowledge_point_id,entity_id) DO UPDATE SET evidence_chunk_id=EXCLUDED.evidence_chunk_id,confidence=GREATEST(exam408_entity_mappings.confidence,EXCLUDED.confidence)`, id(), uid, point.id, entityID, chunk.ID); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO exam408_resource_mappings(user_id,knowledge_point_id,document_id,confidence,source)
 VALUES($1,$2,$3,0.9,'auto') ON CONFLICT DO NOTHING`, uid, point.id, chunk.DocumentID); err != nil {
				return err
			}
			matched[point.id] = entityID
		}
		// Definition-shaped headings yield conservative local candidates, not
		// new authoritative syllabus points or inferred semantic relationships.
		for _, definition := range definitionPattern.FindAllStringSubmatch(chunk.Content, 30) {
			name := strings.TrimSpace(definition[1])
			description := strings.TrimSpace(definition[2])
			skip := map[string]bool{"例题": true, "答案": true, "来源": true, "参考答案": true, "注意": true, "说明": true, "解析": true}
			for _, point := range points {
				if strings.EqualFold(name, point.name) {
					skip[name] = true
				}
			}
			if skip[name] {
				continue
			}
			var entity string
			if err = tx.QueryRow(ctx, `INSERT INTO knowledge_entities(user_id,knowledge_base_id,name,entity_type,description,source,confidence,evidence_chunk_id) VALUES($1,$2,$3,'concept',$4,'rule_candidate',0.8,$5) ON CONFLICT(knowledge_base_id,name,entity_type) DO UPDATE SET evidence_chunk_id=EXCLUDED.evidence_chunk_id RETURNING id`, uid, kid, name, description, chunk.ID).Scan(&entity); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO knowledge_entity_evidence(entity_id,chunk_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, entity, chunk.ID); err != nil {
				return err
			}
			for _, canonical := range matched {
				if canonical == entity {
					continue
				}
				if _, err = tx.Exec(ctx, `INSERT INTO knowledge_relations(user_id,knowledge_base_id,from_entity_id,to_entity_id,relation_type,evidence_chunk_id,source,confidence) SELECT $1,$2,$3,$4,'related',$5,'rule_candidate',0.8 WHERE NOT EXISTS(SELECT 1 FROM knowledge_relations WHERE from_entity_id=$3 AND to_entity_id=$4 AND evidence_chunk_id=$5 AND relation_type='related')`, uid, kid, canonical, entity, chunk.ID); err != nil {
					return err
				}
			}
		}
		for _, rel := range relations {
			from, hasFrom := matched[rel.from]
			to, hasTo := matched[rel.to]
			if !hasFrom || !hasTo || from == to {
				continue
			}
			if _, err = tx.Exec(ctx, `INSERT INTO knowledge_relations(id,user_id,knowledge_base_id,from_entity_id,to_entity_id,relation_type,evidence_chunk_id)
				SELECT $1,$2,$3,$4,$5,$6::varchar(64),$7
				WHERE NOT EXISTS (
					SELECT 1 FROM knowledge_relations
					WHERE knowledge_base_id=$3 AND from_entity_id=$4 AND to_entity_id=$5
					  AND relation_type=$6::varchar(64) AND evidence_chunk_id=$7
				)`, id(), uid, kid, from, to, rel.kind, chunk.ID); err != nil {
				return err
			}
		}
	}
	return p.rememberGraphEvidence(ctx, tx, kid)
}

func (p *Postgres) graphProvenance(ctx context.Context, uid, kid string) (domain.KnowledgeGraph, error) {
	graph := domain.KnowledgeGraph{Nodes: []domain.GraphNode{}, Edges: []domain.GraphEdge{}}
	graph.KnowledgeBaseID = kid
	err := p.DB.QueryRow(ctx, `SELECT is_public FROM knowledge_bases WHERE id=$1 AND (user_id=$2 OR is_public=true)`, kid, uid).Scan(&graph.IsPublic)
	if err != nil {
		return graph, nf(err)
	}
	rows, err := p.DB.Query(ctx, `SELECT e.id,e.name,e.entity_type,e.description,COALESCE(m.knowledge_point_id::text,''),COALESCE(source.id::text,''),COALESCE(source.content,''),COALESCE(source.document_id::text,''),COALESCE((SELECT original_filename FROM documents WHERE id=source.document_id),''),e.source,e.confidence,e.created_at,COALESCE((SELECT lower(s.code) FROM exam408_knowledge_points k JOIN exam408_subjects s ON s.id=k.subject_id WHERE k.id=m.knowledge_point_id),(SELECT subject_code FROM document_catalog WHERE document_id=source.document_id AND review_state='confirmed' AND subject_code IN ('ds','co','os','cn')),'')
		FROM knowledge_entities e JOIN knowledge_bases kb ON kb.id=e.knowledge_base_id
		LEFT JOIN LATERAL (SELECT knowledge_point_id FROM exam408_entity_mappings WHERE entity_id=e.id AND (SELECT count(DISTINCT k.subject_id) FROM exam408_entity_mappings em JOIN exam408_knowledge_points k ON k.id=em.knowledge_point_id WHERE em.entity_id=e.id)=1 ORDER BY confidence DESC,knowledge_point_id LIMIT 1) m ON true
		LEFT JOIN LATERAL (SELECT c.* FROM document_chunks c JOIN (SELECT chunk_id FROM knowledge_entity_evidence WHERE entity_id=e.id UNION SELECT e.evidence_chunk_id WHERE e.evidence_chunk_id IS NOT NULL) links ON links.chunk_id=c.id WHERE c.knowledge_base_id=e.knowledge_base_id ORDER BY c.created_at DESC,c.id LIMIT 1) source ON true
		WHERE e.knowledge_base_id=$1 AND (e.user_id=$2 OR kb.is_public=true) AND (e.pinned OR source.id IS NOT NULL)
		ORDER BY e.name`, kid, uid)
	if err != nil {
		return graph, err
	}
	for rows.Next() {
		var n domain.GraphNode
		if err = rows.Scan(&n.ID, &n.Name, &n.EntityType, &n.Description, &n.KnowledgePointID, &n.EvidenceChunkID, &n.EvidenceExcerpt, &n.EvidenceDocumentID, &n.EvidenceDocumentTitle, &n.Source, &n.Confidence, &n.CreatedAt, &n.SubjectCode); err != nil {
			rows.Close()
			return graph, err
		}
		if len([]rune(n.EvidenceExcerpt)) > 240 {
			r := []rune(n.EvidenceExcerpt)
			n.EvidenceExcerpt = string(r[:240])
		}
		graph.Nodes = append(graph.Nodes, n)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return graph, err
	}
	rows.Close()
	edgeRows, err := p.DB.Query(ctx, `SELECT DISTINCT ON (r.from_entity_id,r.to_entity_id,r.relation_type) r.id,r.from_entity_id,r.to_entity_id,r.relation_type,COALESCE(r.evidence_chunk_id::text,''),COALESCE(c.content,''),COALESCE(c.document_id::text,''),COALESCE((SELECT original_filename FROM documents WHERE id=c.document_id),''),r.source,r.confidence,r.created_at,count(*) OVER(PARTITION BY r.from_entity_id,r.to_entity_id,r.relation_type)
		FROM knowledge_relations r JOIN knowledge_bases kb ON kb.id=r.knowledge_base_id
		LEFT JOIN document_chunks c ON c.id=r.evidence_chunk_id
		WHERE r.knowledge_base_id=$1 AND (r.user_id=$2 OR kb.is_public=true) AND (r.source='canonical' OR r.evidence_chunk_id IS NOT NULL)
		ORDER BY r.from_entity_id,r.to_entity_id,r.relation_type,r.confidence DESC,r.created_at DESC,r.id`, kid, uid)
	if err != nil {
		return graph, err
	}
	defer edgeRows.Close()
	for edgeRows.Next() {
		var e domain.GraphEdge
		if err = edgeRows.Scan(&e.ID, &e.FromEntityID, &e.ToEntityID, &e.RelationType, &e.EvidenceChunkID, &e.EvidenceExcerpt, &e.EvidenceDocumentID, &e.EvidenceDocumentTitle, &e.Source, &e.Confidence, &e.CreatedAt, &e.EvidenceCount); err != nil {
			return graph, err
		}
		if len([]rune(e.EvidenceExcerpt)) > 240 {
			r := []rune(e.EvidenceExcerpt)
			e.EvidenceExcerpt = string(r[:240])
		}
		graph.Edges = append(graph.Edges, e)
	}
	return graph, edgeRows.Err()
}

// AddValidatedGraphRelations accepts only high-confidence proposals whose
// endpoints match canonical 408 point codes and whose evidence is an exact
// substring of a processed chunk in the same document.
func (p *Postgres) AddValidatedGraphRelations(ctx context.Context, uid, kid, did string, candidates []domain.GraphRelationCandidate) error {
	tx, err := p.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var allowed bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM documents d JOIN knowledge_bases kb ON kb.id=d.knowledge_base_id WHERE d.id=$1 AND d.user_id=$2 AND d.knowledge_base_id=$3 AND (kb.user_id=$2 OR kb.is_public=true))`, did, uid, kid).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrNotFound
	}

	for _, candidate := range candidates {
		if candidate.Confidence < 0.75 || candidate.Confidence > 1 ||
			(candidate.RelationType != "prerequisite" && candidate.RelationType != "related") ||
			candidate.FromPointCode == "" || candidate.ToPointCode == "" ||
			candidate.FromPointCode == candidate.ToPointCode || len([]rune(candidate.Evidence)) < 12 ||
			len([]rune(candidate.Evidence)) > 300 {
			continue
		}
		var fromID, fromName, fromDescription, toID, toName, toDescription string
		err = tx.QueryRow(ctx, `SELECT id,name,description FROM exam408_knowledge_points WHERE code=$1`, candidate.FromPointCode).Scan(&fromID, &fromName, &fromDescription)
		if err != nil {
			if err == pgx.ErrNoRows {
				continue
			}
			return err
		}
		err = tx.QueryRow(ctx, `SELECT id,name,description FROM exam408_knowledge_points WHERE code=$1`, candidate.ToPointCode).Scan(&toID, &toName, &toDescription)
		if err != nil {
			if err == pgx.ErrNoRows {
				continue
			}
			return err
		}
		if fromID == toID {
			continue
		}

		var chunkID, content string
		err = tx.QueryRow(ctx, `SELECT id,content FROM document_chunks WHERE document_id=$1 AND user_id=$2 AND knowledge_base_id=$3 AND strpos(content,$4)>0 ORDER BY chunk_index LIMIT 1`, did, uid, kid, candidate.Evidence).Scan(&chunkID, &content)
		if err != nil {
			if err == pgx.ErrNoRows {
				continue
			}
			return err
		}
		lower := strings.ToLower(content)
		if !strings.Contains(lower, strings.ToLower(fromName)) || !strings.Contains(lower, strings.ToLower(toName)) {
			continue
		}

		var fromEntity, toEntity string
		if err = tx.QueryRow(ctx, `INSERT INTO knowledge_entities(id,user_id,knowledge_base_id,name,entity_type,description)
			VALUES($1,$2,$3,$4,'knowledge_point',$5)
			ON CONFLICT(knowledge_base_id,name,entity_type) DO UPDATE SET description=EXCLUDED.description RETURNING id`,
			id(), uid, kid, fromName, fromDescription).Scan(&fromEntity); err != nil {
			return err
		}
		if err = tx.QueryRow(ctx, `INSERT INTO knowledge_entities(id,user_id,knowledge_base_id,name,entity_type,description)
			VALUES($1,$2,$3,$4,'knowledge_point',$5)
			ON CONFLICT(knowledge_base_id,name,entity_type) DO UPDATE SET description=EXCLUDED.description RETURNING id`,
			id(), uid, kid, toName, toDescription).Scan(&toEntity); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO exam408_entity_mappings(id,user_id,knowledge_point_id,entity_id,confidence,evidence_chunk_id)
			VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(user_id,knowledge_point_id,entity_id)
			DO UPDATE SET confidence=GREATEST(exam408_entity_mappings.confidence,EXCLUDED.confidence),evidence_chunk_id=EXCLUDED.evidence_chunk_id`,
			id(), uid, fromID, fromEntity, candidate.Confidence, chunkID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO exam408_entity_mappings(id,user_id,knowledge_point_id,entity_id,confidence,evidence_chunk_id)
			VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(user_id,knowledge_point_id,entity_id)
			DO UPDATE SET confidence=GREATEST(exam408_entity_mappings.confidence,EXCLUDED.confidence),evidence_chunk_id=EXCLUDED.evidence_chunk_id`,
			id(), uid, toID, toEntity, candidate.Confidence, chunkID); err != nil {
			return err
		}

		// Reject a prerequisite proposal that directly contradicts an existing
		// authoritative prerequisite direction.
		if candidate.RelationType == "prerequisite" {
			var reverse bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM exam408_knowledge_point_relations WHERE from_point_id=$1 AND to_point_id=$2 AND relation_type='prerequisite')`, toID, fromID).Scan(&reverse); err != nil {
				return err
			}
			if reverse {
				continue
			}
		}
		if _, err = tx.Exec(ctx, `INSERT INTO knowledge_relations(id,user_id,knowledge_base_id,from_entity_id,to_entity_id,relation_type,evidence_chunk_id,confidence,source)
			SELECT $1,$2,$3,$4,$5,$6::varchar(64),$7,$8,'llm_candidate'
			WHERE NOT EXISTS(SELECT 1 FROM knowledge_relations WHERE knowledge_base_id=$3 AND from_entity_id=$4 AND to_entity_id=$5 AND relation_type=$6::varchar(64) AND evidence_chunk_id=$7)`,
			id(), uid, kid, fromEntity, toEntity, candidate.RelationType, chunkID, candidate.Confidence); err != nil {
			return err
		}
	}
	if err = p.rememberGraphEvidence(ctx, tx, kid); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AddValidatedGraphCandidates persists model-proposed entities and relations
// only when each has exact source evidence and all relation endpoints resolve
// to a canonical point or a separately validated candidate node.
func (p *Postgres) AddValidatedGraphCandidates(ctx context.Context, uid, kid, did string, entities []domain.GraphEntityCandidate, relations []domain.GraphRelationCandidate) error {
	tx, err := p.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var allowed bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM documents d JOIN knowledge_bases kb ON kb.id=d.knowledge_base_id WHERE d.id=$1 AND d.user_id=$2 AND d.knowledge_base_id=$3 AND (kb.user_id=$2 OR kb.is_public=true))`, did, uid, kid).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrNotFound
	}

	type sourceChunk struct{ id, content string }
	chunkRows, err := tx.Query(ctx, `SELECT id,content FROM document_chunks WHERE document_id=$1 AND user_id=$2 AND knowledge_base_id=$3 ORDER BY chunk_index`, did, uid, kid)
	if err != nil {
		return err
	}
	chunks := []sourceChunk{}
	for chunkRows.Next() {
		var ch sourceChunk
		if err = chunkRows.Scan(&ch.id, &ch.content); err != nil {
			chunkRows.Close()
			return err
		}
		chunks = append(chunks, ch)
	}
	if err = chunkRows.Err(); err != nil {
		chunkRows.Close()
		return err
	}
	chunkRows.Close()

	type canonicalPoint struct{ id, code, name, description string }
	pointRows, err := tx.Query(ctx, `SELECT id,code,name,description FROM exam408_knowledge_points`)
	if err != nil {
		return err
	}
	pointsByCode := map[string]canonicalPoint{}
	pointsByName := map[string]canonicalPoint{}
	for pointRows.Next() {
		var pt canonicalPoint
		if err = pointRows.Scan(&pt.id, &pt.code, &pt.name, &pt.description); err != nil {
			pointRows.Close()
			return err
		}
		pointsByCode[pt.code] = pt
		pointsByName[strings.ToLower(pt.name)] = pt
	}
	if err = pointRows.Err(); err != nil {
		pointRows.Close()
		return err
	}
	pointRows.Close()

	findEvidence := func(evidence, requiredA, requiredB string) (sourceChunk, bool) {
		if len([]rune(evidence)) < 12 || len([]rune(evidence)) > 300 {
			return sourceChunk{}, false
		}
		for _, ch := range chunks {
			if strings.Contains(ch.content, evidence) &&
				(requiredA == "" || strings.Contains(strings.ToLower(evidence), strings.ToLower(requiredA))) &&
				(requiredB == "" || strings.Contains(strings.ToLower(evidence), strings.ToLower(requiredB))) {
				return ch, true
			}
		}
		return sourceChunk{}, false
	}
	entityIDs := map[string]string{}
	ensureCanonical := func(pt canonicalPoint, evidenceChunkID string, confidence float64) (string, error) {
		var entityID string
		err := tx.QueryRow(ctx, `INSERT INTO knowledge_entities(id,user_id,knowledge_base_id,name,entity_type,description,confidence,source,evidence_chunk_id)
			VALUES($1,$2,$3,$4,'knowledge_point',$5,1.0,'canonical',NULLIF($6,'')::uuid)
			ON CONFLICT(knowledge_base_id,name,entity_type) DO UPDATE SET description=EXCLUDED.description,confidence=1.0,source='canonical',evidence_chunk_id=COALESCE(EXCLUDED.evidence_chunk_id,knowledge_entities.evidence_chunk_id)
			RETURNING id`, id(), uid, kid, pt.name, pt.description, evidenceChunkID).Scan(&entityID)
		if err != nil {
			return "", err
		}
		if evidenceChunkID != "" {
			if _, err = tx.Exec(ctx, `INSERT INTO exam408_entity_mappings(id,user_id,knowledge_point_id,entity_id,confidence,evidence_chunk_id)
				VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(user_id,knowledge_point_id,entity_id)
				DO UPDATE SET confidence=GREATEST(exam408_entity_mappings.confidence,EXCLUDED.confidence),evidence_chunk_id=EXCLUDED.evidence_chunk_id`,
				id(), uid, pt.id, entityID, confidence, evidenceChunkID); err != nil {
				return "", err
			}
		}
		entityIDs[strings.ToLower(pt.name)] = entityID
		entityIDs[pt.code] = entityID
		return entityID, nil
	}

	if len(entities) > 200 {
		entities = entities[:200]
	}
	allowedTypes := map[string]bool{"concept": true, "algorithm": true, "data_structure": true, "protocol": true, "formula": true, "mechanism": true, "component": true}
	for _, candidate := range entities {
		name := strings.TrimSpace(candidate.Name)
		kind := strings.TrimSpace(candidate.EntityType)
		if len([]rune(name)) < 2 || len([]rune(name)) > 100 || !allowedTypes[kind] ||
			len([]rune(candidate.Description)) > 500 || candidate.Confidence < 0.75 || candidate.Confidence > 1 {
			continue
		}
		if !strings.Contains(strings.ToLower(candidate.Evidence), strings.ToLower(name)) {
			continue
		}
		ch, ok := findEvidence(candidate.Evidence, name, "")
		if !ok {
			continue
		}
		if pt, ok := pointsByName[strings.ToLower(name)]; ok {
			if _, err = ensureCanonical(pt, ch.id, candidate.Confidence); err != nil {
				return err
			}
			continue
		}
		var entityID string
		err = tx.QueryRow(ctx, `INSERT INTO knowledge_entities(id,user_id,knowledge_base_id,name,entity_type,description,confidence,source,evidence_chunk_id)
			VALUES($1,$2,$3,$4,$5,$6,$7,'llm_candidate',$8)
			ON CONFLICT(knowledge_base_id,name,entity_type) DO UPDATE SET
				description=CASE WHEN knowledge_entities.source='llm_candidate' THEN EXCLUDED.description ELSE knowledge_entities.description END,
				confidence=CASE WHEN knowledge_entities.source='llm_candidate' THEN GREATEST(knowledge_entities.confidence,EXCLUDED.confidence) ELSE knowledge_entities.confidence END,
				evidence_chunk_id=CASE WHEN knowledge_entities.source='llm_candidate' THEN EXCLUDED.evidence_chunk_id ELSE knowledge_entities.evidence_chunk_id END
			RETURNING id`, id(), uid, kid, name, kind, candidate.Description, candidate.Confidence, ch.id).Scan(&entityID)
		if err != nil {
			return err
		}
		entityIDs[strings.ToLower(name)] = entityID
	}

	allowedRelations := map[string]bool{"prerequisite": true, "related": true, "part_of": true, "implements": true, "uses": true, "applies_to": true}
	if len(relations) > 200 {
		relations = relations[:200]
	}
	for _, candidate := range relations {
		if candidate.Confidence < 0.75 || candidate.Confidence > 1 || !allowedRelations[candidate.RelationType] {
			continue
		}
		fromKey, toKey := strings.TrimSpace(candidate.FromName), strings.TrimSpace(candidate.ToName)
		if candidate.FromPointCode != "" {
			fromKey = candidate.FromPointCode
		}
		if candidate.ToPointCode != "" {
			toKey = candidate.ToPointCode
		}
		resolve := func(key string) (string, bool, error) {
			if pt, ok := pointsByCode[key]; ok {
				id, e := ensureCanonical(pt, "", candidate.Confidence)
				return id, e == nil, e
			}
			if pt, ok := pointsByName[strings.ToLower(key)]; ok {
				id, e := ensureCanonical(pt, "", candidate.Confidence)
				return id, e == nil, e
			}
			id, ok := entityIDs[strings.ToLower(key)]
			return id, ok, nil
		}

		ch, ok := findEvidence(candidate.Evidence, fromKey, toKey)
		if !ok {
			// Codes may not literally occur in source text; fall back to the
			// canonical display names for evidence matching, never to codes.
			fromName, toName := fromKey, toKey
			if pt, yes := pointsByCode[fromKey]; yes {
				fromName = pt.name
			}
			if pt, yes := pointsByCode[toKey]; yes {
				toName = pt.name
			}
			ch, ok = findEvidence(candidate.Evidence, fromName, toName)
		}
		if !ok {
			continue
		}
		fromEntity, hasFrom, e := resolve(fromKey)
		if e != nil {
			return e
		}
		toEntity, hasTo, e := resolve(toKey)
		if e != nil {
			return e
		}
		if !hasFrom || !hasTo || fromEntity == toEntity {
			continue
		}
		if candidate.RelationType == "prerequisite" {
			var reverse bool
			var fromPointID, toPointID string
			for _, pt := range pointsByCode {
				if pt.name == fromKey || pt.code == fromKey {
					fromPointID = pt.id
				}
				if pt.name == toKey || pt.code == toKey {
					toPointID = pt.id
				}
			}
			if fromPointID != "" && toPointID != "" {
				if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM exam408_knowledge_point_relations WHERE from_point_id=$1 AND to_point_id=$2 AND relation_type='prerequisite')`, toPointID, fromPointID).Scan(&reverse); err != nil {
					return err
				}
				if reverse {
					continue
				}
			}
		}
		if _, err = tx.Exec(ctx, `INSERT INTO knowledge_relations(id,user_id,knowledge_base_id,from_entity_id,to_entity_id,relation_type,evidence_chunk_id,confidence,source)
			SELECT $1,$2,$3,$4,$5,$6::varchar(64),$7,$8,'llm_candidate'
			WHERE NOT EXISTS(SELECT 1 FROM knowledge_relations WHERE knowledge_base_id=$3 AND from_entity_id=$4 AND to_entity_id=$5 AND relation_type=$6::varchar(64) AND evidence_chunk_id=$7)`,
			id(), uid, kid, fromEntity, toEntity, candidate.RelationType, ch.id, candidate.Confidence); err != nil {
			return err
		}
	}
	if err = p.rememberGraphEvidence(ctx, tx, kid); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
