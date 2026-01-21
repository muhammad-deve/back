package handler

import (
	"encoding/json"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
	"gitlab.yurtal.tech/company/pocketbase-app-template/internal/service"
)

// ChatHandler handles the /api/chat endpoint
func (h *Handler) ChatHandler(e *core.RequestEvent) error {
	// Parse request body
	var req service.ChatRequest
	if err := json.NewDecoder(e.Request.Body).Decode(&req); err != nil {
		return e.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
	}

	// Validate message
	if req.Message == "" {
		return e.JSON(http.StatusBadRequest, map[string]string{
			"error": "Message is required",
		})
	}

	// Create Gemini service and get response
	geminiService := service.NewGeminiService(h.cfg.GeminiAPIKey)
	resp, err := geminiService.Chat(req)
	if err != nil {
		h.logger.Error("Chat error", "error", err.Error())
		return e.JSON(http.StatusInternalServerError, map[string]string{
			"error": "I'm having trouble connecting right now. Please try again in a moment!",
		})
	}

	return e.JSON(http.StatusOK, resp)
}
