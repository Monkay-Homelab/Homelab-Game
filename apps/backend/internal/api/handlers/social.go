package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/homelab-game/backend/internal/api/middleware"
	"github.com/homelab-game/backend/internal/database/queries"
	"github.com/homelab-game/backend/internal/models"
)

type SocialHandler struct {
	groups       *queries.GroupQueries
	leaderboard  *queries.LeaderboardQueries
	gameState    *queries.GameStateQueries
}

func NewSocialHandler(groups *queries.GroupQueries, leaderboard *queries.LeaderboardQueries, gameState *queries.GameStateQueries) *SocialHandler {
	return &SocialHandler{groups: groups, leaderboard: leaderboard, gameState: gameState}
}

// POST /api/social/group/create
type createGroupRequest struct {
	Name string `json:"name"`
}

func (h *SocialHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req createGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	if len(req.Name) < 3 || len(req.Name) > 50 {
		http.Error(w, `{"error":"group name must be 3-50 characters"}`, http.StatusBadRequest)
		return
	}

	// Check user isn't already in a group
	existing, _, _ := h.groups.GetUserGroup(r.Context(), userID)
	if existing != nil {
		http.Error(w, `{"error":"you're already in a group"}`, http.StatusConflict)
		return
	}

	group := &models.Group{
		Name:      req.Name,
		FounderID: userID,
	}
	if err := h.groups.Create(r.Context(), group); err != nil {
		http.Error(w, `{"error":"group name already taken"}`, http.StatusConflict)
		return
	}

	// Add founder as member
	h.groups.AddMember(r.Context(), group.ID, userID, "founder")

	members, _ := h.groups.GetMembers(r.Context(), group.ID)
	pool, _ := h.groups.GetGroupComputePool(r.Context(), group.ID)

	json.NewEncoder(w).Encode(map[string]any{
		"group": group, "members": members, "compute_pool": pool,
	})
}

// POST /api/social/group/join
type joinGroupRequest struct {
	Name string `json:"name"`
}

func (h *SocialHandler) JoinGroup(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req joinGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	existing, _, _ := h.groups.GetUserGroup(r.Context(), userID)
	if existing != nil {
		http.Error(w, `{"error":"you're already in a group"}`, http.StatusConflict)
		return
	}

	group, err := h.groups.GetByName(r.Context(), req.Name)
	if err != nil {
		http.Error(w, `{"error":"group not found"}`, http.StatusNotFound)
		return
	}

	h.groups.AddMember(r.Context(), group.ID, userID, "member")

	members, _ := h.groups.GetMembers(r.Context(), group.ID)
	pool, _ := h.groups.GetGroupComputePool(r.Context(), group.ID)

	json.NewEncoder(w).Encode(map[string]any{
		"group": group, "members": members, "compute_pool": pool,
	})
}

// POST /api/social/group/leave
func (h *SocialHandler) LeaveGroup(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	group, member, err := h.groups.GetUserGroup(r.Context(), userID)
	if err != nil {
		http.Error(w, `{"error":"not in a group"}`, http.StatusNotFound)
		return
	}

	if member.Role == "founder" {
		// Founder leaving deletes the group
		h.groups.Delete(r.Context(), group.ID)
	} else {
		h.groups.RemoveMember(r.Context(), group.ID, userID)
	}

	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// GET /api/social/group
func (h *SocialHandler) GetMyGroup(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	group, member, err := h.groups.GetUserGroup(r.Context(), userID)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"group": nil})
		return
	}

	members, _ := h.groups.GetMembers(r.Context(), group.ID)
	pool, _ := h.groups.GetGroupComputePool(r.Context(), group.ID)

	json.NewEncoder(w).Encode(map[string]any{
		"group": group, "members": members, "my_role": member.Role, "compute_pool": pool,
	})
}

// GET /api/social/groups
func (h *SocialHandler) ListGroups(w http.ResponseWriter, r *http.Request) {
	groups, _ := h.groups.List(r.Context(), 50)
	json.NewEncoder(w).Encode(map[string]any{"groups": groups})
}

