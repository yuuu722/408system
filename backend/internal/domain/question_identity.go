package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/local/exam408-assistant/backend/internal/sourcequality"
	"regexp"
	"strings"
)

const PublicQuestionFamilyPrefix = "public-question-v1:"

var questionOperatorSpace = regexp.MustCompile(`\s*([+*/=<>(),;:，。{}-]|\[|\])\s*`)

func QuestionContentFamily(a Asset) string {
	if a.Kind != "question" || strings.TrimSpace(a.Payload.Stem) == "" || len(a.Payload.Options) < 2 {
		return ""
	}
	for _, term := range []string{"如图", "下图", "图示"} {
		if strings.Contains(a.Payload.Stem, term) && len(a.Payload.Images) == 0 {
			return ""
		}
	}
	norm := func(s string) string {
		s = strings.Join(strings.Fields(s), " ")
		s = sourcequality.Normalize(s)
		return questionOperatorSpace.ReplaceAllString(s, "$1")
	}
	options := []string{}
	for _, x := range a.Payload.Options {
		if strings.TrimSpace(x) == "" {
			return ""
		}
		options = append(options, norm(x))
	}
	images := append([]string{}, a.Payload.Images...)
	raw, _ := json.Marshal([]any{norm(a.Payload.Stem), options, images})
	hash := sha256.Sum256(raw)
	return PublicQuestionFamilyPrefix + hex.EncodeToString(hash[:])
}
