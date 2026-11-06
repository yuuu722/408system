package service

import (
	"context"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/testutil"
	"github.com/stretchr/testify/require"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// Opt-in, local-only: exercises the real model, catalog, support review and
// citation mapping with synthetic source text, never a user's uploaded corpus.
func TestLocalOllamaFactAnswerEndToEnd(t *testing.T) {
	endpoint := os.Getenv("OLLAMA_TEST_URL")
	if endpoint == "" {
		t.Skip("local Ollama opt-in")
	}
	u, e := url.Parse(endpoint)
	require.NoError(t, e)
	require.Contains(t, []string{"127.0.0.1", "localhost"}, u.Hostname())
	model := os.Getenv("OLLAMA_TEST_MODEL")
	if model == "" {
		model = "qwen2.5:3b"
	}
	for _, tc := range []struct{ q, text, heading string }{
		{"比较进程与线程的区别", "进程是资源分配的基本单位。线程是处理机调度与执行的基本单位。每个进程通常具有独立的虚拟地址空间。同一进程的线程共享地址空间，各自有独立的栈。", "第二章 / 进程与线程的比较"},
		{"为什么要用PPP？", "PPP用于点到点链路，支持多种网络层协议的数据传输，并提供建立、配置和测试链路的功能。PPP信息部分的长度受最大传送单元MTU的限制。", "第三章 / PPP协议的用途"},
		{"TCP如何实现可靠传输？", "TCP使用序号标识字节在数据流中的位置，接收方据此按序重组数据并识别重复数据。TCP接收方使用确认号告知发送方下一步期望接收的字节。TCP发送方在超时未收到确认时重传尚未确认的数据，通过序号、确认和重传机制实现可靠传输。校验和用于检测TCP报文段传输中的差错。", "第六章 / TCP可靠传输"},
		{"为什么需要虚拟内存？", "传统存储管理要求作业一次性全部装入内存，并一直驻留到运行结束。若作业所需空间超过物理内存容量，作业就无法装入运行；暂时不用的程序和数据也会占据大量内存。虚拟内存基于程序的局部性，只需将当前需要的程序和数据按需调入内存，允许作业分多次装入运行，从逻辑上扩充可用的内存容量。", "第三章 / 虚拟内存"},
	} {
		t.Run(tc.q, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			st := testutil.NewStore()
			kb, e := st.CreateKnowledgeBase(ctx, "u", "合成教材", "")
			require.NoError(t, e)
			embed := OpenAIEmbedding{Dimension: 64, LocalLexical: true}
			vectors, e := embed.Embed(ctx, []string{tc.text})
			require.NoError(t, e)
			st.Docs["fixture"] = domain.Document{ID: "fixture", UserID: "u", KnowledgeBaseID: kb.ID, Status: "ready"}
			st.Chunks = []domain.Chunk{{ID: "c", DocumentID: "fixture", DocumentTitle: "合成教材.md", UserID: "u", KnowledgeBaseID: kb.ID, Content: tc.text, Embedding: vectors[0], Metadata: map[string]any{"resource_type": "textbook", "heading_path": tc.heading, "page": 1}}}
			rag := RAGService{Store: st, Embedder: embed, LLM: OpenAILLM{BaseURL: endpoint, APIKey: "local-fixture", Model: model}, Knowledge: KnowledgeService{Store: st}, Learning: LearningService{Store: st}, TopK: 3, MinScore: .35}
			result, e := rag.Chat(ctx, "u", kb.ID, "", tc.q)
			require.NoError(t, e)
			t.Log("synthetic-source answer:", result.Answer)
			require.NotContains(t, result.Answer, "不是模型生成答案")
			require.NotContains(t, result.Answer, "未能生成")
			require.NotContains(t, result.Answer, "依据不足")
			require.NotEmpty(t, result.Sources)
			require.True(t, adequateCoverage(tc.q, result.Answer, []string{tc.text}))
			for _, source := range result.Sources {
				require.Equal(t, "fixture", source.DocumentID)
				for _, quote := range strings.Split(source.Excerpt, "\n") {
					require.Contains(t, sourceWhitespace(tc.text), sourceWhitespace(quote))
				}
			}
		})
	}
}
