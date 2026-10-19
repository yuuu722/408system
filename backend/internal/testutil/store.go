package testutil

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"math"
	"sort"
	"sync"
	"time"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	Mu        sync.Mutex
	Users     map[string]domain.User
	KBs       map[string]domain.KnowledgeBase
	Docs      map[string]domain.Document
	Chunks    []domain.Chunk
	Subjects  []domain.Subject
	Points    []domain.KnowledgePoint
	Relations []domain.PointRelation
	Events    []domain.LearningEvent
	Masteries map[string]domain.Mastery
	Recs      map[string][]domain.Recommendation
	Messages  []string
}

func NewStore() *Store {
	s := &Store{Users: map[string]domain.User{}, KBs: map[string]domain.KnowledgeBase{}, Docs: map[string]domain.Document{}, Masteries: map[string]domain.Mastery{}, Recs: map[string][]domain.Recommendation{}}
	sub := domain.Subject{ID: "s1", Code: "OS", Name: "操作系统"}
	s.Subjects = []domain.Subject{sub}
	s.Points = []domain.KnowledgePoint{{ID: "p1", SubjectID: "s1", Code: "OS.MEMORY", Name: "虚拟内存", Description: "分页与虚拟内存", Keywords: []string{"虚拟内存", "页面置换"}, SubjectName: "操作系统"}, {ID: "p2", SubjectID: "s1", Code: "OS.PROCESS", Name: "进程", Keywords: []string{"进程"}, SubjectName: "操作系统"}}
	s.Relations = []domain.PointRelation{{ID: "r1", FromPointID: "p2", ToPointID: "p1", RelationType: "prerequisite"}}
	return s
}
func newid() string { return uuid.NewString() }
func (s *Store) CreateUser(_ context.Context, u domain.User) (domain.User, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	for _, x := range s.Users {
		if x.Username == u.Username || x.Email == u.Email {
			return u, errors.New("duplicate")
		}
	}
	if u.ID == "" {
		u.ID = newid()
	}
	u.CreatedAt = time.Now()
	s.Users[u.ID] = u
	return u, nil
}
func (s *Store) UserByLogin(_ context.Context, l string) (domain.User, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	for _, u := range s.Users {
		if u.Username == l || u.Email == l {
			return u, nil
		}
	}
	return domain.User{}, ErrNotFound
}
func (s *Store) UserByID(_ context.Context, id string) (domain.User, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	u, ok := s.Users[id]
	if !ok {
		return u, ErrNotFound
	}
	return u, nil
}
func (s *Store) CreateKnowledgeBase(_ context.Context, uid, n, d string) (domain.KnowledgeBase, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	k := domain.KnowledgeBase{ID: newid(), UserID: uid, Name: n, Description: d, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	s.KBs[k.ID] = k
	return k, nil
}
func (s *Store) ListKnowledgeBases(_ context.Context, uid string) (o []domain.KnowledgeBase, e error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	for _, k := range s.KBs {
		if k.UserID == uid {
			o = append(o, k)
		}
	}
	return
}
func (s *Store) KnowledgeBase(_ context.Context, uid, id string) (domain.KnowledgeBase, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	k, ok := s.KBs[id]
	if !ok || k.UserID != uid {
		return k, ErrNotFound
	}
	return k, nil
}
func (s *Store) DeleteKnowledgeBase(_ context.Context, uid, id string) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	k, ok := s.KBs[id]
	if !ok || k.UserID != uid {
		return ErrNotFound
	}
	delete(s.KBs, id)
	return nil
}
func (s *Store) CreateDocument(_ context.Context, d domain.Document) (domain.Document, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	k, ok := s.KBs[d.KnowledgeBaseID]
	if !ok || k.UserID != d.UserID {
		return d, ErrNotFound
	}
	if d.ID == "" {
		d.ID = newid()
	}
	d.CreatedAt = time.Now()
	s.Docs[d.ID] = d
	return d, nil
}
func (s *Store) ListDocuments(_ context.Context, uid, kid string) (o []domain.Document, e error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	for _, d := range s.Docs {
		if d.UserID == uid && (kid == "" || d.KnowledgeBaseID == kid) {
			o = append(o, d)
		}
	}
	return
}
func (s *Store) Document(_ context.Context, uid, id string) (domain.Document, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	d, ok := s.Docs[id]
	if !ok || d.UserID != uid {
		return d, ErrNotFound
	}
	return d, nil
}
func (s *Store) UpdateDocumentStatus(_ context.Context, id, status, msg string) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	d := s.Docs[id]
	d.Status = status
	d.ErrorMessage = msg
	if status == "processing" {
		d.Progress = 0
		d.ProcessingStage = "queued"
		d.ProcessedChunks = 0
		d.TotalChunks = 0
	}
	if status == "ready" {
		d.Progress = 100
		d.ProcessingStage = "completed"
	}
	s.Docs[id] = d
	return nil
}
func (s *Store) DeleteDocument(_ context.Context, uid, id string) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	d, ok := s.Docs[id]
	if !ok || d.UserID != uid {
		return ErrNotFound
	}
	delete(s.Docs, id)
	return nil
}

