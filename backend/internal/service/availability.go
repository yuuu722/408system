package service

import "fmt"

// PublicError contains only safe, user-facing text, never provider response bodies or credentials.
type PublicError struct {
	Code, Message string
	Status        int
	Retryable     bool
}

func (e *PublicError) Temporary() bool { return e.Retryable }

func (e *PublicError) Error() string { return e.Message }

func unavailable(provider string) error {
	return &PublicError{Code: "model_not_configured", Status: 503, Message: provider + "尚未配置，智能答疑暂不可用。请联系管理员配置模型。"}
}

func providerFailure(provider, reason string, status int) error {
	if status == 401 || status == 403 {
		reason = "认证失败，请检查密钥和模型权限"
	}
	if status == 429 {
		reason = "请求受限，请稍后重试或检查额度"
	}
	if status > 0 && reason == "" {
		reason = fmt.Sprintf("服务返回 HTTP %d，请检查模型名称或服务状态", status)
	}
	return &PublicError{Code: "model_call_failed", Status: 502, Retryable: status == 429 || status >= 500 || (status == 0 && reason == "网络连接失败或请求超时"), Message: provider + "调用失败：" + reason + "。未生成回答，请重试或联系管理员。"}
}
