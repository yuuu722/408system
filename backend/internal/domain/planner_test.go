package domain

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestPlannerEvidenceAndDurations(t *testing.T) {
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	assets := []Asset{{ID: "q", Kind: "question", PointID: "p", Status: "ready", Payload: AssetPayload{FamilyID: "family", Difficulty: 1}},
		{ID: "video", Kind: "video", PointID: "p", Status: "ready", Payload: AssetPayload{StartSeconds: 10, EndSeconds: 131}},
		{ID: "unavailable", Kind: "question", PointID: "p", Status: "unavailable"}}
	cold := PlanCandidates(assets, nil, Profile{}, now)
	require.Len(t, cold, 2)
	require.Contains(t, cold[0].Reason, "待测")
	for _, c := range cold {
		if c.Asset.Kind == "video" {
			require.Equal(t, 3, c.Minutes)
			require.Equal(t, "video_segment", c.DurationSource)
		}
	}
	history := []ReviewEvidence{{FamilyID: "family", PointID: "p", Correct: true, At: now.Add(-time.Hour)}}
	recent := PlanCandidates(assets, history, Profile{}, now)
	require.Len(t, recent, 1)
	require.Equal(t, "video", recent[0].Asset.ID)
	history[0].Correct = false
	remedial := PlanCandidates(assets, history, Profile{}, now)
	require.Len(t, remedial, 2)
	require.Equal(t, "q", remedial[0].Asset.ID)
	history[0].Correct = true
	history[0].At = now.Add(-48 * time.Hour)
	due := PlanCandidates(assets, history, Profile{}, now)
	require.Len(t, due, 2)
	require.Contains(t, due[0].Reason, "规则复习")
}
