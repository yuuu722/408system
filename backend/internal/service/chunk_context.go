package service

import (
	"encoding/json"
	"fmt"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"regexp"
	"strings"
)

type ChunkContext struct {
	Text    string `json:"text"`
	Page    int    `json:"page"`
	Segment int    `json:"segment"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
	Kind    string `json:"kind"`
}

var chapterHeading = regexp.MustCompile(`^(第[一二三四五六七八九十百0-9]+[章节篇]|[0-9]+\.[0-9]+(?:\.[0-9]+)?\s*[^0-9])`)
var premiseSentence = regexp.MustCompile(`假设|前提|已知|^设[^备]|设有|设某|条件为|若|如果|对于|当.+时|在.+(?:系统|方式|情况下|模型)中`)

// Source text is never prefixed with generated summaries: structural context is
// stored separately with exact source offsets and is independently cited.
func ContextualChunks(pages []TextPage, rid, space, title string) []domain.Chunk {
	out := []domain.Chunk{}
	heading := ""
	previousHeading := ""
	chapter, section := "", ""
	updateHeading := func(line string) string {
		if strings.HasPrefix(line, "第") && (strings.Contains(line, "章") || strings.Contains(line, "篇")) {
			chapter = line
			section = ""
		} else {
			section = line
		}
		return strings.Trim(strings.Join([]string{chapter, section}, " / "), " / ")
	}
	for i, page := range pages {
		chunks := MappedChunks(page, rid, space)
		for j := range chunks {
			c := &chunks[j]
			start := c.Metadata["start"].(int)
			before := string([]rune(page.Text)[:start])
			localHeading := heading
			for _, line := range strings.Split(before, "\n") {
				line = strings.TrimSpace(line)
				if len([]rune(line)) <= 80 && chapterHeading.MatchString(line) && !strings.ContainsAny(line, "。！？；") {
					localHeading = updateHeading(line)
				}
			}
			for _, line := range strings.Split(c.Content, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				if len([]rune(line)) <= 80 && chapterHeading.MatchString(line) && !strings.ContainsAny(line, "。！？；") {
					localHeading = updateHeading(line)
				} else {
					break
				}
			}

			contexts := []ChunkContext{}
			prefix := []rune(before)
			offset := max(0, len(prefix)-500)
			if localHeading == heading {
				contexts = append(contexts, contextSpans(page, string(prefix[offset:]), offset)...)
			}
			contexts = append(contexts, contextSpans(page, c.Content, start)...)
			// Only mark self-contained short condition sentences. A long or
			// truncated premise is not silently rewritten into a short claim.
			// Carry only a bounded, literal previous-page continuation, never across a
			// detected new heading. Preserve the immediately preceding complete context.
			if start == 0 && i > 0 && localHeading == previousHeading {
				prev := pages[i-1]
				r := []rune(prev.Text)
				from := max(0, len(r)-300)
				for from > 0 && from < len(r) && !strings.ContainsRune("。！？；\n", r[from-1]) {
					from++
				}
				text := strings.TrimSpace(string(r[from:]))
				if text != "" {
					idx := strings.Index(prev.Text, text)
					begin := len([]rune(prev.Text[:idx]))
					contexts = append([]ChunkContext{{Text: text, Page: prev.Number, Segment: prev.Segment, Start: begin, End: begin + len([]rune(text)), Kind: "preceding_page"}}, contexts...)
				}
			}
			if len(contexts) > 2 {
				contexts = contexts[len(contexts)-2:]
			}
			lead := strings.TrimSpace(c.Content)
			depends := false
			for _, word := range []string{"因此", "所以", "由此", "上述", "此时", "该条件", "在该条件", "这时", "其结果"} {
				if strings.HasPrefix(lead, word) {
					depends = true
				}
			}
			if depends && len(contexts) == 0 {
				c.Metadata["context_incomplete"] = true
			}
			c.Metadata["document_title"] = title
			c.Metadata["heading_path"] = localHeading
			c.Metadata["context_passages"] = contexts
			heading = localHeading
		}
		previousHeading = heading
		out = append(out, chunks...)
	}
	return out
}
func contextSpans(page TextPage, text string, offset int) []ChunkContext {
	out := []ChunkContext{}
	r := []rune(text)
	start := 0
	for end, ch := range r {
		if strings.ContainsRune("。！？；\n", ch) || end == len(r)-1 {
			part := string(r[start : end+1])
			trim := strings.TrimSpace(part)
			if trim != "" && len([]rune(trim)) <= 240 && premiseSentence.MatchString(trim) {
				leading := len([]rune(part)) - len([]rune(strings.TrimLeft(part, " \t\r\n")))
				begin := offset + start + leading
				out = append(out, ChunkContext{Text: trim, Page: page.Number, Segment: page.Segment, Start: begin, End: begin + len([]rune(trim)), Kind: "premise"})
			}
			start = end + 1
		}
	}
	return out
}
func chunkIndexText(c domain.Chunk) string {
	text, _ := c.Metadata["document_title"].(string)
	heading, _ := c.Metadata["heading_path"].(string)
	var contexts []ChunkContext
	raw, _ := json.Marshal(c.Metadata["context_passages"])
	_ = json.Unmarshal(raw, &contexts)
	contextText := ""
	for i, v := range contexts {
		if i >= 2 {
			break
		}
		contextText += "\n" + v.Text
	}
	return text + "\n" + heading + contextText + "\n" + c.Content
}

func premiseChunks(c domain.Chunk) []domain.Chunk {
	raw, _ := json.Marshal(c.Metadata["context_passages"])
	var contexts []ChunkContext
	_ = json.Unmarshal(raw, &contexts)
	out := []domain.Chunk{}
	for _, v := range contexts {
		if v.Text == "" || len([]rune(v.Text)) > 360 || choiceOption.MatchString(v.Text) {
			continue
		}
		meta := map[string]any{"page": v.Page, "revision_id": c.Metadata["revision_id"], "regions": []domain.SourceRegion{{Page: v.Page, Segment: v.Segment, Start: v.Start, End: v.End}}}
		out = append(out, domain.Chunk{ID: c.ID, DocumentID: c.DocumentID, DocumentTitle: c.DocumentTitle, Content: v.Text, SourceLocation: fmt.Sprintf("前提上下文 · 第%d页", v.Page), Metadata: meta, Score: c.Score})
	}
	if len(out) > 2 {
		out = out[:2]
	}
	return out
}
