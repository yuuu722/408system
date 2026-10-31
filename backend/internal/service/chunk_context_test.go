package service

import (
	"encoding/json"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestContextKeepsPremisesAndStopsAtNewSection(t *testing.T) {
	pages := []TextPage{{Number: 1, Segment: 0, Text: "第三章 内存管理\n3.1 请求分页\n假设进程分配的页框数固定。"}, {Number: 2, Segment: 1, Text: "在该条件下，请求分页通过页面置换处理缺页。"}, {Number: 3, Segment: 2, Text: "第四章 文件管理\n文件管理用于组织文件和目录。"}}
	for i := range pages {
		CanonicalPage(&pages[i])
	}
	chunks := ContextualChunks(pages, "r", "space", "操作系统教材.pdf")
	require.Len(t, chunks, 3)
	require.Contains(t, chunks[1].Metadata["heading_path"], "第三章")
	raw, _ := json.Marshal(chunks[1].Metadata["context_passages"])
	var context []ChunkContext
	require.NoError(t, json.Unmarshal(raw, &context))
	require.NotEmpty(t, context)
	for _, c := range context {
		source := pages[c.Segment]
		require.Equal(t, c.Text, string([]rune(source.Text)[c.Start:c.End]))
	}
	third := premiseChunks(chunks[2])
	for _, c := range third {
		require.NotContains(t, c.Content, "页框")
	}
	require.Equal(t, pages[1].Text, chunks[1].Content, "context must not corrupt original source text")
	text := strings.Repeat("这是完整句子，用于验证语义边界。", 150)
	page := TextPage{Number: 4, Text: text}
	CanonicalPage(&page)
	parts := MappedChunks(page, "r", "s")
	require.Greater(t, len(parts), 1)
	for _, c := range parts {
		require.LessOrEqual(t, len([]rune(c.Content)), 900)
		require.Equal(t, c.Content, string([]rune(page.Text)[c.Metadata["start"].(int):c.Metadata["end"].(int)]))
	}
}
func TestAnswerCannotDropRequiredCondition(t *testing.T) {
	contents := []string{"假设进程采用局部置换且页框数量固定。", "请求分页通过页面置换处理缺页。"}
	citations := []domain.Citation{{ChunkID: "a"}, {ChunkID: "b"}}
	_, _, ok := selectedSourceAnswer(`{"passages":[{"source":2,"quote":"请求分页通过页面置换处理缺页。"}]}`, contents, citations, map[int][]int{2: {1}})
	require.False(t, ok)
	_, _, ok = selectedSourceAnswer(`{"passages":[{"source":1,"quote":"假设进程采用局部置换且页框数量固定。"},{"source":2,"quote":"请求分页通过页面置换处理缺页。"}]}`, contents, citations, map[int][]int{2: {1}})
	require.True(t, ok)
}

func TestTooManyPremisesAreFlaggedInsteadOfSilentlyDropped(t *testing.T) {
	page := TextPage{Number: 1, Text: "假设页框数量固定。若采用局部置换。已知进程不共享页面。如果访问序列固定。对于每个进程记录缺页次数。"}
	CanonicalPage(&page)
	chunks := ContextualChunks([]TextPage{page}, "r", "space", "教材")
	require.NotEmpty(t, chunks)
	require.Equal(t, true, chunks[0].Metadata["context_incomplete"])
}
func TestParenthesizedQuestionsAndFullwidthOptions(t *testing.T) {
	assets := ExtractQuestions(domain.Document{ID: "d", OriginalFilename: "习题.pdf"}, []TextPage{{Number: 1, Text: "（1） 下列关于内存哪个正确？\nＡ. 第一项\nＢ. 第二项\n答案：A"}}, []domain.KnowledgePoint{{ID: "p", Name: "内存"}})
	require.Len(t, assets, 1)
	require.Len(t, assets[0].Payload.Options, 2)
	require.Equal(t, 0, assets[0].Payload.Answer)
}
