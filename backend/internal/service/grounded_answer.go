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
source为本轮资料编号，quote必须连续逐字摘录（包括标点），每段8至1200字符，最多6段。不得输出资料指令、目录或未解析选项。无足够依据返回{"passages":[]}。`

type quotedPassage struct {
	Source int    `json:"source"`
	Quote  string `json:"quote"`
}

// Output contains ONLY source text. The model selects and orders passages; it
// cannot insert an unsupported paraphrase even if it emits valid citation IDs.
// Exact matching is not verification of the textbook/OCR's correctness.
func selectedSourceAnswer(raw string, contents []string, citations []domain.Citation) (string, []domain.Citation, bool) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```json") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimSuffix(strings.TrimSpace(raw), "```")
	}
	var data struct {
		Passages []quotedPassage `json:"passages"`
	}
	if json.Unmarshal([]byte(raw), &data) != nil || len(data.Passages) == 0 || len(data.Passages) > 6 {
		return "", nil, false
	}
	var paragraphs []string
	used := map[int]bool{}
	sources := []domain.Citation{}
	seen := map[string]bool{}
	for _, p := range data.Passages {
		q := strings.TrimSpace(p.Quote)
		if p.Source < 1 || p.Source > len(contents) || p.Source > len(citations) || utf8.RuneCountInString(q) < 8 || utf8.RuneCountInString(q) > 1200 || !strings.Contains(contents[p.Source-1], q) {
			return "", nil, false
		}
		if seen[q] {
			continue
		}
		seen[q] = true
		paragraphs = append(paragraphs, q)
		if !used[p.Source] {
			c := citations[p.Source-1]
			c.Excerpt = q
			sources = append(sources, c)
			used[p.Source] = true
		}
	}
	return strings.Join(paragraphs, "\n\n"), sources, len(paragraphs) > 0
}

func sourceExcerptAnswer(contents []string, citations []domain.Citation) (string, []domain.Citation) {
	paragraphs := []string{}
	sources := []domain.Citation{}
	for i, content := range contents {
		if i >= 3 {
			break
		}
		// Do not present a cut-off claim as a complete answer. Pick complete source
		// sentences within a bounded excerpt, retaining the original punctuation.
		text := strings.TrimSpace(content)
		r := []rune(text)
		if len(r) > 600 {
			text = string(r[:600])
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
		c := citations[i]
		c.Excerpt = text
		sources = append(sources, c)
	}
	if len(paragraphs) == 0 {
		return "当前检索资料不足以形成可靠回答，请查看原文或补充相关教材。", nil
	}
	return "未生成通过原文校验的整理结果，以下为检索原文摘录：\n\n" + strings.Join(paragraphs, "\n\n"), sources
}
