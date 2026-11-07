package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPaperScopesKeepRepeatedNumbersSeparateAndIgnoreIP(t *testing.T) {
	for _, h := range []string{"10.1.0.0/16 → 接口 2", "192.168.10.33/27 所在子网"} {
		_, _, ok := questionHeading(h)
		require.False(t, ok)
	}
	pages := []TextPage{{Number: 2, Segment: 1, Text: "第 1 套：模拟卷一\n1. 操作系统管理什么？\nA. 资源 B. 日期\n10.1.0.0/16 → 接口 2"},
		{Number: 3, Segment: 2, Text: "第 2 套：模拟卷二\n1. 操作系统通过什么提供服务？\nA. 日期 B. 系统调用"},
		{Number: 4, Segment: 3, Text: "模拟卷一 参考答案与要点\n1. A\n解析：操作系统负责管理系统资源，为应用程序提供运行环境。\n模拟卷二 参考答案与要点\n1. B\n解析：应用程序通过系统调用使用操作系统提供的服务。"}}
	assets := ExtractQuestions(domain.Document{ID: "q", OriginalFilename: "三套模拟卷.pdf"}, pages, []domain.KnowledgePoint{{ID: "p", Name: "操作系统"}})
	require.Len(t, assets, 2)
	require.Equal(t, "试卷:1", assets[0].Payload.SectionKey)
	require.Equal(t, "试卷:2", assets[1].Payload.SectionKey)
	require.Equal(t, 0, assets[0].Payload.Answer)
	require.Equal(t, 1, assets[1].Payload.Answer)
	require.Contains(t, assets[0].Payload.Explanation, "管理系统资源")
	require.Contains(t, assets[1].Payload.Explanation, "系统调用")
	entries := ParseAnswerFile(pages[2:])
	require.Len(t, entries, 2)
	require.Equal(t, "试卷:1", entries[0].Section)
	require.Equal(t, "试卷:2", entries[1].Section)
}
func TestCompactAnswersNeverReadNumericOptionsAsAnswerKeys(t *testing.T) {
	pages := []TextPage{{Number: 1, Text: "在定点补码表示中，8位补码表示的数是（ ）。A. -128 B. -1 C. 127 D. 255\n某个字包含（ ）。A. 2 字节 B. 4 字节 C. 8 字节 D. 32 字节"}}
	require.Empty(t, ParseAnswerFile(pages))
	entries := ParseAnswerFile([]TextPage{{Number: 2, Text: "参考答案\n1. A 2. B 3. C"}})
	require.Len(t, entries, 3)
}
