package service

import (
	"context"
	"encoding/json"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"strings"
	"unicode"
)

const factCompositionInstruction = `
把以下教材事实整理成直接回答问题的中文答案。只能使用facts及其premises，不补充模型常识，不猜改OCR、数字或公式。
只返回JSON：{"claims":[{"text":"整理好的一段完整回答。","kind":"explanation","facts":[1,2]}]}。
facts填写事实id，不是文件编号。不要抄写原文quote，出处由程序绑定；正文不写来源编号或“根据资料”。
回答原因时解释原因及机制；回答实现时按资料中的步骤讲清。资料只有机制名称时只能概括，不能凭常识扩写细节，可以说明细节依据不足。
必须保留事实中的条件和指代所需前文。计算推导使用kind=derivation并说明条件；其他使用explanation。
最多3段，合计600字，引用不超过8个事实。每段写完。能回答部分时，整理已获支持的部分，并明确说明所引用片段没有展开的细节；只有完全无法回答时返回{"claims":[]}。`

type factPremise struct {
	Source int    `json:"source"`
	Text   string `json:"text"`
}
type answerFact struct {
	ID       int           `json:"id"`
	Source   int           `json:"source"`
	Text     string        `json:"text"`
	Premises []factPremise `json:"premises,omitempty"`
}
type factCatalog struct {
	Facts     []answerFact   `json:"facts"`
	Locations map[int]string `json:"locations"`
}

func buildAnswerFacts(question string, contents []string, citations []domain.Citation, deps map[int][]int) factCatalog {
	out := factCatalog{Facts: []answerFact{}, Locations: map[int]string{}}
	contextOnly := map[int]bool{}
	for _, ids := range deps {
		for _, id := range ids {
			contextOnly[id] = true
		}
	}
	budget := 2200
	seen := map[string]bool{}
	for _, candidate := range sentenceCandidates(question, contents, citations) {
		source := candidate.Source
		if source < 1 || source > len(citations) || contextOnly[source] && len(deps[source]) == 0 {
			continue
		}
		if strings.Contains(candidate.Text, "这里不再") || strings.Contains(candidate.Text, "此处不再") {
			continue
		}
		if seen[candidate.Text] {
			continue
		}
		premises := []factPremise{}
		cost := len([]rune(candidate.Text))
		valid := true
		for _, id := range deps[source] {
			if id < 1 || id > len(contents) || id > len(citations) {
				valid = false
				break
			}
			text := sourceWhitespace(contents[id-1])
			if text == "" || strings.ContainsRune(text, '�') {
				valid = false
				break
			}
			premises = append(premises, factPremise{id, text})
			cost += len([]rune(text))
		}
		for _, prefix := range []string{"因此", "所以", "这会", "这些", "它们", "由此", "由以上", "该过程"} {
			if strings.HasPrefix(candidate.Text, prefix) {
				if candidate.Previous == "" {
					valid = false
				} else {
					premises = append(premises, factPremise{source, candidate.Previous})
					cost += len([]rune(candidate.Previous))
				}
				break
			}
		}
		if !valid || cost > budget {
			continue
		}
		budget -= cost
		seen[candidate.Text] = true
		out.Facts = append(out.Facts, answerFact{ID: len(out.Facts) + 1, Source: source, Text: candidate.Text, Premises: premises})
		out.Locations[source] = citations[source-1].Location
		for _, p := range premises {
			out.Locations[p.Source] = citations[p.Source-1].Location
		}
		if len(out.Facts) >= 10 {
			break
		}
	}
	return out
}
func (c factCatalog) JSON() string {
	facts := []map[string]any{}
	for _, f := range c.Facts {
		view := map[string]any{"id": f.ID, "text": f.Text, "location": c.Locations[f.Source]}
		if len(f.Premises) > 0 {
			p := []string{}
			for _, v := range f.Premises {
				p = append(p, v.Text)
			}
			view["premises"] = p
		}
		facts = append(facts, view)
	}
	raw, _ := json.Marshal(map[string]any{"facts": facts})
	return string(raw)
}

