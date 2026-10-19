package config

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// An endpoint/model change requires reprocessing; keys deliberately do not identify a space.
func (c Config) EmbeddingSpace() string {
	if c.UsesLocalIndex() {
		return fmt.Sprintf("lexical-ngram-v1-%d", c.EmbeddingDimension)
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|%t", strings.TrimRight(c.EmbeddingBaseURL, "/"), c.EmbeddingModel, c.EmbeddingDimension, c.DemoMode))))
}

// Lexical indexing is independent of LLM credentials; remote errors never silently switch spaces.
func (c Config) UsesLocalIndex() bool {
	return !c.DemoMode && (c.EmbeddingMode == "lexical" || c.EmbeddingMode == "auto" && strings.TrimSpace(c.EmbeddingAPIKey) == "")
}
