package config

import (
	"strings"
	"testing"
)

func setRequiredEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://example.invalid/exam408")
	t.Setenv("EMBEDDING_DIMENSION", "1536")
	t.Setenv("AI_DEMO_MODE", "false")
	t.Setenv("RAG_MIN_SCORE", "0.35")
}

func TestAssistantStatusModes(t *testing.T) {
	if (Config{}).AssistantStatus().Enabled {
		t.Fatal("missing keys must disable chat")
	}
	if (Config{LLMAPIKey: " ", EmbeddingAPIKey: "x"}).AssistantStatus().Enabled {
		t.Fatal("whitespace key must disable chat")
	}
	if (Config{DemoMode: true}).AssistantStatus().Mode != "demo" {
		t.Fatal("explicit demo mode missing")
	}
	if (Config{LLMAPIKey: "x", EmbeddingAPIKey: "y"}).AssistantStatus().Mode != "model" {
		t.Fatal("configured mode missing")
	}
}

func TestLocalIndexOnlyRequiresAnswerModel(t *testing.T) {
	if !(Config{EmbeddingMode: "lexical", LLMAPIKey: "configured"}).AssistantStatus().Enabled {
		t.Fatal("local index incorrectly requires embedding key")
	}
	if (Config{EmbeddingMode: "remote", LLMAPIKey: "configured"}).AssistantStatus().Enabled {
		t.Fatal("remote embeddings still require a key")
	}
	if (Config{EmbeddingMode: "lexical"}).AssistantStatus().Enabled {
		t.Fatal("answer model is still required")
	}
}

func TestLoadRequiresJWTSecret(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("JWT_SECRET", "")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET is required") {
		t.Fatalf("expected missing JWT secret error, got %v", err)
	}
}

func TestLoadRejectsPlaceholderJWTSecret(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("JWT_SECRET", "change-this-to-at-least-32-random-characters")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "non-placeholder") {
		t.Fatalf("expected placeholder JWT secret error, got %v", err)
	}
}

func TestLoadAcceptsStrongJWTSecret(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	got, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.JWTSecret != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("unexpected secret in config")
	}
}

func TestStandaloneKnowledgeProcessingDefaults(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("EMBEDDING_API_KEY", "")
	t.Setenv("EMBEDDING_MODE", "auto")
	t.Setenv("MAX_UPLOAD_MB", "200")
	t.Setenv("PERSONAL_KB_ENABLED", "false")
	c, e := Load()
	if e != nil {
		t.Fatal(e)
	}
	if !c.UsesLocalIndex() || c.PersonalKBEnabled || c.MaxUploadBytes != 200<<20 {
		t.Fatalf("unexpected standalone policy")
	}
	localSpace := c.EmbeddingSpace()
	c.EmbeddingAPIKey = "configured-key"
	if c.UsesLocalIndex() || c.EmbeddingSpace() == localSpace {
		t.Fatal("remote space must be isolated")
	}
	for _, size := range []string{"513", "1024", "2048"} {
		t.Setenv("MAX_UPLOAD_MB", size)
		c, err := Load()
		if err != nil {
			t.Fatalf("large file limit %s rejected: %v", size, err)
		}
		if size == "2048" && c.MaxUploadBytes != int64(2)<<30 {
			t.Fatal("2 GiB limit overflow")
		}
	}
	t.Setenv("MAX_UPLOAD_MB", "2049")
	if _, e := Load(); e == nil {
		t.Fatal("oversized transport setting accepted")
	}
}
