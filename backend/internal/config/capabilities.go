package config

import "strings"

type AssistantStatus struct {
	Mode    string `json:"mode"`
	Enabled bool   `json:"enabled"`
	Message string `json:"message"`
}

func (c Config) AssistantStatus() AssistantStatus {
	if c.DemoMode {
		return AssistantStatus{"demo", true, "检索片段预览，非 AI 回答。演示模式不调用大模型，检索结果可能不准确。"}
	}
	if strings.TrimSpace(c.LLMAPIKey) == "" {
		return AssistantStatus{"unavailable", false, "回答模型未配置：请管理员设置有效的 LLM_API_KEY。知识库资料仍保留。"}
	}
	if !c.UsesLocalIndex() && strings.TrimSpace(c.EmbeddingAPIKey) == "" {
		return AssistantStatus{"unavailable", false, "远程检索模型未配置：请管理员设置 EMBEDDING_API_KEY，或明确使用本地 lexical 索引。"}
	}
	return AssistantStatus{"model", true, "模型已配置；实际可用性以调用结果为准。回答请结合引用原文核实。"}
}
