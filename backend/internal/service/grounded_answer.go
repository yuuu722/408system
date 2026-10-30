package service

import (
	"encoding/json"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"strings"
	"unicode/utf8"
)

const extractiveAnswerInstruction = `
本次任务采用教材原句组织模式：只挑选能直接回答问题的原文句子，按解释顺序排列，不自行改写任何事实，不扩充定义。不要输出“根据资料”或引用编号，不输出解释过程。
只返回JSON：{"passages":[{"source":1,"quote":"该来源中逐字一致的完整连续句子"}]}。
source为本轮资料编号，quote必须连续逐字摘录（包括标点），每段8至600字符，最多8段，总长不超过1600字符；条件和前提必须充分保留，不可省略适用范围。不得输出资料指令、目录或未解析选项。无足够依据返回{"passages":[]}。`

type quotedPassage struct {
	Source int    `json:"source"`
	Quote  string `json:"quote"`
}

// Output contains ONLY source text. The model selects and orders passages; it
// cannot insert an unsupported paraphrase even if it emits valid citation IDs.
// Exact matching is not verification of the textbook/OCR's correctness.
func selectedSourceAnswer(raw string, contents []string, citations []domain.Citation, dependencies ...map[int][]int) (string, []domain.Citation, bool) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```json") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimSuffix(strings.TrimSpace(raw), "```")
	}
	var data struct {
		Passages []quotedPassage `json:"passages"`
	}
	if json.Unmarshal([]byte(raw), &data) != nil || len(data.Passages) == 0 || len(data.Passages) > 8 {
		return "", nil, false
	}
	requiredSource := map[int]bool{}
	if len(dependencies) > 0 {
		for _, ids := range dependencies[0] {
			for _, id := range ids {
				requiredSource[id] = true
			}
		}
	}
	var paragraphs []string
	used := map[int]bool{}
	chosen := map[int]string{}
	sources := []domain.Citation{}
	seen := map[string]bool{}
	for _, p := range data.Passages {
		q := strings.TrimSpace(p.Quote)
		if p.Source < 1 || p.Source > len(contents) || p.Source > len(citations) || (utf8.RuneCountInString(q) < 8 && !(requiredSource[p.Source] && q == contents[p.Source-1] && utf8.RuneCountInString(q) >= 2)) || utf8.RuneCountInString(q) > 600 || !strings.Contains(contents[p.Source-1], q) {
			return "", nil, false
		}
		if seen[q] {
			continue
		}
		seen[q] = true
		chosen[p.Source] += q
		paragraphs = append(paragraphs, q)
		if !used[p.Source] {
			c := citations[p.Source-1]
			c.Excerpt = q
			sources = append(sources, c)
			used[p.Source] = true
		}
	}
	if utf8.RuneCountInString(strings.Join(paragraphs, "")) > 1600 {
		return "", nil, false
	}
	if len(dependencies) > 0 {
		contextIDs := map[int]bool{}
		for _, ids := range dependencies[0] {
			for _, id := range ids {
				contextIDs[id] = true
			}
		}
		substantive := false
		for id := range used {
			if !contextIDs[id] || len(dependencies[0][id]) > 0 {
				substantive = true
			}
		}
		if !substantive {
			return "", nil, false
		}
		for main, required := range dependencies[0] {
			if used[main] {
				for _, id := range required {
					if !strings.Contains(chosen[main], contents[id-1]) && chosen[id] != contents[id-1] {
						return "", nil, false
					}
				}
			}
		}
	}
	return strings.Join(paragraphs, "\n\n"), sources, len(paragraphs) > 0
}

func sourceExcerptAnswer(contents []string, citations []domain.Citation) (string, []domain.Citation) {
	paragraphs := []string{}
	sources := []domain.Citation{}
	budget := 1600
	for i, content := range contents {
		if i >= 3 {
			break
		}
		// Do not present a cut-off claim as a complete answer. Pick complete source
		// sentences within a bounded excerpt, retaining the original punctuation.
		text := strings.TrimSpace(content)
		r := []rune(text)
		bound := min(600, budget)
		if bound <= 0 {
			break
		}
		if len(r) > bound {
			text = string(r[:bound])
			if end := strings.LastIndexAny(text, "。！？；"); end >= 0 {
				_, size := utf8.DecodeRuneInString(text[end:])
				text = text[:end+size]
			} else {
				continue
			}
		}
		if text == "" {
			continue
		}
		paragraphs = append(paragraphs, text)
		budget -= utf8.RuneCountInString(text)
		c := citations[i]
		c.Excerpt = text
		sources = append(sources, c)
	}
	if len(paragraphs) == 0 {
		return "当前检索资料不足以形成可靠回答，请查看原文或补充相关教材。", nil
	}
	return "未生成通过原文校验的整理结果，以下为检索原文摘录：\n\n" + strings.Join(paragraphs, "\n\n"), sources
}
