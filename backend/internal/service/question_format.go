package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"regexp"
	"strings"
)

var spacedChapter = regexp.MustCompile(`^第\s*([一二三四五六七八九十百0-9]+)\s*([章节])`)
var questionFooter = regexp.MustCompile(`^[·•—\-\s]*(?:第\s*[0-9]+\s*页(?:[，, ]*共\s*[0-9]+\s*页|\s*[（(]共\s*[0-9]+\s*页[）)])?|[0-9]+\s*页)[·•—\-\s]*$`)
var trailingBookReference = regexp.MustCompile(`[（(]?答案\s*见\s*(?:原书|教材|课本|本书).*$`)
var explicitAnswerSection = regexp.MustCompile(`^(?:(?:参考)?答案(?:与|及|和)?解析|(?:参考)?答案|习题解答|答案详解|试题解析|解析)[:：]?(?:[·.。…\s]*[0-9]+)?$`)
var fullwidthQuestionText = strings.NewReplacer("０", "0", "１", "1", "２", "2", "３", "3", "４", "4", "５", "5", "６", "6", "７", "7", "８", "8", "９", "9", "Ａ", "A", "Ｂ", "B", "Ｃ", "C", "Ｄ", "D", "Ｅ", "E", "Ｆ", "F", "　", " ")

func questionLine(line string) string { return strings.TrimSpace(fullwidthQuestionText.Replace(line)) }

// A reference to the answer page describes the question section. It is not an
// instruction to switch all following question text into answer mode.
func questionHeading(line string) (scope string, answer bool, ok bool) {
	line = questionLine(line)
	if len([]rune(line)) >= 80 || strings.ContainsAny(line, "。！？?") {
		return
	}
	if m := spacedChapter.FindStringSubmatchIndex(line); m != nil {
		prefix := "第" + line[m[2]:m[3]] + line[m[4]:m[5]]
		line = prefix + line[m[1]:]
	}
	h := questionSection.FindString(line)
	if h == "" {
		return
	}
	label := strings.TrimSpace(line[len(h):])
	scope = h
	ok = true
	reference := trailingBookReference.MatchString(label)
	clean := strings.TrimSpace(trailingBookReference.ReplaceAllString(label, ""))
	answer = !reference && explicitAnswerSection.MatchString(clean)
	// Exercise and answer subsections of the same textbook topic are siblings,
	// e.g. 1.1.4 exercises and 1.1.5 answers both belong to topic 1.1.
	parts := strings.Split(h, ".")
	if len(parts) >= 3 && (answer || strings.Contains(clean, "本节习题") || strings.Contains(clean, "本节练习")) {
		scope = strings.Join(parts[:len(parts)-1], ".")
	}
	return
}

func questionBoilerplate(line string) bool {
	line = questionLine(line)
	return strings.HasPrefix(line, "公众号：") || strings.HasPrefix(line, "公众号:") || questionFooter.MatchString(line)
}

var runningChapterNumber = regexp.MustCompile(`^第\s*([0-9]+)\s*章.*[ \t]+[0-9]+$`)

func runningQuestionHeader(line, scope string) bool {
	m := runningChapterNumber.FindStringSubmatch(questionLine(line))
	if m == nil || scope == "" {
		return false
	}
	chapter := strings.Split(scope, ".")[0]
	if strings.HasPrefix(chapter, "第") {
		chapter = strings.TrimSuffix(strings.TrimPrefix(chapter, "第"), "章")
	}
	return canonicalAnswerNumber(chapter) == canonicalAnswerNumber(m[1])
}

// Re-extraction can improve whitespace/footer handling and hence change a
// content hash. An already published question at the same source anchor must
// not be published again merely because that derived hash changed.
func SameQuestionSourceAnchor(a, b domain.Asset) bool {
	if a.Kind != "question" || b.Kind != "question" || a.Payload.DocumentID == "" || a.Payload.DocumentID != b.Payload.DocumentID || a.Payload.Page <= 0 || a.Payload.Page != b.Payload.Page || a.Payload.QuestionNumber == "" || b.Payload.QuestionNumber == "" {
		return false
	}
	if canonicalAnswerNumber(a.Payload.QuestionNumber) != canonicalAnswerNumber(b.Payload.QuestionNumber) {
		return false
	}
	x, y := a.Payload.SectionKey, b.Payload.SectionKey
	return x == y || (x != "" && y != "" && (strings.HasPrefix(x, y+".") || strings.HasPrefix(y, x+".")))
}

func questionStemCue(text string) bool {
	if strings.ContainsAny(text, "?？") {
		return true
	}
	for _, term := range []string{"下列", "以下", "哪项", "哪个", "正确的是", "错误的是", "选择题", "( )", "（ ）", "()", "（）"} {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}
