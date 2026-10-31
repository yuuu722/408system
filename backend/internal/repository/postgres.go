package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/local/exam408-assistant/backend/internal/domain"
)

var ErrNotFound = errors.New("not found")

type Postgres struct {
	Neo4j              *Neo4jGraph
	DocumentJobTimeout time.Duration
	DB                 *pgxpool.Pool
	EmbeddingSpace     string
}

func Open(ctx context.Context, url string) (*Postgres, error) {
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err = db.Ping(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return &Postgres{DB: db}, nil
}
func id() string { return uuid.NewString() }
func nf(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (p *Postgres) CreateUser(ctx context.Context, u domain.User) (domain.User, error) {
	u.ID = id()
	err := p.DB.QueryRow(ctx, `INSERT INTO users(id,username,email,password_hash) VALUES($1,$2,$3,$4) RETURNING created_at,role,disabled`, u.ID, u.Username, u.Email, u.PasswordHash).Scan(&u.CreatedAt, &u.Role, &u.Disabled)
	return u, err
}
func scanUser(row pgx.Row) (u domain.User, err error) {
	err = row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt, &u.Role, &u.Disabled)
	return u, nf(err)
}
func (p *Postgres) UserByLogin(ctx context.Context, login string) (domain.User, error) {
	return scanUser(p.DB.QueryRow(ctx, `SELECT id,username,email,password_hash,created_at,role,disabled FROM users WHERE lower(email)=lower($1) OR lower(username)=lower($1)`, login))
}
func (p *Postgres) UserByID(ctx context.Context, uid string) (domain.User, error) {
	return scanUser(p.DB.QueryRow(ctx, `SELECT id,username,email,password_hash,created_at,role,disabled FROM users WHERE id=$1`, uid))
}

func (p *Postgres) CreateKnowledgeBase(ctx context.Context, uid, name, desc string) (k domain.KnowledgeBase, err error) {
	k.ID = id()
	k.UserID = uid
	k.Name = name
	k.Description = desc
	tx, e := p.DB.Begin(ctx)
	if e != nil {
		return k, e
	}
	defer tx.Rollback(ctx)
	var quota, count int
	if e = tx.QueryRow(ctx, `SELECT personal_kb_limit FROM users WHERE id=$1 FOR UPDATE`, uid).Scan(&quota); e != nil {
		return k, nf(e)
	}
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM knowledge_bases WHERE user_id=$1 AND NOT is_public`, uid).Scan(&count); e != nil {
		return k, e
	}
	if count >= quota {
		return k, &domain.RequestError{Status: 409, Message: "个人知识库数量已达到成员配额，请整理已有知识库或联系管理员调整配额"}
	}
	err = tx.QueryRow(ctx, `INSERT INTO knowledge_bases(id,user_id,name,description) VALUES($1,$2,$3,$4) RETURNING created_at,updated_at,is_public`, k.ID, uid, name, desc).Scan(&k.CreatedAt, &k.UpdatedAt, &k.IsPublic)
	if err == nil {
		err = tx.Commit(ctx)
	}
	return
}
func scanKB(row pgx.Row) (k domain.KnowledgeBase, err error) {
	err = row.Scan(&k.ID, &k.UserID, &k.Name, &k.Description, &k.CreatedAt, &k.UpdatedAt, &k.IsPublic)
	return k, nf(err)
}
func (p *Postgres) KnowledgeBase(ctx context.Context, uid, kid string) (domain.KnowledgeBase, error) {
	return scanKB(p.DB.QueryRow(ctx, `SELECT id,user_id,name,description,created_at,updated_at,is_public FROM knowledge_bases WHERE id=$1 AND (user_id=$2 OR is_public=true)`, kid, uid))
}
func (p *Postgres) ListKnowledgeBases(ctx context.Context, uid string) (out []domain.KnowledgeBase, err error) {
	rows, err := p.DB.Query(ctx, `SELECT id,user_id,name,description,created_at,updated_at,is_public FROM knowledge_bases WHERE user_id=$1 OR is_public=true ORDER BY is_public DESC,updated_at DESC`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		k, e := scanKB(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
func (p *Postgres) DeleteKnowledgeBase(ctx context.Context, uid, kid string) error {
	tag, err := p.DB.Exec(ctx, `DELETE FROM knowledge_bases WHERE id=$1 AND user_id=$2 AND is_public=false`, kid, uid)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (p *Postgres) CreateDocument(ctx context.Context, d domain.Document) (domain.Document, error) {
	d.ID = id()
	tx, err := p.DB.Begin(ctx)
	if err != nil {
		return d, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `INSERT INTO documents(id,knowledge_base_id,user_id,filename,original_filename,mime_type,size,storage_path,resource_type,status) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10 WHERE EXISTS(SELECT 1 FROM knowledge_bases kb WHERE kb.id=$2 AND ((kb.user_id=$3 AND kb.is_public=false) OR (kb.is_public=true AND EXISTS(SELECT 1 FROM users u WHERE u.id=$3 AND u.role='admin')))) RETURNING created_at,updated_at`, d.ID, d.KnowledgeBaseID, d.UserID, d.Filename, d.OriginalFilename, d.MimeType, d.Size, d.StoragePath, d.ResourceType, d.Status).Scan(&d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return d, nf(err)
	}
	purpose := d.Purpose
	if purpose == "" {
		purpose = "knowledge"
	}
	if _, err = tx.Exec(ctx, `INSERT INTO document_catalog(document_id,resource_type,purpose) VALUES($1,$2,$3)`, d.ID, d.ResourceType, purpose); err != nil {
		return d, err
	}
	return d, tx.Commit(ctx)
}
func scanDoc(row pgx.Row) (d domain.Document, err error) {
	err = row.Scan(&d.ID, &d.KnowledgeBaseID, &d.UserID, &d.Filename, &d.OriginalFilename, &d.MimeType, &d.Size, &d.StoragePath, &d.ResourceType, &d.Status, &d.ErrorMessage, &d.CreatedAt, &d.UpdatedAt, &d.Progress, &d.ProcessingStage, &d.ProcessedChunks, &d.TotalChunks, &d.ActiveRevisionID)
	return d, nf(err)
}

const docCols = `id,knowledge_base_id,user_id,filename,original_filename,mime_type,size,storage_path,resource_type,status,error_message,created_at,updated_at,progress,processing_stage,processed_chunks,total_chunks,COALESCE(active_revision_id::text,'')`

func (p *Postgres) Document(ctx context.Context, uid, did string) (domain.Document, error) {
	return scanDoc(p.DB.QueryRow(ctx, `SELECT `+docCols+` FROM documents d WHERE d.id=$1 AND (d.user_id=$2 OR EXISTS(SELECT 1 FROM knowledge_bases kb WHERE kb.id=d.knowledge_base_id AND kb.is_public=true))`, did, uid))
}
func (p *Postgres) ListDocuments(ctx context.Context, uid, kid string) (out []domain.Document, err error) {
	q := `SELECT ` + docCols + ` FROM documents WHERE user_id=$1`
	args := []any{uid}
	if kid != "" {
		q = `SELECT ` + docCols + ` FROM documents WHERE knowledge_base_id=$2 AND (user_id=$1 OR EXISTS(SELECT 1 FROM knowledge_bases kb WHERE kb.id=$2 AND kb.is_public=true))`
		args = []any{uid, kid}
	}
	q += ` ORDER BY created_at DESC`
	rows, err := p.DB.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		d, e := scanDoc(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (p *Postgres) UpdateDocumentStatus(ctx context.Context, did, status, msg string) error {
	tag, err := p.DB.Exec(ctx, `UPDATE documents SET status=$2::text,error_message=$3,updated_at=now(),progress=CASE WHEN $2::text='processing' THEN 0 WHEN $2::text='ready' THEN 100 ELSE progress END,processing_stage=CASE WHEN $2::text='processing' THEN 'queued' WHEN $2::text='ready' THEN 'completed' ELSE processing_stage END,processed_chunks=CASE WHEN $2::text='processing' THEN 0 ELSE processed_chunks END,total_chunks=CASE WHEN $2::text='processing' THEN 0 ELSE total_chunks END WHERE id=$1`, did, status, msg)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
func (p *Postgres) DeleteDocument(ctx context.Context, uid, did string) error {
	if err := p.CanMaintainDocument(ctx, uid, did); err != nil {
		return err
	}
	d, err := p.Document(ctx, uid, did)
	if err != nil {
		return err
	}
	tx, err := p.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM documents WHERE id=$1`, did); err != nil {
		return err
	}
	if err = p.pruneGraph(ctx, tx, d.KnowledgeBaseID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) UpdateDocumentProgress(ctx context.Context, did, stage string, progress, done, total int) error {
	_, err := p.DB.Exec(ctx, `UPDATE documents SET processing_stage=$2,progress=$3,processed_chunks=$4,total_chunks=$5,updated_at=now() WHERE id=$1 AND status='processing'`, did, stage, progress, done, total)
	return err
}

func vectorLiteral(v []float32) string {
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = fmt.Sprintf("%.8f", n)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
func (p *Postgres) ReplaceChunks(ctx context.Context, uid, did string, chunks []domain.Chunk) error {
	tx, err := p.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM knowledge_relations WHERE evidence_chunk_id IN (SELECT id FROM document_chunks WHERE document_id=$1 AND user_id=$2)`, did, uid); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM document_chunks WHERE document_id=$1 AND user_id=$2`, did, uid); err != nil {
		return err
	}
	for _, c := range chunks {
		meta, _ := json.Marshal(c.Metadata)
		_, err = tx.Exec(ctx, `INSERT INTO document_chunks(id,document_id,knowledge_base_id,user_id,content,chunk_index,source_location,metadata,embedding) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::vector)`, c.ID, c.DocumentID, c.KnowledgeBaseID, c.UserID, c.Content, c.ChunkIndex, c.SourceLocation, meta, vectorLiteral(c.Embedding))
		if err != nil {
			return err
		}
	}
	kid := ""
	if len(chunks) > 0 {
		kid = chunks[0].KnowledgeBaseID
	}
	if err = p.buildKnowledgeGraphFromChunks(ctx, tx, uid, kid, chunks); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (p *Postgres) SearchChunks(ctx context.Context, uid, kid string, v []float32, limit int) (out []domain.Chunk, err error) {
	return p.SearchChunksScoped(ctx, uid, kid, v, limit, true)
}

func (p *Postgres) SearchChunksScoped(ctx context.Context, uid, kid string, v []float32, limit int, _ bool) (out []domain.Chunk, err error) {
	// Materialize before distance calculation: legacy dimensions must never reach <=>.
	rows, err := p.DB.Query(ctx, `WITH compatible AS MATERIALIZED (
 SELECT c.*,d.original_filename,kb.is_public,COALESCE(dc.resource_type,d.resource_type) source_kind FROM document_chunks c
 JOIN documents d ON d.id=c.document_id JOIN knowledge_bases kb ON kb.id=c.knowledge_base_id LEFT JOIN document_catalog dc ON dc.document_id=d.id
 WHERE EXISTS(SELECT 1 FROM knowledge_bases chosen WHERE chosen.id=$2 AND (chosen.user_id=$1 OR chosen.is_public))
 AND c.knowledge_base_id=$2 AND (kb.is_public OR c.user_id=$1)
 AND (d.status='ready' OR d.active_revision_id IS NOT NULL)
 AND NOT EXISTS(SELECT 1 FROM document_catalog dc WHERE dc.document_id=d.id AND dc.purpose='questions')
 AND vector_dims(c.embedding)=$5 AND c.metadata->>'embedding_space'=$6
 ), ranked AS (SELECT *,row_number() OVER(PARTITION BY is_public,source_kind ORDER BY embedding <=> $3::vector) rank FROM compatible)
 SELECT c.id,c.document_id,c.knowledge_base_id,c.user_id,c.content,c.chunk_index,c.source_location,c.metadata || jsonb_build_object('resource_type',c.source_kind,'is_public',c.is_public),c.original_filename,
 1-(c.embedding <=> $3::vector) score,c.created_at FROM ranked c WHERE rank<=$4 ORDER BY is_public DESC,score DESC`, uid, kid, vectorLiteral(v), limit, len(v), p.EmbeddingSpace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c domain.Chunk
		var raw []byte
		if err = rows.Scan(&c.ID, &c.DocumentID, &c.KnowledgeBaseID, &c.UserID, &c.Content, &c.ChunkIndex, &c.SourceLocation, &raw, &c.DocumentTitle, &c.Score, &c.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &c.Metadata)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (p *Postgres) ListSubjects(ctx context.Context) (out []domain.Subject, err error) {
	rows, err := p.DB.Query(ctx, `SELECT id,code,name,sort_order FROM exam408_subjects ORDER BY sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s domain.Subject
		if err = rows.Scan(&s.ID, &s.Code, &s.Name, &s.SortOrder); err != nil {
			return
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
func (p *Postgres) ListKnowledgePoints(ctx context.Context) (out []domain.KnowledgePoint, err error) {
	rows, err := p.DB.Query(ctx, `SELECT k.id,k.subject_id,COALESCE(k.parent_id::text,''),k.code,k.name,k.description,k.keywords,k.level,k.sort_order,s.name FROM exam408_knowledge_points k JOIN exam408_subjects s ON s.id=k.subject_id ORDER BY s.sort_order,k.sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k domain.KnowledgePoint
		if err = rows.Scan(&k.ID, &k.SubjectID, &k.ParentID, &k.Code, &k.Name, &k.Description, &k.Keywords, &k.Level, &k.SortOrder, &k.SubjectName); err != nil {
			return
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
func (p *Postgres) KnowledgePoint(ctx context.Context, kid string) (k domain.KnowledgePoint, err error) {
	err = p.DB.QueryRow(ctx, `SELECT k.id,k.subject_id,COALESCE(k.parent_id::text,''),k.code,k.name,k.description,k.keywords,k.level,k.sort_order,s.name FROM exam408_knowledge_points k JOIN exam408_subjects s ON s.id=k.subject_id WHERE k.id=$1`, kid).Scan(&k.ID, &k.SubjectID, &k.ParentID, &k.Code, &k.Name, &k.Description, &k.Keywords, &k.Level, &k.SortOrder, &k.SubjectName)
	return k, nf(err)
}
func (p *Postgres) PointRelations(ctx context.Context, kid string) (out []domain.PointRelation, err error) {
	rows, err := p.DB.Query(ctx, `SELECT id,from_point_id,to_point_id,relation_type FROM exam408_knowledge_point_relations WHERE from_point_id=$1 OR to_point_id=$1`, kid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r domain.PointRelation
		if err = rows.Scan(&r.ID, &r.FromPointID, &r.ToPointID, &r.RelationType); err != nil {
			return
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (p *Postgres) MapDocumentToPoint(ctx context.Context, uid, did, kpid string) error {
	if err := p.CanMaintainDocument(ctx, uid, did); err != nil {
		return err
	}
	_, err := p.DB.Exec(ctx, `INSERT INTO exam408_resource_mappings(id,user_id,knowledge_point_id,document_id,confidence,source) SELECT $1,$2,$3,$4,1,'manual' WHERE EXISTS(SELECT 1 FROM documents d JOIN knowledge_bases kb ON kb.id=d.knowledge_base_id WHERE d.id=$4 AND (d.user_id=$2 OR kb.is_public=true)) ON CONFLICT DO NOTHING`, id(), uid, kpid, did)
	return err
}

func (p *Postgres) KnowledgePointOverview(ctx context.Context, uid, kpid string) (o domain.KnowledgePointOverview, err error) {
	rows, err := p.DB.Query(ctx, `SELECT d.id,d.knowledge_base_id,d.user_id,d.filename,d.original_filename,d.mime_type,d.size,d.storage_path,d.resource_type,d.status,d.error_message,d.created_at,d.updated_at,d.progress,d.processing_stage,d.processed_chunks,d.total_chunks,COALESCE(d.active_revision_id::text,'') FROM documents d JOIN knowledge_bases kb ON kb.id=d.knowledge_base_id WHERE (d.user_id=$1 OR kb.is_public) AND EXISTS(SELECT 1 FROM exam408_resource_mappings m WHERE m.document_id=d.id AND m.knowledge_point_id=$2) ORDER BY d.updated_at DESC`, uid, kpid)
	if err != nil {
		return o, err
	}
	for rows.Next() {
		d, e := scanDoc(rows)
		if e != nil {
			rows.Close()
			return o, e
		}
		o.Resources = append(o.Resources, d)
	}
	rows.Close()
	var m domain.Mastery
	err = p.DB.QueryRow(ctx, `SELECT m.user_id,m.knowledge_point_id,k.name,s.name,m.mastery_score,m.weakness_score,m.confidence,m.interaction_count,m.last_interaction_at,m.updated_at FROM exam408_user_mastery m JOIN exam408_knowledge_points k ON k.id=m.knowledge_point_id JOIN exam408_subjects s ON s.id=k.subject_id WHERE m.user_id=$1 AND m.knowledge_point_id=$2`, uid, kpid).Scan(&m.UserID, &m.KnowledgePointID, &m.KnowledgePointName, &m.SubjectName, &m.MasteryScore, &m.WeaknessScore, &m.Confidence, &m.InteractionCount, &m.LastInteractionAt, &m.UpdatedAt)
	if err == nil {
		o.Mastery = &m
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return o, err
	}
	erows, err := p.DB.Query(ctx, `SELECT id,user_id,COALESCE(subject_id::text,''),COALESCE(knowledge_point_id::text,''),COALESCE(resource_id::text,''),event_type,metadata,created_at FROM exam408_learning_events WHERE user_id=$1 AND knowledge_point_id=$2 ORDER BY created_at DESC LIMIT 10`, uid, kpid)
	if err != nil {
		return o, err
	}
	defer erows.Close()
	for erows.Next() {
		var e domain.LearningEvent
		var raw []byte
		if err = erows.Scan(&e.ID, &e.UserID, &e.SubjectID, &e.KnowledgePointID, &e.ResourceID, &e.EventType, &raw, &e.CreatedAt); err != nil {
			return o, err
		}
		_ = json.Unmarshal(raw, &e.Metadata)
		o.RecentEvents = append(o.RecentEvents, e)
	}
	return o, erows.Err()
}

func (p *Postgres) CreateConversation(ctx context.Context, uid, kid, title string) (string, error) {
	cid := id()
	_, err := p.DB.Exec(ctx, `INSERT INTO conversations(id,user_id,knowledge_base_id,title) SELECT $1,$2,$3,$4 WHERE EXISTS(SELECT 1 FROM knowledge_bases WHERE id=$3 AND (user_id=$2 OR is_public=true))`, cid, uid, kid, title)
	return cid, err
}
func (p *Postgres) SaveMessage(ctx context.Context, uid, cid, role, content string, citations []domain.Citation) error {
	raw, _ := json.Marshal(citations)
	tag, err := p.DB.Exec(ctx, `INSERT INTO messages(id,conversation_id,user_id,role,content,citations) SELECT $1,$2,$3,$4,$5,$6 WHERE EXISTS(SELECT 1 FROM conversations WHERE id=$2 AND user_id=$3)`, id(), cid, uid, role, content, raw)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (p *Postgres) CreateLearningEvent(ctx context.Context, e domain.LearningEvent) (domain.LearningEvent, error) {
	e.ID = id()
	raw, _ := json.Marshal(e.Metadata)
	err := p.DB.QueryRow(ctx, `INSERT INTO exam408_learning_events(id,user_id,subject_id,knowledge_point_id,resource_id,event_type,metadata) VALUES($1,$2,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid,NULLIF($5,'')::uuid,$6,$7) RETURNING created_at`, e.ID, e.UserID, e.SubjectID, e.KnowledgePointID, e.ResourceID, e.EventType, raw).Scan(&e.CreatedAt)
	return e, err
}
func deltas(event string) (mastery, weakness, confidence float64) {
	switch event {
	case "quiz_correct":
		return .12, -.10, .12
	case "quiz_wrong":
		return -.04, .18, .15
	case "resource_complete":
		return .05, -.03, .06
	case "knowledge_point_view":
		return .01, 0, .02
	case "rag_question":
		return 0, .025, .03
	default:
		return 0, 0, .01
	}
}
func (p *Postgres) ApplyMastery(ctx context.Context, e domain.LearningEvent) error {
	// Passive interactions are recorded, but do not assert knowledge mastery.
	if e.EventType != "quiz_correct" && e.EventType != "quiz_wrong" {
		return nil
	}
	if e.KnowledgePointID == "" {
		return nil
	}
	m, w, c := deltas(e.EventType)
	_, err := p.DB.Exec(ctx, `INSERT INTO exam408_user_mastery(user_id,knowledge_point_id,mastery_score,weakness_score,confidence,interaction_count,last_interaction_at) VALUES($1,$2,GREATEST(0,$3),GREATEST(0,$4),GREATEST(0,$5),1,now()) ON CONFLICT(user_id,knowledge_point_id) DO UPDATE SET mastery_score=LEAST(1,GREATEST(0,exam408_user_mastery.mastery_score+$3)),weakness_score=LEAST(1,GREATEST(0,exam408_user_mastery.weakness_score+$4)),confidence=LEAST(1,exam408_user_mastery.confidence+$5),interaction_count=exam408_user_mastery.interaction_count+1,last_interaction_at=now(),updated_at=now()`, e.UserID, e.KnowledgePointID, m, w, c)
	return err
}
func (p *Postgres) ListMastery(ctx context.Context, uid string, weak bool) (out []domain.Mastery, err error) {
	q := `SELECT m.user_id,m.knowledge_point_id,k.name,s.name,m.mastery_score,m.weakness_score,m.confidence,m.interaction_count,m.last_interaction_at,m.updated_at FROM exam408_user_mastery m JOIN exam408_knowledge_points k ON k.id=m.knowledge_point_id JOIN exam408_subjects s ON s.id=k.subject_id WHERE m.user_id=$1`
	if weak {
		q += ` AND m.weakness_score>=0.1`
	}
	q += ` ORDER BY m.weakness_score DESC,m.updated_at DESC`
	rows, err := p.DB.Query(ctx, q, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m domain.Mastery
		if err = rows.Scan(&m.UserID, &m.KnowledgePointID, &m.KnowledgePointName, &m.SubjectName, &m.MasteryScore, &m.WeaknessScore, &m.Confidence, &m.InteractionCount, &m.LastInteractionAt, &m.UpdatedAt); err != nil {
			return
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (p *Postgres) ReplaceRecommendations(ctx context.Context, uid string, recs []domain.Recommendation) error {
	tx, err := p.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE exam408_recommendations SET status='expired' WHERE user_id=$1 AND status='active'`, uid); err != nil {
		return err
	}
	for _, r := range recs {
		_, err = tx.Exec(ctx, `INSERT INTO exam408_recommendations(id,user_id,knowledge_point_id,score,reason,recommended_action) VALUES($1,$2,$3,$4,$5,$6)`, id(), uid, r.KnowledgePointID, r.Score, r.Reason, r.RecommendedAction)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (p *Postgres) ListRecommendations(ctx context.Context, uid string) (out []domain.Recommendation, err error) {
	rows, err := p.DB.Query(ctx, `SELECT r.id,r.user_id,r.knowledge_point_id,k.name,s.name,r.score,r.reason,r.recommended_action,r.status,r.generated_at,r.expires_at FROM exam408_recommendations r JOIN exam408_knowledge_points k ON k.id=r.knowledge_point_id JOIN exam408_subjects s ON s.id=k.subject_id WHERE r.user_id=$1 AND r.status='active' AND r.expires_at>now() ORDER BY r.score DESC LIMIT 10`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r domain.Recommendation
		if err = rows.Scan(&r.ID, &r.UserID, &r.KnowledgePointID, &r.KnowledgePointName, &r.SubjectName, &r.Score, &r.Reason, &r.RecommendedAction, &r.Status, &r.GeneratedAt, &r.ExpiresAt); err != nil {
			return
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (p *Postgres) Dashboard(ctx context.Context, uid string) (d domain.Dashboard, err error) {
	if err = p.DB.QueryRow(ctx, `SELECT count(*) FROM documents WHERE user_id=$1 AND (status='ready' OR active_revision_id IS NOT NULL)`, uid).Scan(&d.ReadyDocumentCount); err != nil {
		return d, err
	}
	if err = p.DB.QueryRow(ctx, `SELECT count(*) FROM knowledge_bases WHERE user_id=$1`, uid).Scan(&d.KnowledgeBaseCount); err != nil {
		return d, err
	}
	d.Recommendations, err = p.ListRecommendations(ctx, uid)
	if err != nil {
		return d, err
	}
	d.Weaknesses, err = p.ListMastery(ctx, uid, true)
	if err != nil {
		return d, err
	}
	if len(d.Weaknesses) > 5 {
		d.Weaknesses = d.Weaknesses[:5]
	}
	d.RecentDocuments, err = p.ListDocuments(ctx, uid, "")
	if err != nil {
		return d, err
	}
	if len(d.RecentDocuments) > 5 {
		d.RecentDocuments = d.RecentDocuments[:5]
	}
	rows, e := p.DB.Query(ctx, `SELECT s.id,s.code,s.name,count(k.id),count(m.knowledge_point_id),COALESCE(avg(m.mastery_score),0) FROM exam408_subjects s LEFT JOIN exam408_knowledge_points k ON k.subject_id=s.id LEFT JOIN exam408_user_mastery m ON m.knowledge_point_id=k.id AND m.user_id=$1 GROUP BY s.id ORDER BY s.sort_order`, uid)
	if e != nil {
		return d, e
	}
	defer rows.Close()
	for rows.Next() {
		var x domain.SubjectProgress
		if e = rows.Scan(&x.ID, &x.Code, &x.Name, &x.KnowledgePointCount, &x.TouchedCount, &x.Progress); e != nil {
			return d, e
		}
		d.Subjects = append(d.Subjects, x)
	}
	erows, err := p.DB.Query(ctx, `SELECT id,user_id,COALESCE(subject_id::text,''),COALESCE(knowledge_point_id::text,''),COALESCE(resource_id::text,''),event_type,metadata,created_at FROM exam408_learning_events WHERE user_id=$1 ORDER BY created_at DESC LIMIT 8`, uid)
	if err != nil {
		return d, err
	}
	defer erows.Close()
	for erows.Next() {
		var ev domain.LearningEvent
		var raw []byte
		if err = erows.Scan(&ev.ID, &ev.UserID, &ev.SubjectID, &ev.KnowledgePointID, &ev.ResourceID, &ev.EventType, &raw, &ev.CreatedAt); err != nil {
			return d, err
		}
		if err = json.Unmarshal(raw, &ev.Metadata); err != nil {
			return d, err
		}
		d.RecentEvents = append(d.RecentEvents, ev)
	}
	return d, erows.Err()
}

var _ domain.Store = (*Postgres)(nil)
var _ = time.Now
