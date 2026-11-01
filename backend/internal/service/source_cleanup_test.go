package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCleanedPageOffsetsAndBlockIDsAreStable(t *testing.T) {
	p := TextPage{Number: 225, Segment: 0, Blocks: []domain.DocumentBlock{{ID: "body", Text: "虚 拟 内 存按需装入。\nSeas 唯一官方淘宝店\n地址 = x + y"}, {ID: "footer", Text: "第225页"}}}
	CanonicalPage(&p)
	require.Equal(t, "虚拟内存按需装入。\n地址 = x + y", p.Text)
	require.Equal(t, 0, p.Blocks[0].Start)
	require.Equal(t, len([]rune(p.Text)), p.Blocks[0].End)
	old := p.Blocks[0].ID
	CanonicalPage(&p)
	require.Equal(t, old, p.Blocks[0].ID)
	for _, c := range MappedChunks(p, "rev", "space") {
		start := c.Metadata["start"].(int)
		end := c.Metadata["end"].(int)
		require.Equal(t, c.Content, string([]rune(p.Text)[start:end]))
	}
}
