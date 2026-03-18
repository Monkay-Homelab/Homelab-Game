package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/homelab-game/backend/internal/api/middleware"
	"github.com/homelab-game/backend/internal/database/queries"
	"github.com/homelab-game/backend/internal/game/engine"
)

type GameHandler struct {
	gameState *queries.GameStateQueries
	engine    *engine.Engine
}

func NewGameHandler(gameState *queries.GameStateQueries, eng *engine.Engine) *GameHandler {
	return &GameHandler{gameState: gameState, engine: eng}
}

func (h *GameHandler) GetState(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	gs, err := h.gameState.GetByUserID(r.Context(), userID)
	if err != nil {
		http.Error(w, `{"error":"game state not found"}`, http.StatusNotFound)
		return
	}

	// Calculate idle progress since last tick
	now := time.Now()
	h.engine.ProcessIdleProgress(gs, now)

	// Save updated state
	if err := h.gameState.Update(r.Context(), gs); err != nil {
		http.Error(w, `{"error":"failed to update game state"}`, http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(gs)
}

type actionRequest struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func (h *GameHandler) PerformAction(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req actionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	gs, err := h.gameState.GetByUserID(r.Context(), userID)
	if err != nil {
		http.Error(w, `{"error":"game state not found"}`, http.StatusNotFound)
		return
	}

	// Process idle progress first
	now := time.Now()
	h.engine.ProcessIdleProgress(gs, now)

	// Validate and apply action
	if err := h.engine.ProcessAction(gs, req.Type, req.Payload); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	// Save updated state
	if err := h.gameState.Update(r.Context(), gs); err != nil {
		http.Error(w, `{"error":"failed to update game state"}`, http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(gs)
}
