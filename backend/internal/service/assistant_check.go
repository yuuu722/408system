package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"strings"
	"time"
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
	if model, ok := s.LLM.(interface{ ModelRouteDescription() string }); ok {
		report.Checks = append(report.Checks, AssistantCheck{"模型配置摘要", true, model.ModelRouteDescription()})
	}
	graphOK := true
	if s.KAGEnabled {
		graph, ok := s.Store.(interface{ CheckGraph(context.Context) error })
		if !ok {
			report.Checks = append(report.Checks, AssistantCheck{"Neo4j图谱", false, "图谱服务未接入"})
			graphOK = false
		} else {
			graphCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			e := graph.CheckGraph(graphCtx)
			cancel()
			if e != nil {
				report.Checks = append(report.Checks, AssistantCheck{"Neo4j图谱", false, "图谱连接检测失败；仍继续检测模型与检索服务"})
				graphOK = false
			} else {
				report.Checks = append(report.Checks, AssistantCheck{"Neo4j图谱", true, "图谱服务可连接；文档与图谱同步情况以实际知识库为准"})
			}
		}
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
	if model, ok := s.LLM.(jsonAnswerer); ok {
		answer, err = model.AnswerJSON(ctx, `只返回JSON {"ok":true}，用于检查结构化接口。`, "本次为无用户资料的连接测试。")
		if err == nil {
			var result struct {
				OK bool `json:"ok"`
			}
			if json.Unmarshal([]byte(modelJSONObject(answer)), &result) != nil || !result.OK {
				err = providerFailure("大模型", "结构化检测未返回有效JSON", 0)
			}
		}
	} else if llm, ok := s.LLM.(domain.StreamingLLM); ok {
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
	if model, ok := s.LLM.(jsonAnswerer); ok {
		longEvidence := strings.Repeat("这是一段无用户数据的合成资料，用于验证较长输入能否正常处理。", 65)
		probe, e := model.AnswerJSON(ctx, `只返回JSON {"ok":true}，不要总结资料。`, longEvidence)
		valid := false
		if e == nil {
			var result struct {
				OK bool `json:"ok"`
			}
			valid = json.Unmarshal([]byte(modelJSONObject(probe)), &result) == nil && result.OK
		}
		if !valid {
			message := "后端短请求成功，但较长输入未通过JSON检测；请检查模型运行日志与上下文资源"
			if e != nil {
				message += "。" + safe(e)
			}
			report.Checks = append(report.Checks, AssistantCheck{"模型较长输入", false, message})
			return report
		}
		report.Checks = append(report.Checks, AssistantCheck{"模型较长输入", true, "后端实际配置已通过短JSON和较长合成输入检测；不等于全部教材问答已验收"})
	}
	report.Passed = graphOK
	return report
}
