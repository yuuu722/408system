package service

import (
	"context"
	"errors"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"strings"
)

type AssistantCheck struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}
type AssistantCheckReport struct {
	Passed bool             `json:"passed"`
	Checks []AssistantCheck `json:"checks"`
}

// Probes actual provider response formats. It never exposes responses, API keys,
// corpus contents, or generated model text to the diagnostics UI.
func (s RAGService) CheckProviders(ctx context.Context) AssistantCheckReport {
	report := AssistantCheckReport{Checks: []AssistantCheck{}}
	if s.Embedder == nil || s.LLM == nil {
		report.Checks = append(report.Checks, AssistantCheck{"服务初始化", false, "模型组件未初始化，请检查后端启动配置"})
		return report
	}
	safe := func(err error) string {
		var public *PublicError
		if errors.As(err, &public) {
			return public.Message
		}
		return "连接检测失败，请管理员查看后端日志"
	}
	vectors, err := s.Embedder.Embed(ctx, []string{"408问答连接检测"})
	if err != nil {
		report.Checks = append(report.Checks, AssistantCheck{"检索服务", false, safe(err)})
		return report
	}
	if len(vectors) != 1 || len(vectors[0]) == 0 {
		report.Checks = append(report.Checks, AssistantCheck{"检索服务", false, "检索服务返回空向量或无效格式"})
		return report
	}
	report.Checks = append(report.Checks, AssistantCheck{"检索服务", true, "已成功生成检索向量"})
	var answer string
	const question = "请根据资料回复连接正常并标注引用。"
	const evidence = "[1] 连接检测资料：本次请求用于验证模型接口是否返回有效文本，不涉及用户知识库。"
	if llm, ok := s.LLM.(domain.StreamingLLM); ok {
		answer, err = llm.AnswerStream(ctx, question, evidence, nil, func(string) error { return ctx.Err() })
	} else {
		answer, err = s.LLM.Answer(ctx, question, evidence)
	}
	if err != nil {
		report.Checks = append(report.Checks, AssistantCheck{"回答模型", false, safe(err)})
		return report
	}
	if strings.TrimSpace(answer) == "" {
		report.Checks = append(report.Checks, AssistantCheck{"回答模型", false, "回答模型返回空文本"})
		return report
	}
	report.Checks = append(report.Checks, AssistantCheck{"回答模型", true, "模型接口已返回有效文本；具体问题仍需检索到资料依据"})
	report.Passed = true
	return report
}
