package service

import (
	"github.com/local/exam408-assistant/backend/internal/domain"
	"regexp"
	"strings"
)

// Filename rules suggest metadata, never silently approve it or invent a subject.
func SuggestDocumentCatalog(d domain.Document) domain.DocumentCatalog {
	c := domain.DocumentCatalog{DocumentID: d.ID, ResourceType: d.ResourceType, Purpose: "knowledge", ReviewState: "pending", Reason: "按文件名建议；请管理员核对分类与用途"}
	text := strings.ToLower(d.OriginalFilename)
	rules := []struct{ kind, pattern string }{
		{"outline", `考纲|大纲|syllabus`}, {"solution", `答案|解析|详解|解答`},
		{"past_exam", `真题|模拟|试卷|习题|题集|题册|做题本|选择题|综合题|能力评估`},
		{"textbook", `教材|课本|复习全书|高清带书签|电子版|王道.*(数据结构|操作系统|计算机)|计算机.*第.*版`},
		{"courseware", `课件|讲义|幻灯`}, {"notes", `笔记|总结|知识点|提纲`},
	}
	if c.ResourceType == "" || c.ResourceType == "other" || regexp.MustCompile(`考纲|大纲`).MatchString(text) {
		c.ResourceType = "other"
		for _, r := range rules {
			if regexp.MustCompile(r.pattern).MatchString(text) {
				c.ResourceType = r.kind
				break
			}
		}
	}
	subjects := []struct{ code, pattern string }{{"ds", `数据结构|data.structure|\bds\b`}, {"co", `组成原理|计算机组成|\bco\b`}, {"os", `操作系统|operating.system|\bos\b`}, {"cn", `计算机网络|computer.network|\bcn\b`}}
	count := 0
	for _, r := range subjects {
		if regexp.MustCompile(r.pattern).MatchString(text) {
			c.SubjectCode = r.code
			count++
		}
	}
	if count != 1 {
		c.SubjectCode = ""
	}
	if c.ResourceType == "past_exam" {
		c.Purpose = "questions"
	}
	return c
}
