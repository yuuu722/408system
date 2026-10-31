package service

import (
	"encoding/json"
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const sentenceSelectionInstruction = `
请从本轮“完整原句目录”中挑选能直接回答用户问题的句子，并按解释顺序排列。只返回JSON：{"selected":[1,3]}。数字是句子编号，不是文件编号。优先选择直接解释问题、文字完整清晰的句子，不选择导读或明显OCR乱码。最多选4句，不要抄写句子，不要解释选择过程，不得自行补充事实。前提由系统保留。没有合适句子则返回{"selected":[]}。`

var numberedLine = regexp.MustCompile(`^\s*[0-9]+(?:[)）、]|[.．]\s+)\s*`)
var numericHeading = regexp.MustCompile(`^[0-9]+(?:[.．][0-9]+)*[.．]?\s*[\p{Han}A-Za-z][^。！？；?]{0,60}$`)
var cnWhitespace = regexp.MustCompile(`([\p{Han}])\s+([\p{Han}])`)
var cnContinuation = regexp.MustCompile(`([\p{Han}，。；：、])\\\r?\n`)
var pageNoise = regexp.MustCompile(`^(?:[0-9A-F]+H\s*[|｜]|[·—\s]*第\s*[0-9]+\s*页|公众号[:：])`)

type answerSentence struct {
	Source   int
	Text     string
	Previous string
	Score    float64
}

func sourceWhitespace(text string) string {
	text = cnContinuation.ReplaceAllString(text, "$1\n")
	text = strings.Join(strings.Fields(text), " ")
	for i := 0; i < 2; i++ {
		text = cnWhitespace.ReplaceAllString(text, "$1$2")
	}
	return strings.TrimSpace(text)
}
func danglingSentence(text string) bool {
	text = strings.TrimSpace(text)
	if strings.HasSuffix(text, "，") || strings.HasSuffix(text, ",") || strings.HasSuffix(text, "：") || strings.HasSuffix(text, ":") {
		return true
	}
	for _, end := range []string{"所以", "因此", "由于", "因为", "从而", "使得", "以及", "则"} {
		if strings.HasSuffix(text, end) {
			return true
		}
	}
	return false
}
func completeSourceSentences(content string) []string {
	var cleaned strings.Builder
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		chars := []rune(line)
		if len(chars) > 1 && chars[len(chars)-1] == '\\' && (unicode.Is(unicode.Han, chars[len(chars)-2]) || strings.ContainsRune("，。；：、", chars[len(chars)-2])) {
			line = string(chars[:len(chars)-1])
		}
		if mark := strings.LastIndexAny(line, "?？"); mark >= 0 {
			_, width := utf8.DecodeRuneInString(line[mark:])
			line = strings.TrimSpace(line[mark+width:])
		}
		if line == "" {
			cleaned.WriteString("\n\n")
			continue
		}
		if strings.HasPrefix(line, "答案如下") || strings.HasPrefix(line, "在学习本节") || strings.HasPrefix(line, "读者要掌握") || pageNoise.MatchString(line) || ((chapterHeading.MatchString(line) || numericHeading.MatchString(line)) && !strings.ContainsAny(line, "。！？；")) || strings.ContainsAny(line, "?？") {
			cleaned.WriteString("\n\n")
			continue
		}
		cleaned.WriteString(numberedLine.ReplaceAllString(line, ""))
		cleaned.WriteByte(' ')
	}
	out := []string{}
	add := func(text string) {
		text = sourceWhitespace(text)
		if len([]rune(text)) < 8 || len([]rune(text)) > 450 || danglingSentence(text) || strings.ContainsAny(text, "?？") {
			return
		}
		out = append(out, text)
	}
	for _, paragraph := range strings.Split(cleaned.String(), "\n\n") {
		runes := []rune(paragraph)
		start := 0
		for i, r := range runes {
			if strings.ContainsRune("。！？；", r) || (r == '.' && (i+1 == len(runes) || unicode.IsSpace(runes[i+1])) && (i == 0 || !unicode.IsDigit(runes[i-1]))) {
				add(string(runes[start : i+1]))
				start = i + 1
			}
		}
		tail := strings.TrimSpace(string(runes[start:]))
		if len([]rune(tail)) <= 180 && !danglingSentence(tail) {
			for _, verb := range []string{"用于", "通过", "提供", "利用", "是", "可以", "允许", "不必", "必须"} {
				if strings.Contains(tail, verb) {
					add(tail)
					break
				}
			}
		}
	}
	return out
}
func sentenceCandidates(question string, contents []string) []answerSentence {
	out := []answerSentence{}
	seen := map[string]bool{}
	why := strings.Contains(question, "为什么") || strings.Contains(question, "原因") || strings.Contains(question, "必要")
	for i, content := range contents {
		parts := completeSourceSentences(content)
		for position, text := range parts {
			if why && !strings.Contains(question, "实现") && (strings.Contains(text, "实现需要建立") || strings.Contains(text, "实现必须建立") || strings.Contains(text, "采用连续分配方式")) {
				continue
			}
			key := strings.Map(func(r rune) rune {
				if unicode.IsSpace(r) {
					return -1
				}
				return r
			}, text)
			if seen[key] {
				continue
			}
			seen[key] = true
			score := lexicalEvidenceScore(question, text)
			if question == "" {
				score = .2
			}
			if why {
				for _, term := range []string{"不足", "无法运行", "不能运行", "不能全部", "局部性", "暂时不用", "并发度", "占据"} {
					if strings.Contains(text, term) {
						score += .8
					}
				}
			}
			previous := ""
			if position > 0 {
				previous = parts[position-1]
			}
			out = append(out, answerSentence{Source: i + 1, Text: text, Previous: previous, Score: score})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > 16 {
		out = out[:16]
	}
	return out
}
func sentenceCatalog(question string, contents []string) string {
	var out strings.Builder
	for i, c := range sentenceCandidates(question, contents) {
		fmt.Fprintf(&out, "句子%d（来源%d）：%s\n", i+1, c.Source, c.Text)
	}
	return out.String()
}
func parseSelection(raw string) []int {
	a, b := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
	if a < 0 || b < a {
		return nil
	}
	var data struct {
		Selected []int `json:"selected"`
	}
	if json.Unmarshal([]byte(raw[a:b+1]), &data) != nil {
		return nil
	}
	return data.Selected
}

// Model text is never treated as an answer. It can only select deterministic,
// source-backed sentences. A malformed selection uses bounded source ranking.
func sentenceAnswer(raw, question string, contents []string, citations []domain.Citation, dependencies map[int][]int) (string, []domain.Citation, bool) {
	candidates := sentenceCandidates(question, contents)
	ids := parseSelection(raw)
	model := len(ids) > 0 && len(ids) <= 4
	if model {
		for _, id := range ids {
			if id < 1 || id > len(candidates) {
				model = false
				break
			}
		}
	}
	contextSources := map[int]bool{}
	for _, values := range dependencies {
		for _, value := range values {
			contextSources[value] = true
		}
	}
	if !model {
		ids = nil
		for i, c := range candidates {
			if contextSources[c.Source] && len(dependencies[c.Source]) == 0 {
				continue
			}
			if len(ids) >= 3 {
				break
			}
			if len(ids) > 0 && c.Score < candidates[0].Score-.35 {
				continue
			}
			ids = append(ids, i+1)
		}
	}
	paragraphs := []string{}
	sources := []domain.Citation{}
	seen := map[string]bool{}
	sourceIndex := map[string]int{}
	budget := 1100
	add := func(text string, source int) bool {
		text = sourceWhitespace(text)
		if text == "" || seen[text] {
			return true
		}
		if source < 1 || source > len(citations) || len([]rune(text)) > budget {
			return false
		}
		seen[text] = true
		budget -= len([]rune(text))
		paragraphs = append(paragraphs, text)
		ref := citations[source-1]
		ref.Excerpt = text
		key := ref.DocumentID + ":" + ref.RevisionID + fmt.Sprintf(":%d", ref.Page)
		if ref.Page == 0 {
			key += ":" + ref.ChunkID
		}
		if pos, ok := sourceIndex[key]; ok {
			sources[pos].Excerpt += "\n" + text
			sources[pos].Regions = append(sources[pos].Regions, ref.Regions...)
		} else {
			sourceIndex[key] = len(sources)
			sources = append(sources, ref)
		}
		return true
	}
	for _, id := range ids {
		candidate := candidates[id-1]
		if contextSources[candidate.Source] && len(dependencies[candidate.Source]) == 0 {
			continue
		}
		conditions := []struct {
			text   string
			source int
		}{}
		valid := true
		for _, prefix := range []string{"这会", "这些", "它们", "因此", "所以", "由此", "由以上", "该过程"} {
			if strings.HasPrefix(candidate.Text, prefix) {
				if candidate.Previous == "" {
					valid = false
				} else {
					conditions = append(conditions, struct {
						text   string
						source int
					}{candidate.Previous, candidate.Source})
				}
				break
			}
		}
		for _, required := range dependencies[candidate.Source] {
			if required < 1 || required > len(contents) {
				valid = false
				break
			}
			parts := completeSourceSentences(contents[required-1])
			if len(parts) == 0 {
				condition := sourceWhitespace(contents[required-1])
				if len([]rune(condition)) >= 2 && len([]rune(condition)) <= 240 && !danglingSentence(condition) {
					parts = []string{condition}
				}
			}
			if len(parts) == 0 {
				valid = false
				break
			}
			conditions = append(conditions, struct {
				text   string
				source int
			}{strings.Join(parts, " "), required})
		}
		if !valid {
			continue
		}
		needed := len([]rune(candidate.Text))
		for _, p := range conditions {
			if !seen[p.text] && !strings.Contains(candidate.Text, p.text) {
				needed += len([]rune(p.text))
			}
		}
		if needed > budget {
			continue
		}
		for _, p := range conditions {
			if !strings.Contains(candidate.Text, p.text) {
				add(p.text, p.source)
			}
		}
		add(candidate.Text, candidate.Source)
	}
	if len(paragraphs) == 0 {
		return "当前检索结果缺少完整、可靠的讲解依据，暂时无法回答。请查看原文或补充相关教材。", nil, false
	}
	return strings.Join(paragraphs, "\n\n"), sources, model
}
