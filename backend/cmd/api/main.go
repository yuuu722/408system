package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/local/exam408-assistant/backend/internal/config"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"github.com/local/exam408-assistant/backend/internal/httpapi"
	"github.com/local/exam408-assistant/backend/internal/repository"
	"github.com/local/exam408-assistant/backend/internal/service"
	"github.com/local/exam408-assistant/backend/migrations"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		if err := checkHealth("http://127.0.0.1:8080/healthz"); err != nil {
			os.Exit(1)
		}
		return
	}
	ctx := context.Background()
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		log.Error("configuration failed", "error", err)
		os.Exit(1)
	}
	store, err := repository.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer store.DB.Close()
	store.EmbeddingSpace = cfg.EmbeddingSpace()
	store.DocumentJobTimeout = cfg.DocumentJobTimeout

	if err = migrations.Up(ctx, store.DB); err != nil {
		log.Error("migration failed", "error", err)
		os.Exit(1)
	}
	if err = store.EnsurePublicDocuments(ctx, cfg.StorageRoot); err != nil {
		log.Error("public curriculum", "error", err)
		os.Exit(1)
	}
	if err = store.RefreshAssetAvailability(ctx); err != nil {
		log.Error("resource availability", "error", err)
		os.Exit(1)
	}
	embed := &service.OpenAIEmbedding{BaseURL: cfg.EmbeddingBaseURL, APIKey: cfg.EmbeddingAPIKey, Model: cfg.EmbeddingModel, Dimension: cfg.EmbeddingDimension, DemoMode: cfg.DemoMode, LocalLexical: cfg.UsesLocalIndex()}
	llm := &service.OpenAILLM{BaseURL: cfg.LLMBaseURL, APIKey: cfg.LLMAPIKey, Model: cfg.LLMModel, DemoMode: cfg.DemoMode}
	var graphLLM domain.LLMProvider
	if cfg.LLMAPIKey != "" && !cfg.DemoMode && os.Getenv("GRAPH_EXTRACTION_ENABLED") == "true" {
		graphLLM = llm
	}
	learning := service.LearningService{Store: store}
	knowledge := service.KnowledgeService{Store: store}
	srv := &httpapi.Server{QuestionAI: service.QuestionAI{Text: *llm, Vision: service.OpenAILLM{BaseURL: os.Getenv("VISION_BASE_URL"), APIKey: os.Getenv("VISION_API_KEY"), Model: os.Getenv("VISION_MODEL")}}, Platform: store, Store: store, Auth: service.AuthService{Store: store, Secret: []byte(cfg.JWTSecret), TTL: cfg.JWTTTL}, Documents: &service.DocumentService{EmbeddingSpace: cfg.EmbeddingSpace(), Store: store, Parser: service.Parser{Endpoint: os.Getenv("DOCUMENT_PARSER_URL"), Token: os.Getenv("DOCUMENT_PARSER_TOKEN"), Mode: os.Getenv("DOCUMENT_PARSER_MODE")}, Embedder: embed, GraphLLM: graphLLM, StorageRoot: cfg.StorageRoot, MaxBytes: cfg.MaxUploadBytes, Log: log}, Knowledge: knowledge, Learning: learning, Recommendations: service.RecommendationService{Store: store}, RAG: service.RAGService{Vision: &service.OpenAILLM{BaseURL: os.Getenv("VISION_BASE_URL"), APIKey: os.Getenv("VISION_API_KEY"), Model: os.Getenv("VISION_MODEL")}, Store: store, Embedder: embed, LLM: llm, Knowledge: knowledge, Learning: learning, TopK: cfg.RAGTopK, MinScore: cfg.RAGMinScore, Log: log}, Config: cfg, Log: log}
	srv.Generator = service.QuestionGenerator{Store: store, Embedder: embed, Model: *llm, MinScore: cfg.RAGMinScore}
	if os.Getenv("APP_ROLE") == "worker" {
		workerCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		log.Info("document worker started")
		for workerCtx.Err() == nil {
			found, e := store.RunDocumentJob(workerCtx, srv.Documents.Process)
			if e != nil {
				log.Error("document job", "error", e)
			}
			if !found || e != nil {
				select {
				case <-workerCtx.Done():
				case <-time.After(2 * time.Second):
				}
			}
		}
		return
	}
	httpServer := &http.Server{Addr: cfg.HTTPAddr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Minute, WriteTimeout: 30 * time.Minute, IdleTimeout: 60 * time.Second}
	go func() {
		log.Info("server listening", "address", cfg.HTTPAddr)
		if e := httpServer.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			log.Error("server stopped", "error", e)
			os.Exit(1)
		}
	}()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdown)
}
