package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSourceSelectionNeverPublishesInventedParaphrases(t *testing.T) {
	content := []string{"虚拟内存利用程序的局部性，允许程序不必全部装入内存即可运行。页框属于物理内存。", "虚拟性是从逻辑上扩充内存容量。"}
	refs := []domain.Citation{{DocumentID: "book", Page: 12}, {DocumentID: "book2", Page: 13}}
	answer, sources, ok := selectedSourceAnswer(`{"passages":[{"source":2,"quote":"虚拟性是从逻辑上扩充内存容量。"},{"source":1,"quote":"虚拟内存利用程序的局部性，允许程序不必全部装入内存即可运行。"}]}`, content, refs)
	require.True(t, ok)
	require.NotContains(t, answer, "[1]")
	require.NotContains(t, answer, "根据资料")
	require.Equal(t, "book2", sources[0].DocumentID)
	require.Len(t, sources, 2)
	for _, raw := range []string{
		`{"passages":[{"source":1,"quote":"虚拟内存将数据放到磁盘上的页框中。"}]}`,
		`{"passages":[{"source":99,"quote":"虚拟性是从逻辑上扩充内存容量。"}]}`,
		`虚拟内存将数据放到磁盘上的页框中。[1]`,
		`{"passages":[]}`,
	} {
		body, _, valid := selectedSourceAnswer(raw, content, refs)
		require.False(t, valid)
		require.Empty(t, body)
	}
	fallback, sources := sourceExcerptAnswer(content, refs)
	require.NotContains(t, fallback, "磁盘上的页框")
	require.NotContains(t, fallback, "[1]")
	require.Len(t, sources, 2)
}