// POST /api/social/group/promote
type promoteRequest struct {
	UserID string `json:"user_id"`
}

func (h *SocialHandler) PromoteMember(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req promoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	group, member, err := h.groups.GetUserGroup(r.Context(), userID)
	if err != nil {
		http.Error(w, `{"error":"not in a group"}`, http.StatusNotFound)
		return
	}

	if member.Role != "founder" && member.Role != "admin" {
		http.Error(w, `{"error":"only founders and admins can promote"}`, http.StatusForbidden)
		return
	}

	// Verify target is actually in this group
	isMember, _ := h.groups.IsMember(r.Context(), group.ID, req.UserID)
	if !isMember {
		http.Error(w, `{"error":"user is not in this group"}`, http.StatusBadRequest)
		return
	}

	h.groups.SetRole(r.Context(), group.ID, req.UserID, "admin")
	members, _ := h.groups.GetMembers(r.Context(), group.ID)

	json.NewEncoder(w).Encode(map[string]any{"members": members})
}

// POST /api/social/group/kick
type kickRequest struct {
	UserID string `json:"user_id"`
}

func (h *SocialHandler) KickMember(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req kickRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	group, member, err := h.groups.GetUserGroup(r.Context(), userID)
	if err != nil {
		http.Error(w, `{"error":"not in a group"}`, http.StatusNotFound)
		return
	}

	if member.Role != "founder" && member.Role != "admin" {
		http.Error(w, `{"error":"only founders and admins can kick"}`, http.StatusForbidden)
		return
	}

	if req.UserID == group.FounderID {
		http.Error(w, `{"error":"cannot kick the founder"}`, http.StatusForbidden)
		return
	}

	// Verify target is actually in this group
	isMember, _ := h.groups.IsMember(r.Context(), group.ID, req.UserID)
	if !isMember {
		http.Error(w, `{"error":"user is not in this group"}`, http.StatusBadRequest)
		return
	}

	h.groups.RemoveMember(r.Context(), group.ID, req.UserID)
	members, _ := h.groups.GetMembers(r.Context(), group.ID)

	json.NewEncoder(w).Encode(map[string]any{"members": members})
}

// GET /api/social/leaderboard?category=compute
func (h *SocialHandler) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	if category == "" {
		category = "compute"
	}

	if category == "group" {
		entries, err := h.leaderboard.GetTopGroups(r.Context(), 50)
		if err != nil {
			log.Printf("leaderboard query error (group): %v", err)
		}
		if entries == nil {
			entries = []models.LeaderboardEntry{}
		}
		json.NewEncoder(w).Encode(map[string]any{"category": category, "entries": entries})
		return
	}

	entries, err := h.leaderboard.GetTopByCategory(r.Context(), category, 50)
	if err != nil {
		log.Printf("leaderboard query error (%s): %v", category, err)
	}
	if entries == nil {
		entries = []models.LeaderboardEntry{}
	}
	json.NewEncoder(w).Encode(map[string]any{"category": category, "entries": entries})
}

// POST /api/social/leaderboard/update — called internally to refresh scores
func (h *SocialHandler) UpdateLeaderboards(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	gs, err := h.gameState.GetByUserID(r.Context(), userID)
	if err != nil {
		http.Error(w, `{"error":"game state not found"}`, http.StatusNotFound)
		return
	}

	// Update all leaderboard categories for this user
	h.leaderboard.UpdateScore(r.Context(), userID, "compute", gs.ComputeUnits)
	h.leaderboard.UpdateScore(r.Context(), userID, "reputation", gs.Reputation)
	h.leaderboard.UpdateScore(r.Context(), userID, "colo_count", int64(gs.ColoCount))
	h.leaderboard.UpdateScore(r.Context(), userID, "money", gs.Money)
	h.leaderboard.UpdateScore(r.Context(), userID, "bitcoin_balance", gs.BitcoinBalance)

	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}
