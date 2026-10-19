package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/local/exam408-assistant/backend/internal/domain"
)

type OpenAIEmbedding struct {
	BaseURL, APIKey, Model string
	Dimension              int
	Client                 *http.Client
	DemoMode               bool
	LocalLexical           bool
}

func (p OpenAIEmbedding) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if p.LocalLexical {
		return (LexicalEmbedding{Dimension: p.Dimension}).Embed(ctx, texts)
	}
	if p.DemoMode {
		return DeterministicEmbedding{Dimension: p.Dimension}.Embed(ctx, texts)
	}
	if strings.TrimSpace(p.APIKey) == "" {
		return nil, unavailable("向量模型")
	}
	body, _ := json.Marshal(map[string]any{"model": p.Model, "input": texts, "dimensions": p.Dimension})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.BaseURL, "/")+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	c := p.Client
	if c == nil {
		c = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, providerFailure("向量模型", "网络连接失败或请求超时", 0)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, providerFailure("向量模型", "", resp.StatusCode)
	}
	var out struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, providerFailure("向量模型", "返回格式无效", 0)
	}
	vectors := make([][]float32, len(texts))
	for _, v := range out.Data {
		if v.Index < 0 || v.Index >= len(vectors) || vectors[v.Index] != nil || len(v.Embedding) != p.Dimension {
			return nil, providerFailure("向量模型", "返回向量数量或维度无效", 0)
		}
		vectors[v.Index] = v.Embedding
	}
	for _, v := range vectors {
		if len(v) != p.Dimension {
			return nil, providerFailure("向量模型", "返回向量不完整", 0)
		}
	}
	return vectors, nil
}

type DeterministicEmbedding struct{ Dimension int }

func (p DeterministicEmbedding) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		v := make([]float32, p.Dimension)
		tokens := strings.Fields(strings.ToLower(text))
		if len(tokens) == 0 {
			tokens = []string{text}
		}
		for _, token := range tokens {
			h := sha256.Sum256([]byte(token))
			idx := int(binary.BigEndian.Uint32(h[:4]) % uint32(p.Dimension))
			sign := float32(1)
			if h[4]&1 == 1 {
				sign = -1
			}
			v[idx] += sign
		}
		var norm float64
		for _, n := range v {
			norm += float64(n * n)
		}
		if norm > 0 {
			root := float32(math.Sqrt(norm))
			for j := range v {
				v[j] /= root
			}
		}
		out[i] = v
	}
	return out, nil
}

type OpenAILLM struct {
	BaseURL, APIKey, Model string
	Client                 *http.Client
	DemoMode               bool
	LocalLexical           bool
}

func (p OpenAILLM) Answer(ctx context.Context, question, evidence string) (string, error) {
	if p.DemoMode {
		if strings.TrimSpace(evidence) == "" {
			return "当前知识库中没有检索到足够资料，暂时无法基于资料回答这个问题。", nil
		}
		return "检索片段预览，非 AI 回答\n\n" + evidence, nil
	}
	if strings.TrimSpace(p.APIKey) == "" {
		return "", unavailable("大模型")
	}
	system := "你是408考研学习助手。只能根据给定资料回答；资料不足时明确说依据不足。回答使用中文，并用[1]、[2]标注依据。"
	body, _ := json.Marshal(map[string]any{"model": p.Model, "temperature": 0.2, "messages": []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": "问题：" + question + "\n\n资料：\n" + evidence}}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	c := p.Client
	if c == nil {
		c = &http.Client{Timeout: 90 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", providerFailure("大模型", "网络连接失败或请求超时", 0)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", providerFailure("大模型", "", resp.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", providerFailure("大模型", "返回格式无效", 0)
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", providerFailure("大模型", "返回内容为空", 0)
	}
	return out.Choices[0].Message.Content, nil
}

var _ domain.EmbeddingProvider = (*OpenAIEmbedding)(nil)
var _ domain.LLMProvider = (*OpenAILLM)(nil)
