package service

import (
	"encoding/json"
	"github.com/local/exam408-assistant/backend/internal/sourcequality"
	"regexp"
	"strings"
)

type comparisonSpec struct {
	Left  string `json:"left"`
	Right string `json:"right"`
}
type comparisonRow struct {
	Dimension  string `json:"dimension"`
	Left       string `json:"left"`
	Right      string `json:"right"`
	LeftFacts  []int  `json:"left_facts"`
	RightFacts []int  `json:"right_facts"`
}

var comparisonPair = regexp.MustCompile(`^(.{1,24}?)(?:与|和|跟|、|[Vv][Ss]\.?)\s*(.{1,24}?)$`)

func comparisonTargets(q string) *comparisonSpec {
	if questionIntent(q) != "比较分析" {
		return nil
	}
	q = strings.Trim(strings.TrimSpace(q), "?？。 ")
	for _, prefix := range []string{"请帮我", "请", "比较一下", "比较", "对比", "分析", "说明"} {
		q = strings.TrimSpace(strings.TrimPrefix(q, prefix))
	}
	for _, suffix := range []string{"有哪些区别", "有什么区别", "有何区别", "有什么不同", "有何不同", "的区别", "的异同", "的差异", "区别", "异同", "差异"} {
		q = strings.TrimSpace(strings.TrimSuffix(q, suffix))
	}
	match := comparisonPair.FindStringSubmatch(q)
	if len(match) != 3 {
		return nil
	}
	left, right := strings.TrimSpace(match[1]), strings.TrimSpace(match[2])
	if strings.EqualFold(left, right) || !sourcequality.TechnicalNoun(left) || !sourcequality.TechnicalNoun(right) {
		return nil
	}
	return &comparisonSpec{left, right}
}
func mentions(text, name string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(name))
}
func comparisonEvidence(q, text string) string {
	spec := comparisonTargets(q)
	if spec == nil {
		return text
	}
	out := []string{}
	for _, sentence := range completeSourceSentences(text) {
		if !mentions(sentence, spec.Left) && !mentions(sentence, spec.Right) {
			continue
		}
		// Explicit other comparisons and instructional prefaces are not evidence
		// for the selected pair. Do not discard normal definitions of either side.
		wrong := false
		for _, other := range []string{"进程与程序", "程序与进程", "自旋锁与信号量", "信号量与自旋锁"} {
			if strings.Contains(sentence, other) && !strings.Contains(spec.Left+"与"+spec.Right, other) {
				wrong = true
			}
		}
		for _, noise := range []string{"首先必须理解", "读者应掌握", "学习本章", "学习操作系统", "在动态性、并发性和独立性上也有区别"} {
			wrong = wrong || strings.Contains(sentence, noise)
		}
		if !wrong {
			out = append(out, sentence)
		}
	}
	return strings.Join(out, "\n")
}

const comparisonCompositionInstruction = `本题是比较分析。只能用facts及premises逐项比较comparison.left和comparison.right，不补充模型常识。
只返回JSON：{"comparisons":[{"dimension":"资源","left":"左侧对象的具体情况。","right":"右侧对象的具体情况。","left_facts":[1],"right_facts":[2]}]}。
每侧分别引用支持该侧结论的事实编号，不能互换主语。维度使用短名词，每侧写完整句，不能只说有区别、不同、存在差异。仅比较本题两个对象，不引入其他对象的比较。
优先覆盖证据中有完整支持的定义、资源、调度、开销、通信等维度，最多5行。保留用户级/内核级线程等适用条件，不把有条件结论绝对化。原文没给的维度不猜补。无法同时支持双方时不写该行；全部无依据则返回{"comparisons":[]}。不要写来源编号、Markdown或额外文字。`

