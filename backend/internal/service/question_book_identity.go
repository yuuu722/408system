package service

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var leadingBookOrder = regexp.MustCompile(`^\s*\d{1,2}[.、_-]\s*`)

// Keep edition/year/author information. A subject name cannot establish that
// an independently uploaded answer table belongs to a question booklet.
func SameQuestionBook(question, answer string) bool {
	identity := func(name string) string {
		name = strings.Map(func(r rune) rune {
			if r >= 0xff01 && r <= 0xff5e {
				return r - 0xfee0
			}
			return r
		}, name)
		name = strings.ReplaceAll(name, "\\", "/")
		name = filepath.Base(name)
		name = strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
		name = leadingBookOrder.ReplaceAllString(name, "")
		for _, term := range []string{"参考答案与解析", "答案与解析", "参考答案", "答案解析", "习题解答", "习题答案", "课后习题", "题目", "答案", "解析", "解答", "练习题", "习题", "题集", "题册", "做题本", "solutions", "answers"} {
			name = strings.ReplaceAll(name, term, "")
		}
		return strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
				return r
			}
			return -1
		}, name)
	}
	q, a := identity(question), identity(answer)
	if len([]rune(q)) < 8 || q != a {
		return false
	}
	for _, generic := range []string{"数据结构", "操作系统", "计算机操作系统", "计算机网络", "计算机组成原理", "408", "408考研"} {
		if q == generic {
			return false
		}
	}
	return true
}
