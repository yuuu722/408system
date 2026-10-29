package domain

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPublishingCannotBypassCompletenessOrSourceAnswer(t *testing.T) {
	a := Asset{Kind: "question", PointID: "p", Payload: AssetPayload{Stem: "测试题", Source: "合成参考答案来源", Options: []string{"甲", "乙"}, Answer: 0, AnswerOrigin: "source", ReviewStatus: "pending"}}
	PrepareAsset(&a)
	require.Equal(t, "unavailable", a.Status)
	a.Payload.ReviewStatus = "published"
	PrepareAsset(&a)
	require.Equal(t, "ready", a.Status)
	a.Payload.AnswerOrigin = "ai_pending"
	a.Payload.Answer = -1
	PrepareAsset(&a)
	require.Equal(t, "unavailable", a.Status)
	require.Contains(t, a.Payload.Issue, "可靠答案")
	a.Payload.Issue = ""
	a.Payload.AnswerOrigin = "source"
	a.Payload.Answer = 0
	a.Payload.Options = []string{"甲", "甲"}
	PrepareAsset(&a)
	require.Equal(t, "unavailable", a.Status)
}