func compositionInstruction(q string) string {
	if comparisonTargets(q) != nil {
		return comparisonCompositionInstruction
	}
	return factCompositionInstruction
}
func comparisonSchema() map[string]any {
	refs := map[string]any{"type": "array", "minItems": 1, "maxItems": 4, "items": map[string]any{"type": "integer", "minimum": 1}}
	row := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"dimension", "left", "right", "left_facts", "right_facts"}, "properties": map[string]any{"dimension": map[string]string{"type": "string"}, "left": map[string]string{"type": "string"}, "right": map[string]string{"type": "string"}, "left_facts": refs, "right_facts": refs}}
	return map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "grounded_comparison", "strict": true, "schema": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"comparisons"}, "properties": map[string]any{"comparisons": map[string]any{"type": "array", "maxItems": 5, "items": row}}}}}
}
func expandComparison(raw string, spec *comparisonSpec, catalog factCatalog) (string, []comparisonRow, string) {
	var data struct {
		Comparisons []comparisonRow `json:"comparisons"`
	}
	if json.Unmarshal([]byte(modelJSONObject(raw)), &data) != nil || len(data.Comparisons) == 0 || len(data.Comparisons) > 5 {
		return "", nil, "invalid_comparison_structure"
	}
	claims := []map[string]any{}
	seen := map[string]bool{}
	for _, row := range data.Comparisons {
		dimension := strings.TrimSpace(row.Dimension)
		if len([]rune(dimension)) < 2 || len([]rune(dimension)) > 16 || seen[dimension] {
			return "", nil, "invalid_comparison_structure"
		}
		seen[dimension] = true
		for i, side := range []string{row.Left, row.Right} {
			refs, name := row.LeftFacts, spec.Left
			if i == 1 {
				refs, name = row.RightFacts, spec.Right
			}
			other := spec.Right
			if i == 1 {
				other = spec.Left
			}
			if strings.HasPrefix(strings.TrimSpace(side), other) && !strings.HasPrefix(strings.TrimSpace(side), name) {
				return "", nil, "comparison_side_unsupported"
			}
			if len(refs) == 0 || len(refs) > 4 || !completeAnswerText(side) || vagueComparison(side) {
				return "", nil, "incomplete_comparison"
			}
			evidence := ""
			for _, id := range refs {
				if id < 1 || id > len(catalog.Facts) {
					return "", nil, "unknown_fact_id"
				}
				evidence += catalog.Facts[id-1].Text
				for _, p := range catalog.Facts[id-1].Premises {
					evidence += p.Text
				}
			}
			if !mentions(evidence, name) {
				return "", nil, "comparison_side_unsupported"
			}
			context := row.Dimension + side
			if mentions(name, "线程") {
				for _, pair := range [][2]string{{"用户级线程", "用户线程"}, {"内核级线程", "内核线程"}} {
					other := "用户级线程"
					if pair[0] == other {
						other = "内核级线程"
					}
					if strings.Contains(evidence, pair[0]) && !strings.Contains(evidence, other) && !strings.Contains(context, pair[0]) && !strings.Contains(context, pair[1]) {
						return "", nil, "missing_premise"
					}
				}
				if strings.Contains(evidence, "同一进程") && strings.Contains(side, "共享") {
					scoped := false
					for _, term := range []string{"同一进程", "同进程", "进程内", "所属进程", "同属"} {
						scoped = scoped || strings.Contains(context, term)
					}
					if !scoped {
						return "", nil, "missing_premise"
					}
				}
			}

		}
		text := dimension + "：\n" + spec.Left + "：" + row.Left + "\n" + spec.Right + "：" + row.Right
		ids := []int{}
		unique := map[int]bool{}
		for _, id := range append(append([]int{}, row.LeftFacts...), row.RightFacts...) {
			if !unique[id] {
				ids = append(ids, id)
				unique[id] = true
			}
		}
		claims = append(claims, map[string]any{"text": text, "kind": "explanation", "facts": ids})
	}
	encoded, _ := json.Marshal(map[string]any{"claims": claims})
	return string(encoded), data.Comparisons, ""
}
func vagueComparison(s string) bool {
	for _, word := range []string{"有所不同", "存在差异", "有区别", "也有区别", "不一样"} {
		if strings.Contains(s, word) {
			return true
		}
	}
	return false
}

func renderComparison(spec *comparisonSpec, rows []comparisonRow) string {
	cell := func(s string) string {
		return strings.NewReplacer("|", "\\|", "\n", " ", "\r", " ").Replace(strings.TrimSpace(s))
	}
	var out strings.Builder
	out.WriteString("| 比较维度 | " + cell(spec.Left) + " | " + cell(spec.Right) + " |\n|---|---|---|\n")
	for _, row := range rows {
		out.WriteString("| " + cell(row.Dimension) + " | " + cell(row.Left) + " | " + cell(row.Right) + " |\n")
	}
	return strings.TrimSpace(out.String())
}
