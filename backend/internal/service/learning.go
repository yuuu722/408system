package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"sort"
	"strings"
	"time"
)

type KnowledgeService struct{ Store domain.Store }

func (s KnowledgeService) Detect(ctx context.Context, text string) ([]domain.KnowledgePoint, error) {
	points, err := s.Store.ListKnowledgePoints(ctx)
	if err != nil {
		return nil, err
	}
	lower := strings.ToLower(text)
	type scored struct {
		p domain.KnowledgePoint
		n int
	}
	var hits []scored
	for _, p := range points {
		n := 0
		for _, k := range append(p.Keywords, p.Name) {
			if k != "" && strings.Contains(lower, strings.ToLower(k)) {
				n++
			}
		}
		if n > 0 {
			hits = append(hits, scored{p, n})
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].n > hits[j].n })
	if len(hits) > 3 {
		hits = hits[:3]
	}
	out := make([]domain.KnowledgePoint, len(hits))
	for i, h := range hits {
		out[i] = h.p
	}
	return out, nil
}

type LearningService struct{ Store domain.Store }

var ErrUntrustedLearningEvent = errors.New("learning event requires a trusted server workflow")

func (s LearningService) RecordUserEvent(ctx context.Context, e domain.LearningEvent) (domain.LearningEvent, error) {
	if !domain.UserRecordableEvents[e.EventType] {
		return e, fmt.Errorf("%w: %q", ErrUntrustedLearningEvent, e.EventType)
	}
	return s.Record(ctx, e)
}

func (s LearningService) Record(ctx context.Context, e domain.LearningEvent) (domain.LearningEvent, error) {
	if !domain.AllowedEvents[e.EventType] {
		return e, fmt.Errorf("unsupported event type")
	}
	saved, err := s.Store.CreateLearningEvent(ctx, e)
	if err != nil {
		return saved, err
	}
	if err = s.Store.ApplyMastery(ctx, saved); err != nil {
		return saved, err
	}
	return saved, nil
}

type RecommendationService struct{ Store domain.Store }

func (s RecommendationService) Generate(ctx context.Context, uid string) ([]domain.Recommendation, error) {
	if planner, ok := s.Store.(interface {
		GenerateTasks(context.Context, string) error
		PlanRecommendations(context.Context, string) ([]domain.Recommendation, error)
	}); ok {
		if e := planner.GenerateTasks(ctx, uid); e != nil {
			return nil, e
		}
		return planner.PlanRecommendations(ctx, uid)
	}
	mastery, err := s.Store.ListMastery(ctx, uid, false)
	if err != nil {
		return nil, err
	}
	recs := make([]domain.Recommendation, 0, len(mastery))
	for _, m := range mastery {
		score := .55*m.WeaknessScore + .25*(1-m.MasteryScore) + .20*m.Confidence
		if score < .12 {
			continue
		}
		reason := fmt.Sprintf("%s当前掌握度 %.0f%%、薄弱度 %.0f%%，基于 %d 次学习交互。", m.KnowledgePointName, m.MasteryScore*100, m.WeaknessScore*100, m.InteractionCount)
		action := "review_material"
		if m.WeaknessScore > .45 {
			action = "practice_and_review"
		}
		recs = append(recs, domain.Recommendation{UserID: uid, KnowledgePointID: m.KnowledgePointID, Score: score, Reason: reason, RecommendedAction: action, Status: "active", GeneratedAt: time.Now(), ExpiresAt: time.Now().Add(7 * 24 * time.Hour)})
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].Score > recs[j].Score })
	if len(recs) > 10 {
		recs = recs[:10]
	}
	if err = s.Store.ReplaceRecommendations(ctx, uid, recs); err != nil {
		return nil, err
	}
	return s.Store.ListRecommendations(ctx, uid)
}
