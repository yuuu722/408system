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
	if strings.TrimSpace(c.LLMAPIKey) == "" || strings.TrimSpace(c.EmbeddingAPIKey) == "" {
		return AssistantStatus{"unavailable", false, "尚未配置大模型或向量模型，智能答疑暂不可用。请联系管理员配置模型。"}
	}
	return AssistantStatus{"model", true, "模型已配置；实际可用性以调用结果为准。回答请结合引用原文核实。"}
}
