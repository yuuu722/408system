package config

import "testing"

func TestEmbeddingSpaceIsolation(t *testing.T) {
	c := Config{EmbeddingBaseURL: "http://localhost/v1", EmbeddingModel: "local-v1", EmbeddingDimension: 768}
	original := c.EmbeddingSpace()
	c.EmbeddingAPIKey = "rotated-secret"
	if c.EmbeddingSpace() != original {
		t.Fatal("key rotation should preserve vectors")
	}
	c.DemoMode = true
	if c.EmbeddingSpace() == original {
		t.Fatal("demo and real vectors must be isolated")
	}
	c.DemoMode = false
	c.EmbeddingModel = "local-v2"
	if c.EmbeddingSpace() == original {
		t.Fatal("model revisions must be isolated")
	}
}

func TestLocalEmbeddingDimension(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("EMBEDDING_DIMENSION", "768")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EMBEDDING_DIMENSION", "0")
	if _, err := Load(); err == nil {
		t.Fatal("invalid dimension accepted")
	}
}
