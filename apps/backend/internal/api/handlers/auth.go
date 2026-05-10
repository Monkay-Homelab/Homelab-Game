package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/mail"
	"regexp"
	"strings"

	"github.com/homelab-game/backend/internal/auth"
	"github.com/homelab-game/backend/internal/database/queries"
)

var validDisplayName = regexp.MustCompile(`^[a-zA-Z0-9]+$`)

var blockedWords = map[string]bool{
	"fuck": true, "shit": true, "ass": true, "bitch": true, "dick": true,
	"cunt": true, "nigger": true, "nigga": true, "faggot": true, "fag": true,
	"retard": true, "whore": true, "slut": true, "porn": true, "xxx": true,
	"cock": true, "pussy": true, "tits": true, "nazi": true, "hitler": true,
}

var blockedPatterns = regexp.MustCompile(`(?i)(https?://|www\.|\.com|\.net|\.org|\.io)`)

type AuthHandler struct {
	users               *queries.UserQueries
	gameState           *queries.GameStateQueries
	jwtSecret           string
	registrationEnabled bool
}

func NewAuthHandler(users *queries.UserQueries, gameState *queries.GameStateQueries, jwtSecret string, registrationEnabled bool) *AuthHandler {
	return &AuthHandler{users: users, gameState: gameState, jwtSecret: jwtSecret, registrationEnabled: registrationEnabled}
}

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	Token string `json:"token"`
	User  any    `json:"user"`
}

// clientIP extracts the real client IP from the request. Prefers X-Real-IP
// (set by trusted reverse proxy) over X-Forwarded-For. For XFF, uses the
// rightmost IP which is the one appended by the trusted proxy.
func clientIP(r *http.Request) string {
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	return r.RemoteAddr
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if !h.registrationEnabled {
		slog.Warn("registration failed", "reason", "registration_disabled", "client_ip", clientIP(r))
		http.Error(w, `{"error":"registration is currently disabled"}`, http.StatusForbidden)
		return
	}

	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("registration failed", "reason", "invalid_request_body", "client_ip", clientIP(r))
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.Password == "" || req.DisplayName == "" {
		slog.Warn("registration failed", "email", req.Email, "reason", "missing_required_fields", "client_ip", clientIP(r))
		http.Error(w, `{"error":"email, password, and display_name are required"}`, http.StatusBadRequest)
		return
	}

	req.DisplayName = strings.TrimSpace(req.DisplayName)

	if len(req.DisplayName) < 2 || len(req.DisplayName) > 20 {
		slog.Warn("registration failed", "email", req.Email, "reason", "display_name_length", "client_ip", clientIP(r))
		http.Error(w, `{"error":"display name must be 2-20 characters"}`, http.StatusBadRequest)
		return
	}

	if !validDisplayName.MatchString(req.DisplayName) {
		slog.Warn("registration failed", "email", req.Email, "reason", "display_name_invalid_chars", "client_ip", clientIP(r))
		http.Error(w, `{"error":"display name must be alphanumeric only (a-z, 0-9)"}`, http.StatusBadRequest)
		return
	}

	lower := strings.ToLower(req.DisplayName)
	for word := range blockedWords {
		if strings.Contains(lower, word) {
			slog.Warn("registration failed", "email", req.Email, "reason", "display_name_blocked_content", "client_ip", clientIP(r))
			http.Error(w, `{"error":"display name contains inappropriate content"}`, http.StatusBadRequest)
			return
		}
	}

	if blockedPatterns.MatchString(req.DisplayName) {
		slog.Warn("registration failed", "email", req.Email, "reason", "display_name_contains_link", "client_ip", clientIP(r))
		http.Error(w, `{"error":"display name cannot contain links"}`, http.StatusBadRequest)
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	if len(req.Email) > 255 {
		slog.Warn("registration failed", "email", req.Email[:50], "reason", "email_too_long", "client_ip", clientIP(r))
		http.Error(w, `{"error":"email too long"}`, http.StatusBadRequest)
		return
	}

	if _, err := mail.ParseAddress(req.Email); err != nil || !strings.Contains(req.Email, ".") {
		slog.Warn("registration failed", "email", req.Email, "reason", "invalid_email", "client_ip", clientIP(r))
		http.Error(w, `{"error":"invalid email address"}`, http.StatusBadRequest)
		return
	}

	if len(req.Password) < 8 || len(req.Password) > 128 {
		slog.Warn("registration failed", "email", req.Email, "reason", "password_length", "client_ip", clientIP(r))
		http.Error(w, `{"error":"password must be 8-128 characters"}`, http.StatusBadRequest)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		slog.Warn("registration failed", "email", req.Email, "reason", "password_hash_error", "client_ip", clientIP(r))
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	user, err := h.users.Create(r.Context(), req.Email, hash, req.DisplayName)
	if err != nil {
		slog.Warn("registration failed", "email", req.Email, "reason", "duplicate_email", "client_ip", clientIP(r))
		http.Error(w, `{"error":"email already exists"}`, http.StatusConflict)
		return
	}

	// Create initial game state for new user
	_, err = h.gameState.Create(r.Context(), user.ID)
	if err != nil {
		slog.Warn("registration failed", "email", req.Email, "reason", "game_state_creation_error", "user_id", user.ID, "client_ip", clientIP(r))
		http.Error(w, `{"error":"failed to create game state"}`, http.StatusInternalServerError)
		return
	}

	token, err := auth.GenerateToken(user.ID, h.jwtSecret)
	if err != nil {
		slog.Warn("registration failed", "email", req.Email, "reason", "token_generation_error", "user_id", user.ID, "client_ip", clientIP(r))
		http.Error(w, `{"error":"failed to generate token"}`, http.StatusInternalServerError)
		return
	}

	slog.Info("user registered", "email", req.Email, "user_id", user.ID, "client_ip", clientIP(r))

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(authResponse{Token: token, User: user})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("login failed", "reason", "invalid_request_body", "client_ip", clientIP(r))
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.Password == "" {
		slog.Warn("login failed", "reason", "missing_required_fields", "client_ip", clientIP(r))
		http.Error(w, `{"error":"email and password are required"}`, http.StatusBadRequest)
		return
	}

	user, err := h.users.GetByEmail(r.Context(), req.Email)
	if err != nil {
		slog.Warn("login failed", "email", req.Email, "reason", "invalid_credentials", "client_ip", clientIP(r))
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	if user.PasswordHash == nil || !auth.CheckPassword(req.Password, *user.PasswordHash) {
		slog.Warn("login failed", "email", req.Email, "reason", "invalid_credentials", "client_ip", clientIP(r))
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	token, err := auth.GenerateToken(user.ID, h.jwtSecret)
	if err != nil {
		slog.Warn("login failed", "email", req.Email, "reason", "token_generation_error", "client_ip", clientIP(r))
		http.Error(w, `{"error":"failed to generate token"}`, http.StatusInternalServerError)
		return
	}

	slog.Info("user logged in", "email", req.Email, "user_id", user.ID, "client_ip", clientIP(r))

	_ = json.NewEncoder(w).Encode(authResponse{Token: token, User: user})
}