func (s *Store) UpdateDocumentProgress(_ context.Context, id, stage string, progress, done, total int) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	d := s.Docs[id]
	d.ProcessingStage = stage
	d.Progress = progress
	d.ProcessedChunks = done
	d.TotalChunks = total
	s.Docs[id] = d
	return nil
}
func (s *Store) ReplaceChunks(_ context.Context, uid, did string, c []domain.Chunk) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	var keep []domain.Chunk
	for _, x := range s.Chunks {
		if !(x.UserID == uid && x.DocumentID == did) {
			keep = append(keep, x)
		}
	}
	s.Chunks = append(keep, c...)
	return nil
}
func cosine(a, b []float32) float64 {
	var dot, aa, bb float64
	for i := 0; i < len(a) && i < len(b); i++ {
		dot += float64(a[i] * b[i])
		aa += float64(a[i] * a[i])
		bb += float64(b[i] * b[i])
	}
	if aa*bb == 0 {
		return 0
	}
	return dot / math.Sqrt(aa*bb)
}
func (s *Store) SearchChunks(_ context.Context, uid, kid string, v []float32, n int) (o []domain.Chunk, e error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	for _, c := range s.Chunks {
		d, exists := s.Docs[c.DocumentID]
		if c.UserID == uid && c.KnowledgeBaseID == kid && exists && d.Status == "ready" {
			c.Score = cosine(c.Embedding, v)
			o = append(o, c)
		}
	}
	sort.Slice(o, func(i, j int) bool { return o[i].Score > o[j].Score })
	if len(o) > n {
		o = o[:n]
	}
	return
}
func (s *Store) ListSubjects(context.Context) ([]domain.Subject, error) { return s.Subjects, nil }
func (s *Store) ListKnowledgePoints(context.Context) ([]domain.KnowledgePoint, error) {
	return s.Points, nil
}
func (s *Store) KnowledgePoint(_ context.Context, id string) (domain.KnowledgePoint, error) {
	for _, p := range s.Points {
		if p.ID == id {
			return p, nil
		}
	}
	return domain.KnowledgePoint{}, ErrNotFound
}
func (s *Store) PointRelations(_ context.Context, id string) (o []domain.PointRelation, e error) {
	for _, r := range s.Relations {
		if r.FromPointID == id || r.ToPointID == id {
			o = append(o, r)
		}
	}
	return
}
func (s *Store) MapDocumentToPoint(context.Context, string, string, string) error { return nil }
func (s *Store) KnowledgePointOverview(ctx context.Context, uid, kid string) (domain.KnowledgePointOverview, error) {
	m, _ := s.ListMastery(ctx, uid, false)
	o := domain.KnowledgePointOverview{}
	for i := range m {
		if m[i].KnowledgePointID == kid {
			o.Mastery = &m[i]
		}
	}
	for _, e := range s.Events {
		if e.UserID == uid && e.KnowledgePointID == kid {
			o.RecentEvents = append(o.RecentEvents, e)
		}
	}
	return o, nil
}
func (s *Store) CreateConversation(context.Context, string, string, string) (string, error) {
	return newid(), nil
}
func (s *Store) SaveMessage(_ context.Context, uid, cid, role, content string, _ []domain.Citation) error {
	s.Messages = append(s.Messages, uid+cid+role+content)
	return nil
}
func (s *Store) CreateLearningEvent(_ context.Context, e domain.LearningEvent) (domain.LearningEvent, error) {
	e.ID = newid()
	e.CreatedAt = time.Now()
	s.Events = append(s.Events, e)
	return e, nil
}
func (s *Store) ApplyMastery(_ context.Context, e domain.LearningEvent) error {
	if e.KnowledgePointID == "" {
		return nil
	}
	key := e.UserID + e.KnowledgePointID
	m := s.Masteries[key]
	m.UserID = e.UserID
	m.KnowledgePointID = e.KnowledgePointID
	m.KnowledgePointName = "虚拟内存"
	m.SubjectName = "操作系统"
	switch e.EventType {
	case "quiz_wrong":
		m.WeaknessScore += .18
		m.Confidence += .15
	case "quiz_correct":
		m.MasteryScore += .12
		m.WeaknessScore -= .1
	case "resource_complete":
		m.MasteryScore += .05
	case "rag_question":
		m.WeaknessScore += .025
	}
	if m.WeaknessScore < 0 {
		m.WeaknessScore = 0
	}
	m.InteractionCount++
	now := time.Now()
	m.LastInteractionAt = &now
	s.Masteries[key] = m
	return nil
}
func (s *Store) ListMastery(_ context.Context, uid string, weak bool) (o []domain.Mastery, e error) {
	for _, m := range s.Masteries {
		if m.UserID == uid && (!weak || m.WeaknessScore >= .1) {
			o = append(o, m)
		}
	}
	sort.Slice(o, func(i, j int) bool { return o[i].WeaknessScore > o[j].WeaknessScore })
	return
}
func (s *Store) ReplaceRecommendations(_ context.Context, uid string, r []domain.Recommendation) error {
	s.Recs[uid] = r
	return nil
}
func (s *Store) ListRecommendations(_ context.Context, uid string) ([]domain.Recommendation, error) {
	r := s.Recs[uid]
	for i := range r {
		r[i].KnowledgePointName = "虚拟内存"
		r[i].SubjectName = "操作系统"
	}
	return r, nil
}
func (s *Store) Dashboard(ctx context.Context, uid string) (d domain.Dashboard, e error) {
	d.Recommendations, _ = s.ListRecommendations(ctx, uid)
	d.Weaknesses, _ = s.ListMastery(ctx, uid, true)
	d.RecentDocuments, _ = s.ListDocuments(ctx, uid, "")
	d.KnowledgeBaseCount = len(s.KBs)
	return
}

var _ domain.Store = (*Store)(nil)
