package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DocumentJobTimeout                                                      time.Duration
	HTTPAddr, DatabaseURL, RedisAddr, RedisPassword, JWTSecret, StorageRoot string
	AdminBootstrapToken                                                     string
	EmbeddingMode                                                           string
	PersonalKBEnabled                                                       bool
	LLMBaseURL, LLMAPIKey, LLMModel                                         string
	EmbeddingBaseURL, EmbeddingAPIKey, EmbeddingModel                       string
	CORSOrigins                                                             []string
	JWTTTL                                                                  time.Duration
	MaxUploadBytes                                                          int64
	EmbeddingDimension, RAGTopK                                             int
	DemoMode                                                                bool
	RAGMinScore                                                             float64
}

func Load() (Config, error) {
	demo, err := strconv.ParseBool(env("AI_DEMO_MODE", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("AI_DEMO_MODE: %w", err)
	}
	minScore, err := strconv.ParseFloat(env("RAG_MIN_SCORE", "0.35"), 64)
	if err != nil || !(minScore > 0 && minScore <= 1) {
		return Config{}, fmt.Errorf("RAG_MIN_SCORE must be in (0, 1]")
	}
	ttl, err := time.ParseDuration(env("JWT_TTL", "24h"))
	if err != nil {
		return Config{}, fmt.Errorf("JWT_TTL: %w", err)
	}
	dim, err := strconv.Atoi(env("EMBEDDING_DIMENSION", "1536"))
	if err != nil {
		return Config{}, fmt.Errorf("EMBEDDING_DIMENSION: %w", err)
	}
	topK, err := strconv.Atoi(env("RAG_TOP_K", "5"))
	if err != nil {
		return Config{}, fmt.Errorf("RAG_TOP_K: %w", err)
	}
	maxMB, err := strconv.ParseInt(env("MAX_UPLOAD_MB", "200"), 10, 64)
	if err != nil {
		return Config{}, fmt.Errorf("MAX_UPLOAD_MB: %w", err)
	}
	if maxMB < 1 || maxMB > 2048 {
		return Config{}, fmt.Errorf("MAX_UPLOAD_MB must be between 1 and 2048")
	}
	mode := env("EMBEDDING_MODE", "auto")
	if mode != "auto" && mode != "lexical" && mode != "remote" {
		return Config{}, fmt.Errorf("invalid EMBEDDING_MODE")
	}
	personal, err := strconv.ParseBool(env("PERSONAL_KB_ENABLED", "false"))
	if err != nil {
		return Config{}, err
	}
	jobTimeout, err := time.ParseDuration(env("DOCUMENT_JOB_TIMEOUT", "2h"))
	if err != nil || jobTimeout < time.Minute || jobTimeout > 4*time.Hour {
		return Config{}, fmt.Errorf("invalid DOCUMENT_JOB_TIMEOUT (1m to 4h)")
	}
	c := Config{DocumentJobTimeout: jobTimeout, EmbeddingMode: mode, PersonalKBEnabled: personal,
		DemoMode: demo, RAGMinScore: minScore,
		HTTPAddr: env("HTTP_ADDR", ":8080"), DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisAddr: env("REDIS_ADDR", "localhost:6379"), RedisPassword: os.Getenv("REDIS_PASSWORD"),
		JWTSecret: strings.TrimSpace(os.Getenv("JWT_SECRET")), JWTTTL: ttl,
		StorageRoot: env("STORAGE_ROOT", "./storage/uploads"), MaxUploadBytes: maxMB << 20,
		AdminBootstrapToken: strings.TrimSpace(os.Getenv("ADMIN_BOOTSTRAP_TOKEN")),
		LLMBaseURL:          env("LLM_BASE_URL", "https://api.openai.com/v1"), LLMAPIKey: os.Getenv("LLM_API_KEY"), LLMModel: env("LLM_MODEL", "gpt-4.1-mini"),
		EmbeddingBaseURL: env("EMBEDDING_BASE_URL", "https://api.openai.com/v1"), EmbeddingAPIKey: os.Getenv("EMBEDDING_API_KEY"), EmbeddingModel: env("EMBEDDING_MODEL", "text-embedding-3-small"), EmbeddingDimension: dim,
		RAGTopK: topK, CORSOrigins: strings.Split(env("CORS_ORIGINS", "http://localhost:5173"), ","),
	}
	if c.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if c.JWTSecret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET is required")
	}
	if len(c.JWTSecret) < 32 || strings.Contains(strings.ToLower(c.JWTSecret), "change-this") {
		return Config{}, fmt.Errorf("JWT_SECRET must contain at least 32 non-placeholder characters")
	}
	if c.EmbeddingDimension < 1 || c.EmbeddingDimension > 2000 {
		return Config{}, fmt.Errorf("EMBEDDING_DIMENSION must be between 1 and 2000")
	}
	return c, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
