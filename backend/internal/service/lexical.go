package service

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"unicode"
)

// LexicalEmbedding is a local feature-hashing index, not a learned semantic
// model. Chinese bigrams and word features need neither a network nor API keys.
type LexicalEmbedding struct{ Dimension int }

func (LexicalEmbedding) IsLocalIndex() bool { return true }

func (p LexicalEmbedding) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if p.Dimension < 1 {
		return nil, fmt.Errorf("invalid lexical dimension")
	}
	out := make([][]float32, len(texts))
	for i, text := range texts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		v := make([]float32, p.Dimension)
		add := func(token string) {
			h := sha256.Sum256([]byte(token))
			idx := int(binary.BigEndian.Uint32(h[:4]) % uint32(p.Dimension))
			v[idx]++
		}
		word := []rune{}
		flush := func() {
			if len(word) == 0 {
				return
			}
			add(string(word))
			for j := 0; j < len(word)-1; j++ {
				add(string(word[j : j+2]))
			}
			word = nil
		}
		for _, ch := range strings.ToLower(text) {
			if unicode.IsLetter(ch) || unicode.IsDigit(ch) {
				word = append(word, ch)
			} else {
				flush()
			}
		}
		flush()
		norm := float64(0)
		for j, x := range v {
			if x > 0 {
				v[j] = 1 + float32(math.Log(float64(x)))
			}
			norm += float64(v[j]) * float64(v[j])
		}
		if norm == 0 {
			add("empty-text")
			norm = 1
		}
		for j := range v {
			v[j] /= float32(math.Sqrt(norm))
		}
		out[i] = v
	}
	return out, nil
}
