package service_test

import (
	"context"
	"encoding/json"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/service"
	"github.com/local/exam408-assistant/backend/internal/testutil"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type intentStore struct {
	*testutil.Store
	extra []domain.Chunk
	kids  []string
}

func (s *intentStore) SearchTextbookEvidence(_ context.Context, _ string, kid string, topics, needs []string, _ int) ([]domain.Chunk, error) {
	s.kids = append(s.kids, kid)
	return s.extra, nil
}
func TestWhyQuestionRecoversTraditionalMemoryReasonOutsideVectorHits(t *testing.T) {
	ctx := context.Background()
	st := &intentStore{Store: testutil.NewStore()}
	kb, e := st.CreateKnowledgeBase(ctx, "u", "教材", "")
	require.NoError(t, e)
	reason := "传统存储管理要求作业一次性全部装入并驻留在内存中。作业大于可用物理内存时无法运行，暂时不用的程序也会占用内存。"
	mechanism := "基于局部性原理，应用程序只需将当前要运行的少数页面装入内存便可运行，其余部分暂留磁盘。"
	for _, id := range []string{"cause", "mechanism"} {
		st.Docs[id] = domain.Document{ID: id, UserID: "u", KnowledgeBaseID: kb.ID, Status: "ready"}
	}
	st.Chunks = []domain.Chunk{{ID: "m", DocumentID: "mechanism", UserID: "u", KnowledgeBaseID: kb.ID, Content: mechanism, Embedding: []float32{1, 0}, Metadata: map[string]any{"resource_type": "textbook", "heading_path": "3.2 虚拟内存"}}}
	st.extra = []domain.Chunk{{ID: "r", DocumentID: "cause", UserID: "u", KnowledgeBaseID: kb.ID, Content: reason, Score: 1, Metadata: map[string]any{"resource_type": "textbook", "heading_path": "3.2 虚拟内存"}}}
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Messages []struct{ Content string } }
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		last := body.Messages[1].Content
		content := `{"supported":true}`
		if !strings.Contains(last, `"supported"`) {
			require.Contains(t, last, "一次性")
			require.Contains(t, last, "驻留")
			require.Contains(t, last, "局部性")
			content = `{"claims":[{"text":"传统存储管理要求作业一次性装入并驻留，作业大小受到物理内存容量限制，暂时不用的部分也会占用内存。虚拟内存基于局部性，只需调入当前使用的部分，其余部分留在磁盘。","kind":"explanation","facts":[1,2,3]}]}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}, "finish_reason": "stop"}}})
	}))
	defer model.Close()
	rag := service.RAGService{Store: st, Embedder: fixedEmbedding{}, LLM: service.OpenAILLM{BaseURL: model.URL, APIKey: "fixture"}, Knowledge: service.KnowledgeService{Store: st}, Learning: service.LearningService{Store: st}, TopK: 3, MinScore: .35}
	result, e := rag.Chat(ctx, "u", kb.ID, "", "为什么需要虚拟内存？")
	require.NoError(t, e)
	require.Equal(t, []string{kb.ID}, st.kids)
	require.NotContains(t, result.Answer, "原句（不是模型生成答案）")
	require.Contains(t, result.Answer, "物理内存容量限制")
	require.NotNil(t, result.Generation)
	require.True(t, result.Generation.Attempted)
	require.True(t, result.Generation.ProviderReturned)
	require.True(t, result.Generation.Verified)
	require.Equal(t, 1, result.Generation.Attempts)
	seen := map[string]bool{}
	for _, s := range result.Sources {
		seen[s.DocumentID] = true
	}
	require.True(t, seen["cause"])
	require.True(t, seen["mechanism"])
}
func TestGenerationTraceShowsReturnedModelButInvalidReview(t *testing.T) {
	ctx := context.Background()
	st := testutil.NewStore()
	kb, e := st.CreateKnowledgeBase(ctx, "u", "教材", "")
	require.NoError(t, e)
	source := "虚拟内存允许作业按需调入内存。"
	st.Docs["d"] = domain.Document{ID: "d", UserID: "u", KnowledgeBaseID: kb.ID, Status: "ready"}
	st.Chunks = []domain.Chunk{{ID: "c", DocumentID: "d", UserID: "u", KnowledgeBaseID: kb.ID, Content: source, Embedding: []float32{1, 0}}}
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Messages []struct{ Content string } }
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		content := `{"claims":[{"text":"虚拟内存支持作业按需调入内存。","facts":[1]}]}`
		if strings.Contains(body.Messages[1].Content, `"supported"`) {
			content = `{"verdict":"PRIVATE"}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}, "finish_reason": "stop"}}})
	}))
	defer model.Close()
	rag := service.RAGService{Store: st, Embedder: fixedEmbedding{}, LLM: service.OpenAILLM{BaseURL: model.URL, APIKey: "fixture"}, Knowledge: service.KnowledgeService{Store: st}, Learning: service.LearningService{Store: st}, TopK: 3, MinScore: .35}
	result, e := rag.Chat(ctx, "u", kb.ID, "", "解释虚拟内存")
	require.NoError(t, e)
	require.True(t, result.Generation.Attempted)
	require.True(t, result.Generation.ProviderReturned)
	require.False(t, result.Generation.Verified)
	require.Equal(t, "invalid_review_format", result.Generation.FailureCode)
	require.Contains(t, result.Generation.Notice, "格式无效")
	require.NotContains(t, result.Generation.Notice, "PRIVATE")
	require.NotContains(t, result.Answer, "PRIVATE")
}

