package domain

import "strings"

// Readiness is computed, never approved by a publisher or supplied by a client.
func PrepareAsset(a *Asset) {
	a.Status = "unavailable"
	if a.Payload.AnswerOrigin == "" {
		if a.Payload.Answer >= 0 {
			a.Payload.AnswerOrigin = "source"
		} else {
			a.Payload.AnswerOrigin = "ai_pending"
		}
	}
	if a.Payload.Issue != "" {
		return
	}
	if a.PointID == "" {
		a.Payload.Issue = "未匹配知识点（Knowledge point unavailable）"
		return
	}
	if a.Kind == "question" {
		if strings.TrimSpace(a.Payload.Stem) == "" || len(a.Payload.Options) < 2 || len(a.Payload.Options) > 6 {
			a.Payload.Issue = "题干或选项不完整（Incomplete question）"
			return
		}
		seen := map[string]bool{}
		for _, v := range a.Payload.Options {
			v = strings.TrimSpace(v)
			if v == "" || seen[v] {
				a.Payload.Issue = "选项为空或重复（Invalid options）"
				return
			}
			seen[v] = true
		}
		if a.Payload.AnswerOrigin == "source" && (a.Payload.Answer < 0 || a.Payload.Answer >= len(a.Payload.Options)) {
			a.Payload.Issue = "资料答案超出选项范围（Invalid source answer）"
			return
		}
	} else if a.Payload.URL == "" && a.Payload.DocumentID == "" && !(a.Kind == "material" && strings.TrimSpace(a.Payload.Stem) != "") {
		a.Payload.Issue = "缺少资源地址（Missing resource）"
		return
	}
	a.Status = "ready"
}
