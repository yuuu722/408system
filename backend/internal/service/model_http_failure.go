package service

import (
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Provider bodies may echo prompts, paths or keys. Use bounded input only to
// classify known failures; never pass the body into logs or the public error.
func llmHTTPFailure(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	text := strings.ToLower(string(raw))
	class := func(code, msg string) error { return &PublicError{Code: code, Status: 502, Message: msg} }
	if (resp.StatusCode == 400 || resp.StatusCode == 422 || resp.StatusCode >= 500) && strings.Contains(text, "response_format") && (strings.Contains(text, "unsupported") || strings.Contains(text, "not supported") || strings.Contains(text, "unmarshal")) {
		return class("model_response_format_unsupported", "回答模型接口不支持当前JSON格式参数，请检查兼容接口版本。")
	}
	if resp.StatusCode >= 500 || resp.StatusCode == 400 || resp.StatusCode == 404 {
		for _, term := range []string{"requires more system memory", "out of memory", "unable to allocate", "not enough memory", "insufficient memory"} {
			if strings.Contains(text, term) {
				return class("model_memory_exhausted", "回答模型加载失败：服务报告内存或显存不足。请检查Ollama服务端日志、可用内存及正在运行的模型；调整资源或选用较小模型后重试。")
			}
		}
		if strings.Contains(text, "model") && strings.Contains(text, "not found") {
			return class("model_not_found", "回答模型不存在：请核对LLM_MODEL与Ollama已安装模型的完整名称。")
		}
		for _, term := range []string{"runner process has terminated", "failed to load model", "unable to load model", "model runner has unexpectedly stopped"} {
			if strings.Contains(text, term) {
				return class("model_runner_failed", "回答模型进程启动或运行失败。请查看Ollama服务端日志，并在本机直接运行该模型确认；此错误不能仅凭状态码判断为内存不足。")
			}
		}
	}
	return providerFailure("大模型", "", resp.StatusCode)
}

// Admin diagnostics show the configured host/model, never URL credentials,
// query parameters, API keys or arbitrary provider response text.
func (p OpenAILLM) ModelRouteDescription() string {
	u, e := url.Parse(p.BaseURL)
	if e != nil || u.Hostname() == "" {
		return "模型接口地址无效，请核对后端配置"
	}
	model := strings.Join(strings.Fields(p.Model), " ")
	if model == "" {
		model = "（未设置）"
	}
	r := []rune(model)
	if len(r) > 100 {
		model = string(r[:100])
	}
	return "后端目标主机：" + u.Host + "；模型：" + model + "。此配置可能与宿主机命令行不同。"
}