func TestInvalidReviewIsRetriedWithoutRegeneratingSupportedAnswer(t *testing.T) {
	ctx := context.Background()
	st := testutil.NewStore()
	kb, e := st.CreateKnowledgeBase(ctx, "u", "教材", "")
	require.NoError(t, e)
	source := "虚拟内存允许作业按需调入内存。"
	st.Docs["d"] = domain.Document{ID: "d", UserID: "u", KnowledgeBaseID: kb.ID, Status: "ready"}
	st.Chunks = []domain.Chunk{{ID: "c", DocumentID: "d", UserID: "u", KnowledgeBaseID: kb.ID, Content: source, Embedding: []float32{1, 0}}}
	generationCalls, reviewCalls := 0, 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct{ Content string }
			Format   map[string]any `json:"response_format"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		content := `{"claims":[{"text":"虚拟内存支持作业按需调入内存。","facts":[1]}]}`
		if strings.Contains(body.Messages[1].Content, `"supported"`) {
			reviewCalls++
			require.Equal(t, "json_schema", body.Format["type"])
			content = `{"supported":true}`
			if reviewCalls == 1 {
				content = `{"wrong_field":true}`
			}
		} else {
			generationCalls++
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}, "finish_reason": "stop"}}})
	}))
	defer model.Close()
	rag := service.RAGService{Store: st, Embedder: fixedEmbedding{}, LLM: service.OpenAILLM{BaseURL: model.URL, APIKey: "fixture"}, Knowledge: service.KnowledgeService{Store: st}, Learning: service.LearningService{Store: st}, TopK: 3, MinScore: .35}
	result, e := rag.Chat(ctx, "u", kb.ID, "", "解释虚拟内存")
	require.NoError(t, e)
	require.Equal(t, 1, generationCalls)
	require.Equal(t, 2, reviewCalls)
	require.True(t, result.Generation.Verified)
	require.Equal(t, 1, result.Generation.Attempts)
	require.NotContains(t, result.Answer, "原句（不是模型生成答案）")
}

func TestPPPWhyRecoversPurposeEvidenceAndAcceptsGroundedModelReply(t *testing.T) {
	ctx := context.Background()
	st := &intentStore{Store: testutil.NewStore()}
	kb, e := st.CreateKnowledgeBase(ctx, "u", "教材", "")
	require.NoError(t, e)
	format := "IP数据报在PPP帧中是信息部分，其长度受最大传送单元MTU的限制。PPP帧首部有四个字段，尾部有两个字段。"
	purpose := "PPP用于点到点链路，支持多种网络层协议的数据传输，并提供建立、配置和测试链路的功能。"
	for _, id := range []string{"format", "purpose"} {
		st.Docs[id] = domain.Document{ID: id, UserID: "u", KnowledgeBaseID: kb.ID, Status: "ready"}
	}
	st.Chunks = []domain.Chunk{{ID: "f", DocumentID: "format", UserID: "u", KnowledgeBaseID: kb.ID, Content: format, Embedding: []float32{1, 0}, Metadata: map[string]any{"resource_type": "textbook", "heading_path": "PPP帧格式"}}}
	st.extra = []domain.Chunk{{ID: "p", DocumentID: "purpose", UserID: "u", KnowledgeBaseID: kb.ID, Content: purpose, Score: 1, Metadata: map[string]any{"resource_type": "textbook", "heading_path": "PPP协议的功能"}}}
	requests, reviews := 0, 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Messages []struct{ Content string } }
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		content := `{"supported":true}`
		if strings.Contains(body.Messages[1].Content, `"supported"`) {
			reviews++
		} else {
			requests++
			require.Contains(t, body.Messages[1].Content, "点到点链路")
			content = `{"claims":[{"text":"使用PPP可以在点到点链路上传输多种网络层协议的数据，并提供链路建立、配置和测试功能。","kind":"explanation","facts":[1]}]}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}, "finish_reason": "stop"}}})
	}))
	defer model.Close()
	rag := service.RAGService{Store: st, Embedder: fixedEmbedding{}, LLM: service.OpenAILLM{BaseURL: model.URL, APIKey: "fixture"}, Knowledge: service.KnowledgeService{Store: st}, Learning: service.LearningService{Store: st}, TopK: 3, MinScore: .35}
	result, e := rag.Chat(ctx, "u", kb.ID, "", "为什么要用PPP")
	require.NoError(t, e)
	require.True(t, result.Generation.Verified)
	require.Equal(t, 1, requests)
	require.Equal(t, 1, reviews)
	require.Contains(t, result.Answer, "点到点链路")
	require.NotContains(t, result.Answer, "未通过")
	require.NotEmpty(t, result.Sources)
	for _, source := range result.Sources {
		require.Equal(t, "purpose", source.DocumentID)
		require.Contains(t, purpose, source.Excerpt)
	}
}

