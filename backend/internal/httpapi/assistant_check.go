package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/local/exam408-assistant/backend/internal/service"
)

func (s *Server) checkAssistant(w http.ResponseWriter, r *http.Request) {
	status := s.Config.AssistantStatus()
	if !status.Enabled || status.Mode == "demo" {
		write(w, 200, service.AssistantCheckReport{Checks: []service.AssistantCheck{{Name: "模型配置", Passed: false, Message: status.Message}}})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	write(w, 200, s.RAG.CheckProviders(ctx))
}
