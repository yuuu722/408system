package service

import (
	"encoding/json"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"strings"
	"unicode/utf8"
)

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
		q := sourceWhitespace(p.Quote)
		if p.Source < 1 || p.Source > len(contents) || p.Source > len(citations) || (utf8.RuneCountInString(q) < 8 && !(requiredSource[p.Source] && q == contents[p.Source-1] && utf8.RuneCountInString(q) >= 2)) || utf8.RuneCountInString(q) > 600 || !strings.Contains(sourceWhitespace(contents[p.Source-1]), q) || danglingSentence(q) || strings.ContainsAny(q, "?？") || strings.Contains(q, "在学习本节") {
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
					if !strings.Contains(chosen[main], sourceWhitespace(contents[id-1])) && chosen[id] != sourceWhitespace(contents[id-1]) {
						return "", nil, false
					}
				}
			}
		}
	}
	return strings.Join(paragraphs, "\n\n"), sources, len(paragraphs) > 0
}

func sourceExcerptAnswer(contents []string, citations []domain.Citation) (string, []domain.Citation) {
	text, sources, _ := sentenceAnswer("", "", contents, citations, nil)
	return text, sources
}