func TestPPPWhyDoesNotTreatFrameOnlyFallbackAsAnAnswer(t *testing.T) {
	ctx := context.Background()
	st := testutil.NewStore()
	kb, e := st.CreateKnowledgeBase(ctx, "u", "教材", "")
	require.NoError(t, e)
	text := "PPP帧首部有四个字段，尾部有两个字段。PPP信息部分的长度受限于最大传送单元MTU。"
	st.Docs["d"] = domain.Document{ID: "d", UserID: "u", KnowledgeBaseID: kb.ID, Status: "ready"}
	st.Chunks = []domain.Chunk{{ID: "c", DocumentID: "d", UserID: "u", KnowledgeBaseID: kb.ID, Content: text, Embedding: []float32{1, 0}, Metadata: map[string]any{"resource_type": "textbook"}}}
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"claims":[{"text":"PPP帧首部有四个字段，尾部有两个字段。","kind":"explanation","facts":[1]}]}`}, "finish_reason": "stop"}}})
	}))
	defer model.Close()
	rag := service.RAGService{Store: st, Embedder: fixedEmbedding{}, LLM: service.OpenAILLM{BaseURL: model.URL, APIKey: "fixture"}, Knowledge: service.KnowledgeService{Store: st}, Learning: service.LearningService{Store: st}, TopK: 3, MinScore: .35}
	result, e := rag.Chat(ctx, "u", kb.ID, "", "为什么要用PPP")
	require.NoError(t, e)
	require.False(t, result.Generation.Verified)
	require.True(t, result.Generation.Attempted)
	require.Contains(t, result.Answer, "不足以解释")
	require.NotContains(t, result.Answer, "以下为教材中可核对的完整原句")
}

func TestPPPWhyStillRejectsProviderReplyWhenSupportReviewFails(t *testing.T) {
	ctx := context.Background()
	st := testutil.NewStore()
	kb, e := st.CreateKnowledgeBase(ctx, "u", "教材", "")
	require.NoError(t, e)
	text := "PPP用于点到点链路，支持多种网络层协议，并提供建立和配置链路的功能。"
	st.Docs["d"] = domain.Document{ID: "d", UserID: "u", KnowledgeBaseID: kb.ID, Status: "ready"}
	st.Chunks = []domain.Chunk{{ID: "c", DocumentID: "d", UserID: "u", KnowledgeBaseID: kb.ID, Content: text, Embedding: []float32{1, 0}, Metadata: map[string]any{"resource_type": "textbook"}}}
	reviews := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Messages []struct{ Content string } }
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		value := `{"claims":[{"text":"PPP为点到点链路提供可靠传输服务，通过重传纠正所有差错。","kind":"explanation","facts":[1]}]}`
		if strings.Contains(body.Messages[1].Content, `"supported"`) {
			reviews++
			value = `{"supported":false}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": value}, "finish_reason": "stop"}}})
	}))
	defer model.Close()
	rag := service.RAGService{Store: st, Embedder: fixedEmbedding{}, LLM: service.OpenAILLM{BaseURL: model.URL, APIKey: "fixture"}, Knowledge: service.KnowledgeService{Store: st}, Learning: service.LearningService{Store: st}, TopK: 3, MinScore: .35}
	result, e := rag.Chat(ctx, "u", kb.ID, "", "为什么要用PPP")
	require.NoError(t, e)
	require.False(t, result.Generation.Verified)
	require.GreaterOrEqual(t, reviews, 1)
	require.Equal(t, "unsupported_claim", result.Generation.FailureCode)
	require.NotContains(t, result.Answer, "通过重传纠正所有差错")
}

