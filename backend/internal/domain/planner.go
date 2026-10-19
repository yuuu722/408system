package domain

import (
	"fmt"
	"sort"
	"time"
)

type ReviewEvidence struct {
	FamilyID, PointID string
	Correct           bool
	At                time.Time
}
type PlanCandidate struct {
	Asset                  Asset
	Minutes                int
	DurationSource, Reason string
	Priority               float64
}
type DailyPlan struct {
	ID, Date, Strategy              string
	BudgetMinutes, AllocatedMinutes int
}

func Family(a Asset) string {
	if a.Payload.FamilyID != "" {
		return a.Payload.FamilyID
	}
	return a.ID
}

// A transparent policy, not a trained ability or forgetting model.
func PlanCandidates(assets []Asset, history []ReviewEvidence, p Profile, now time.Time) []PlanCandidate {
	sort.SliceStable(history, func(i, j int) bool { return history[i].At.Before(history[j].At) })
	last := map[string]ReviewEvidence{}
	streak := map[string]int{}
	for _, v := range history {
		last[v.FamilyID] = v
		if v.Correct {
			streak[v.FamilyID]++
		} else {
			streak[v.FamilyID] = 0
		}
	}
	tested, correct := map[string]int{}, map[string]int{}
	for _, v := range last {
		tested[v.PointID]++
		if v.Correct {
			correct[v.PointID]++
		}
	}
	intervals := []int{1, 3, 7, 14, 30}
	out := []PlanCandidate{}
	for _, a := range assets {
		if a.Status != "ready" || a.PointID == "" {
			continue
		}
		priority := 20.0
		reason := "知识点尚待测，安排基础学习或诊断练习。"
		if n := tested[a.PointID]; n > 0 {
			accuracy := float64(correct[a.PointID]) / float64(n)
			priority = 30 + 40*(1-accuracy)
			reason = fmt.Sprintf("该知识点有效题族最近作答正确率 %.0f%%，优先补充薄弱内容。", accuracy*100)
		}
		if a.Kind == "question" {
			if review, ok := last[Family(a)]; ok {
				days := 0
				if review.Correct {
					days = intervals[min(max(streak[Family(a)]-1, 0), len(intervals)-1)]
				}
				if now.Before(review.At.Add(time.Duration(days) * 24 * time.Hour)) {
					continue
				}
				priority += 25
				reason = "已到规则复习时间；间隔基于有效作答结果，不代表遗忘概率。"
			}
			if a.Payload.DifficultyOrigin == "user" {
				target := 2
				if p.Stage == "sprint" {
					target = 4
				} else if p.Stage == "review" {
					target = 3
				}
				if tested[a.PointID] > 0 && correct[a.PointID]*2 < tested[a.PointID] {
					target = 1
				}
				distance := a.Payload.Difficulty - target
				if distance < 0 {
					distance = -distance
				}
				priority -= float64(distance) * 4
			} else {
				reason += " 题目难度尚未校准。"
			}
		}
		if a.Kind == "video" && p.Preference == "video" || a.Kind == "material" && p.Preference == "text" {
			priority += 5
		}
		minutes, source := 10, "default_estimate"
		if a.Kind == "question" {
			minutes = 5
		}
		if a.Kind == "video" {
			minutes = 15
			if a.Payload.EndSeconds > a.Payload.StartSeconds {
				minutes = (a.Payload.EndSeconds - a.Payload.StartSeconds + 59) / 60
				source = "video_segment"
			}
		}
		if a.Payload.EstimatedMinutes > 0 {
			minutes = a.Payload.EstimatedMinutes
			source = "user_estimate"
		}
		out = append(out, PlanCandidate{Asset: a, Minutes: minutes, DurationSource: source, Reason: reason, Priority: priority})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority == out[j].Priority {
			return out[i].Asset.ID < out[j].Asset.ID
		}
		return out[i].Priority > out[j].Priority
	})
	return out
}
