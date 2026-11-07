package domain

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestQuestionFamilyNormalizesLayoutButNotMeaningOrOptions(t *testing.T) {
	a := Asset{ID: "a", IsPublic: true, Kind: "question", Status: "ready", Payload: AssetPayload{Stem: "操作系统\n管理什么？", Options: []string{"系统资源", "其它"}, Answer: 0}}
	b := a
	b.ID = "b"
	b.Payload.Stem = "操作系统管理什么？"
	b.Payload.Images = []string{}
	require.Equal(t, QuestionContentFamily(a), QuestionContentFamily(b))
	b.Payload.Options = []string{"其它", "系统资源"}
	require.NotEqual(t, QuestionContentFamily(a), QuestionContentFamily(b))
	b = a
	b.Payload.Stem = "操作系统不管理什么？"
	require.NotEqual(t, QuestionContentFamily(a), QuestionContentFamily(b))
	b = a
	b.Payload.Stem = "ab c"
	a.Payload.Stem = "a bc"
	require.NotEqual(t, QuestionContentFamily(a), QuestionContentFamily(b))
	a.Payload.Stem = "如图所示的系统结构"
	require.Empty(t, QuestionContentFamily(a))
}
func TestPublicVariantsCollapseWithoutRemovingSourceReferences(t *testing.T) {
	a := Asset{ID: "a", IsPublic: true, Kind: "question", Status: "ready", Payload: AssetPayload{Stem: "操作系统管理什么？", Options: []string{"资源", "其它"}, DocumentID: "doc-a", Page: 2}}
	a.Payload.FamilyID = QuestionContentFamily(a)
	b := a
	b.ID = "b"
	b.Payload.DocumentID = "doc-b"
	private := b
	private.ID = "private"
	private.IsPublic = false
	rows := CollapsePublicQuestionVariants([]Asset{a, b, private})
	require.Len(t, rows, 2)
	require.Len(t, rows[0].Payload.VariantSources, 1)
	require.Equal(t, "doc-b", rows[0].Payload.VariantSources[0].DocumentID)
	require.Equal(t, "private", rows[1].ID)
}

func TestStaleFamilyCannotHideChangedQuestionOrGrade(t *testing.T) {
	a := Asset{ID: "a", IsPublic: true, Kind: "question", Status: "ready", Payload: AssetPayload{Stem: "操作系统管理什么？", Options: []string{"资源", "其它"}, Answer: 0}}
	a.Payload.FamilyID = QuestionContentFamily(a)
	b := a
	b.ID = "b"
	b.Payload.Answer = 1
	c := a
	c.ID = "c"
	c.Payload.Stem = "操作系统不管理什么？"
	require.Len(t, CollapsePublicQuestionVariants([]Asset{a, b, c}), 3)
}