func TestComparisonChatUsesSeparateSidesAndDropsOtherComparisons(t *testing.T) {
	ctx := context.Background()
	st := testutil.NewStore()
	kb, e := st.CreateKnowledgeBase(ctx, "u", "教材", "")
	require.NoError(t, e)
	text := "进程是资源分配和保护的基本单位。线程是处理机调度与执行的基本单位。每个进程通常具有独立的虚拟地址空间。同一进程的线程共享地址空间，各自有独立的栈。进程与程序有本质区别，进程是动态的而程序是静态的。自旋锁与信号量的区别在于是否阻塞进程。"
	st.Docs["d"] = domain.Document{ID: "d", UserID: "u", KnowledgeBaseID: kb.ID, Status: "ready"}
	st.Chunks = []domain.Chunk{{ID: "c", DocumentID: "d", UserID: "u", KnowledgeBaseID: kb.ID, Content: text, Embedding: []float32{1, 0}, Metadata: map[string]any{"resource_type": "textbook", "heading_path": "进程与线程的比较", "page": 21}}}
	reviews := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages       []struct{ Content string }
			ResponseFormat map[string]any `json:"response_format"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		prompt := body.Messages[1].Content
		value := `{"supported":true}`
		if strings.Contains(prompt, `"supported"`) {
			reviews++
			require.Contains(t, prompt, "comparison_rows")
		} else {
			require.Contains(t, prompt, "left_facts")
			require.NotContains(t, prompt, "自旋锁")
			require.NotContains(t, prompt, "有本质区别")
			pos := strings.LastIndex(prompt, `{"comparison":`)
			require.GreaterOrEqual(t, pos, 0)
			var facts struct {
				Facts []struct {
					ID   int
					Text string
				}
			}
			require.NoError(t, json.Unmarshal([]byte(prompt[pos:]), &facts))
			find := func(term string) int {
				for _, f := range facts.Facts {
					if strings.Contains(f.Text, term) {
						return f.ID
					}
				}
				t.Errorf("missing %s", term)
				return 0
			}
			rows := []map[string]any{
				{"dimension": "基本作用", "left": "进程是资源分配和保护的基本单位。", "right": "线程是处理机调度与执行的基本单位。", "left_facts": []int{find("资源分配")}, "right_facts": []int{find("处理机调度")}},
				{"dimension": "地址空间", "left": "每个进程通常具有独立的虚拟地址空间。", "right": "同一进程的线程共享地址空间，各自有独立的栈。", "left_facts": []int{find("每个进程")}, "right_facts": []int{find("同一进程")}},
			}
			raw, _ := json.Marshal(map[string]any{"comparisons": rows})
			value = string(raw)
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": value}, "finish_reason": "stop"}}})
	}))
	defer model.Close()
	rag := service.RAGService{Store: st, Embedder: fixedEmbedding{}, LLM: service.OpenAILLM{BaseURL: model.URL, APIKey: "fixture"}, Knowledge: service.KnowledgeService{Store: st}, Learning: service.LearningService{Store: st}, TopK: 3, MinScore: .35}
	result, e := rag.Chat(ctx, "u", kb.ID, "", "比较进程与线程的区别")
	require.NoError(t, e)
	require.True(t, result.Generation.Verified, result.Generation.Notice)
	require.Equal(t, 1, reviews)
	require.Contains(t, result.Answer, "地址空间")
	require.NotContains(t, result.Answer, "自旋锁")
	require.NotEmpty(t, result.Sources)
	require.Equal(t, 21, result.Sources[0].Page)
}

func TestVirtualMemoryRepairsSuperficialAnswerUsingTextbookFacts(t *testing.T) {
	ctx := context.Background()
	st := &intentStore{Store: testutil.NewStore()}
	kb, e := st.CreateKnowledgeBase(ctx, "u", "教材", "")
	require.NoError(t, e)
	reason := "传统存储管理要求作业一次性全部装入并驻留在内存中。作业大于可用物理内存时无法运行，暂时不用的程序也会占用内存。"
	mechanism := "基于局部性原理，应用程序只需将当前要运行的少数页面装入内存便可运行，其余部分暂留磁盘。"
	for _, id := range []string{"cause", "mechanism"} {
		st.Docs[id] = domain.Document{ID: id, UserID: "u", KnowledgeBaseID: kb.ID, Status: "ready"}
	}
	st.Chunks = []domain.Chunk{{ID: "m", DocumentID: "mechanism", UserID: "u", KnowledgeBaseID: kb.ID, Content: mechanism, Embedding: []float32{1, 0}, Metadata: map[string]any{"resource_type": "textbook", "heading_path": "3.2 虚拟内存"}}}
	st.extra = []domain.Chunk{{ID: "r", DocumentID: "cause", UserID: "u", KnowledgeBaseID: kb.ID, Content: reason, Score: 1, Metadata: map[string]any{"resource_type": "textbook", "heading_path": "3.2 虚拟内存"}}}
	generationCalls := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Messages []struct{ Content string } }
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		last := body.Messages[1].Content
		content := `{"supported":true}`
		if !strings.Contains(last, `"supported"`) {
			require.Contains(t, last, "一次性")
			require.Contains(t, last, "驻留")
			require.Contains(t, last, "局部性")
			content = `{"claims":[{"text":"传统存储管理要求作业一次性装入并驻留，作业大小受到物理内存容量限制，暂时不用的部分也会占用内存。虚拟内存基于局部性，只需调入当前使用的部分，其余部分留在磁盘。","kind":"explanation","facts":[1,2,3]}]}`
			generationCalls++
			if generationCalls == 1 {
				content = `{"claims":[{"text":"传统管理的一次性和驻留性造成浪费，虚拟内存通过对换提高利用率。","kind":"explanation","facts":[1,2]}]}`
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}, "finish_reason": "stop"}}})
	}))
	defer model.Close()
	rag := service.RAGService{Store: st, Embedder: fixedEmbedding{}, LLM: service.OpenAILLM{BaseURL: model.URL, APIKey: "fixture"}, Knowledge: service.KnowledgeService{Store: st}, Learning: service.LearningService{Store: st}, TopK: 3, MinScore: .35}
	result, e := rag.Chat(ctx, "u", kb.ID, "", "为什么需要虚拟内存？")
	require.NoError(t, e)
	require.Equal(t, []string{kb.ID}, st.kids)
	require.NotContains(t, result.Answer, "原句（不是模型生成答案）")
	require.Contains(t, result.Answer, "物理内存容量限制")
	require.NotNil(t, result.Generation)
	require.True(t, result.Generation.Attempted)
	require.True(t, result.Generation.ProviderReturned)
	require.True(t, result.Generation.Verified)
	require.Equal(t, 2, result.Generation.Attempts)
	require.Equal(t, 2, generationCalls)
	seen := map[string]bool{}
	for _, s := range result.Sources {
		seen[s.DocumentID] = true
	}
	require.True(t, seen["cause"])
	require.True(t, seen["mechanism"])
}