// The model emits references, never verbatim quotations. Exact source text and
// mandatory premises are reconstructed exclusively from the server catalog.
func (s RAGService) factComposedAnswer(ctx context.Context, raw, question string, catalog factCatalog, contents []string, citations []domain.Citation, deps map[int][]int) (string, []domain.Citation, bool, string) {
	var data struct {
		Claims []struct {
			Text     string          `json:"text"`
			Kind     string          `json:"kind"`
			Facts    []int           `json:"facts"`
			Evidence []quotedPassage `json:"evidence"`
		} `json:"claims"`
	}
	if json.Unmarshal([]byte(modelJSONObject(raw)), &data) != nil {
		return "", nil, false, "invalid_json"
	}
	if len(data.Claims) == 0 {
		return "", nil, false, "model_abstained"
	}
	if len(data.Claims) > 3 {
		return "", nil, false, "excess_claims"
	}
	claims := []groundedClaim{}
	reviewClaims := []map[string]any{}
	selected := map[int]bool{}
	for _, claim := range data.Claims {
		// Strict compatibility for older providers; the old quote verifier remains.
		if len(claim.Facts) == 0 && len(claim.Evidence) > 0 {
			text, sources, ok, why := s.composedAnswerDetail(ctx, raw, question, contents, citations, deps)
			return text, sources, ok, why
		}
		if len(claim.Facts) == 0 || len(claim.Facts) > 8 {
			return "", nil, false, "missing_fact_ids"
		}
		text := strings.TrimSpace(claim.Text)
		r := []rune(text)
		// Missing final punctuation is formatting, not permission to complete a tail.
		if len(r) >= 12 && unicode.Is(unicode.Han, r[len(r)-1]) && completeAnswerText(text+"。") {
			text += "。"
		}
		kind := strings.ToLower(strings.TrimSpace(claim.Kind))
		if kind == "" {
			kind = "explanation"
		}
		grounded := groundedClaim{Text: text, Kind: kind}
		seen := map[string]bool{}
		add := func(source int, text string) {
			key := string(rune(source)) + "\x00" + text
			if !seen[key] {
				grounded.Evidence = append(grounded.Evidence, quotedPassage{source, text})
				seen[key] = true
			}
		}
		for _, id := range claim.Facts {
			if id < 1 || id > len(catalog.Facts) {
				return "", nil, false, "unknown_fact_id"
			}
			selected[id] = true
			fact := catalog.Facts[id-1]
			add(fact.Source, fact.Text)
			for _, p := range fact.Premises {
				add(p.Source, p.Text)
			}
		}
		claims = append(claims, grounded)
		reviewClaims = append(reviewClaims, map[string]any{"text": text, "kind": kind, "facts": claim.Facts})
	}
	expanded, _ := json.Marshal(map[string]any{"claims": claims})
	chosen := factCatalog{Locations: catalog.Locations}
	for _, fact := range catalog.Facts {
		if selected[fact.ID] {
			chosen.Facts = append(chosen.Facts, fact)
		}
	}
	var reviewData map[string]any
	_ = json.Unmarshal([]byte(chosen.JSON()), &reviewData)
	reviewData["question"] = question
	reviewData["claims"] = reviewClaims
	reviewPayload, _ := json.Marshal(reviewData)
	text, sources, ok, why := s.composedAnswerDetail(ctx, string(expanded), question, contents, citations, deps, string(reviewPayload))
	if !ok {
		return "", nil, false, why
	}
	return text, sources, true, ""
}

// A small output grammar prevents local models from omitting fact references.
// Providers without JSON-schema support fall back to ordinary JSON mode.
func factResponseFormat(evidence string) map[string]any {
	var catalog struct {
		Facts []json.RawMessage `json:"facts"`
	}
	_ = json.Unmarshal([]byte(evidence), &catalog)
	ref := map[string]any{"type": "integer", "minimum": 1}
	if len(catalog.Facts) > 0 {
		ref["maximum"] = len(catalog.Facts)
	}
	claim := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"text", "kind", "facts"}, "properties": map[string]any{
		"text": map[string]any{"type": "string"}, "kind": map[string]any{"type": "string", "enum": []string{"explanation", "derivation"}}, "facts": map[string]any{"type": "array", "minItems": 1, "maxItems": 8, "items": ref},
	}}
	return map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "grounded_answer", "strict": true, "schema": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"claims"}, "properties": map[string]any{"claims": map[string]any{"type": "array", "maxItems": 3, "items": claim}}}}}
}
