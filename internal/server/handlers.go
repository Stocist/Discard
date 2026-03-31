package server

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/Stocist/discard/internal/auth"
	"github.com/Stocist/discard/internal/database"
	"github.com/Stocist/discard/internal/models"
	"github.com/Stocist/discard/internal/upload"
	"github.com/google/uuid"
)

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		jsonError(w, "invalid multipart form", http.StatusBadRequest)
		return
	}

	var displayName *string
	if dn := strings.TrimSpace(r.FormValue("display_name")); dn != "" {
		if len(dn) > 64 {
			jsonError(w, "display name must be 64 characters or less", http.StatusBadRequest)
			return
		}
		displayName = &dn
	}

	var avatarPath *string
	if fh, _, err := r.FormFile("avatar"); err == nil {
		fh.Close()
		fileHeader := r.MultipartForm.File["avatar"][0]
		result, err := upload.ProcessFile(s.uploadDir, fileHeader)
		if err != nil {
			log.Printf("avatar upload error: %v", err)
			jsonError(w, "failed to process avatar", http.StatusBadRequest)
			return
		}
		avatarPath = &result.FilePath
	}

	userRepo := &database.UserRepo{DB: s.db}
	updated, err := userRepo.UpdateProfile(r.Context(), user.ID, displayName, avatarPath)
	if err != nil {
		jsonError(w, "failed to update profile", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// --- Servers ---

func (s *Server) handleCreateServer(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var input struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if input.Name == "" {
		jsonError(w, "name is required", http.StatusBadRequest)
		return
	}
	if len(input.Name) > 100 {
		jsonError(w, "server name must be 100 characters or less", http.StatusBadRequest)
		return
	}

	// Generate invite code.
	codeBytes := make([]byte, 8)
	rand.Read(codeBytes)
	inviteCode := hex.EncodeToString(codeBytes)

	srv := &models.Server{
		Name:       input.Name,
		OwnerID:    user.ID,
		InviteCode: &inviteCode,
	}

	serverRepo := &database.ServerRepo{DB: s.db}
	if err := serverRepo.CreateServer(r.Context(), srv); err != nil {
		jsonError(w, "failed to create server", http.StatusInternalServerError)
		return
	}

	// Add creator as member.
	memberRepo := &database.ServerMemberRepo{DB: s.db}
	if err := memberRepo.AddMember(r.Context(), &models.ServerMember{
		UserID:   user.ID,
		ServerID: srv.ID,
	}); err != nil {
		jsonError(w, "failed to add owner as member", http.StatusInternalServerError)
		return
	}

	// Create default "general" text channel.
	channelName := "general"
	ch := &models.Channel{
		ServerID: &srv.ID,
		Name:     &channelName,
		Type:     "text",
		Position: 0,
	}
	channelRepo := &database.ChannelRepo{DB: s.db}
	if err := channelRepo.CreateChannel(r.Context(), ch); err != nil {
		jsonError(w, "failed to create default channel", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(srv)
}

func (s *Server) handleListServers(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	serverRepo := &database.ServerRepo{DB: s.db}
	servers, err := serverRepo.ListUserServers(r.Context(), user.ID)
	if err != nil {
		jsonError(w, "failed to list servers", http.StatusInternalServerError)
		return
	}
	if servers == nil {
		servers = []models.Server{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(servers)
}

func (s *Server) handleGetServer(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	serverID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid server id", http.StatusBadRequest)
		return
	}

	// Check membership.
	memberRepo := &database.ServerMemberRepo{DB: s.db}
	isMember, err := memberRepo.IsMember(r.Context(), user.ID, serverID)
	if err != nil {
		jsonError(w, "failed to check membership", http.StatusInternalServerError)
		return
	}
	if !isMember {
		jsonError(w, "forbidden", http.StatusForbidden)
		return
	}

	serverRepo := &database.ServerRepo{DB: s.db}
	srv, err := serverRepo.GetServerByID(r.Context(), serverID)
	if err == sql.ErrNoRows {
		jsonError(w, "server not found", http.StatusNotFound)
		return
	}
	if err != nil {
		jsonError(w, "failed to get server", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(srv)
}

func (s *Server) handleUpdateServer(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	serverID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid server id", http.StatusBadRequest)
		return
	}

	// Only the owner can update the server.
	serverRepo := &database.ServerRepo{DB: s.db}
	srv, err := serverRepo.GetServerByID(r.Context(), serverID)
	if err == sql.ErrNoRows {
		jsonError(w, "server not found", http.StatusNotFound)
		return
	}
	if err != nil {
		jsonError(w, "failed to get server", http.StatusInternalServerError)
		return
	}
	if srv.OwnerID != user.ID {
		jsonError(w, "only the server owner can update settings", http.StatusForbidden)
		return
	}

	var name string
	var iconPath *string

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			jsonError(w, "invalid multipart form", http.StatusBadRequest)
			return
		}
		name = strings.TrimSpace(r.FormValue("name"))

		// Process optional icon file.
		if fh, _, err := r.FormFile("icon"); err == nil {
			fh.Close()
			fileHeader := r.MultipartForm.File["icon"][0]
			result, err := upload.ProcessFile(s.uploadDir, fileHeader)
			if err != nil {
				log.Printf("icon upload error: %v", err)
				jsonError(w, "failed to process icon", http.StatusBadRequest)
				return
			}
			iconPath = &result.FilePath
		}
	} else {
		var input struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		name = strings.TrimSpace(input.Name)
	}

	if name == "" {
		jsonError(w, "name is required", http.StatusBadRequest)
		return
	}
	if len(name) > 100 {
		jsonError(w, "server name must be 100 characters or less", http.StatusBadRequest)
		return
	}

	updated, err := serverRepo.UpdateServer(r.Context(), serverID, name, iconPath)
	if err != nil {
		jsonError(w, "failed to update server", http.StatusInternalServerError)
		return
	}

	// Broadcast server update to all clients so sidebars refresh.
	out, err := json.Marshal(map[string]any{
		"type":   "server_update",
		"server": updated,
	})
	if err == nil {
		s.hub.BroadcastAll(out)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

func (s *Server) handleDeleteServer(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	serverID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid server id", http.StatusBadRequest)
		return
	}

	// Only the owner can delete the server.
	serverRepo := &database.ServerRepo{DB: s.db}
	srv, err := serverRepo.GetServerByID(r.Context(), serverID)
	if err == sql.ErrNoRows {
		jsonError(w, "server not found", http.StatusNotFound)
		return
	}
	if err != nil {
		jsonError(w, "failed to get server", http.StatusInternalServerError)
		return
	}
	if srv.OwnerID != user.ID {
		jsonError(w, "only the server owner can delete this server", http.StatusForbidden)
		return
	}

	if err := serverRepo.DeleteServer(r.Context(), serverID); err != nil {
		jsonError(w, "failed to delete server", http.StatusInternalServerError)
		return
	}

	// Broadcast server deletion to all clients.
	out, err := json.Marshal(map[string]any{
		"type":      "server_delete",
		"server_id": serverID.String(),
	})
	if err == nil {
		s.hub.BroadcastAll(out)
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Channels ---

func (s *Server) handleCreateChannel(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	serverID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid server id", http.StatusBadRequest)
		return
	}

	// Only server owner can create channels.
	serverRepo := &database.ServerRepo{DB: s.db}
	srv, err := serverRepo.GetServerByID(r.Context(), serverID)
	if err == sql.ErrNoRows {
		jsonError(w, "server not found", http.StatusNotFound)
		return
	}
	if err != nil {
		jsonError(w, "failed to get server", http.StatusInternalServerError)
		return
	}
	if srv.OwnerID != user.ID {
		jsonError(w, "only the server owner can create channels", http.StatusForbidden)
		return
	}

	var input struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if input.Name == "" {
		jsonError(w, "name is required", http.StatusBadRequest)
		return
	}
	if len(input.Name) > 100 {
		jsonError(w, "channel name must be 100 characters or less", http.StatusBadRequest)
		return
	}
	if input.Type == "" {
		input.Type = "text"
	}

	ch := &models.Channel{
		ServerID: &serverID,
		Name:     &input.Name,
		Type:     input.Type,
	}
	channelRepo := &database.ChannelRepo{DB: s.db}
	if err := channelRepo.CreateChannel(r.Context(), ch); err != nil {
		jsonError(w, "failed to create channel", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ch)
}

func (s *Server) handleListChannels(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	serverID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid server id", http.StatusBadRequest)
		return
	}

	memberRepo := &database.ServerMemberRepo{DB: s.db}
	isMember, err := memberRepo.IsMember(r.Context(), user.ID, serverID)
	if err != nil {
		jsonError(w, "failed to check membership", http.StatusInternalServerError)
		return
	}
	if !isMember {
		jsonError(w, "forbidden", http.StatusForbidden)
		return
	}

	channelRepo := &database.ChannelRepo{DB: s.db}
	channels, err := channelRepo.ListServerChannels(r.Context(), serverID)
	if err != nil {
		jsonError(w, "failed to list channels", http.StatusInternalServerError)
		return
	}
	if channels == nil {
		channels = []models.Channel{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(channels)
}

func (s *Server) handleUpdateChannel(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	serverID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid server id", http.StatusBadRequest)
		return
	}

	channelID, err := uuid.Parse(r.PathValue("channelId"))
	if err != nil {
		jsonError(w, "invalid channel id", http.StatusBadRequest)
		return
	}

	// Only server owner can rename channels.
	serverRepo := &database.ServerRepo{DB: s.db}
	srv, err := serverRepo.GetServerByID(r.Context(), serverID)
	if err == sql.ErrNoRows {
		jsonError(w, "server not found", http.StatusNotFound)
		return
	}
	if err != nil {
		jsonError(w, "failed to get server", http.StatusInternalServerError)
		return
	}
	if srv.OwnerID != user.ID {
		jsonError(w, "only the server owner can rename channels", http.StatusForbidden)
		return
	}

	var input struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		jsonError(w, "name is required", http.StatusBadRequest)
		return
	}
	if len(input.Name) > 100 {
		jsonError(w, "channel name must be 100 characters or less", http.StatusBadRequest)
		return
	}

	// Verify channel belongs to this server.
	channelRepo := &database.ChannelRepo{DB: s.db}
	ch, err := channelRepo.GetChannelByID(r.Context(), channelID)
	if err == sql.ErrNoRows {
		jsonError(w, "channel not found", http.StatusNotFound)
		return
	}
	if err != nil {
		jsonError(w, "failed to get channel", http.StatusInternalServerError)
		return
	}
	if ch.ServerID == nil || *ch.ServerID != serverID {
		jsonError(w, "channel does not belong to this server", http.StatusBadRequest)
		return
	}

	updated, err := channelRepo.UpdateChannel(r.Context(), channelID, input.Name)
	if err != nil {
		jsonError(w, "failed to update channel", http.StatusInternalServerError)
		return
	}

	// Broadcast channel update to all connected clients.
	out, err := json.Marshal(map[string]any{
		"type":    "channel_update",
		"channel": updated,
	})
	if err == nil {
		s.hub.BroadcastAll(out)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

func (s *Server) handleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	serverID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid server id", http.StatusBadRequest)
		return
	}

	channelID, err := uuid.Parse(r.PathValue("channelId"))
	if err != nil {
		jsonError(w, "invalid channel id", http.StatusBadRequest)
		return
	}

	// Only server owner can delete channels.
	serverRepo := &database.ServerRepo{DB: s.db}
	srv, err := serverRepo.GetServerByID(r.Context(), serverID)
	if err == sql.ErrNoRows {
		jsonError(w, "server not found", http.StatusNotFound)
		return
	}
	if err != nil {
		jsonError(w, "failed to get server", http.StatusInternalServerError)
		return
	}
	if srv.OwnerID != user.ID {
		jsonError(w, "only the server owner can delete channels", http.StatusForbidden)
		return
	}

	// Verify channel belongs to this server.
	channelRepo := &database.ChannelRepo{DB: s.db}
	ch, err := channelRepo.GetChannelByID(r.Context(), channelID)
	if err == sql.ErrNoRows {
		jsonError(w, "channel not found", http.StatusNotFound)
		return
	}
	if err != nil {
		jsonError(w, "failed to get channel", http.StatusInternalServerError)
		return
	}
	if ch.ServerID == nil || *ch.ServerID != serverID {
		jsonError(w, "channel does not belong to this server", http.StatusBadRequest)
		return
	}

	if err := channelRepo.DeleteChannel(r.Context(), channelID); err != nil {
		jsonError(w, "failed to delete channel", http.StatusInternalServerError)
		return
	}

	// Broadcast channel deletion to all connected clients.
	out, err := json.Marshal(map[string]string{
		"type":       "channel_delete",
		"channel_id": channelID.String(),
		"server_id":  serverID.String(),
	})
	if err == nil {
		s.hub.BroadcastAll(out)
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Members ---

func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	serverID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid server id", http.StatusBadRequest)
		return
	}

	memberRepo := &database.ServerMemberRepo{DB: s.db}
	isMember, err := memberRepo.IsMember(r.Context(), user.ID, serverID)
	if err != nil {
		jsonError(w, "failed to check membership", http.StatusInternalServerError)
		return
	}
	if !isMember {
		jsonError(w, "forbidden", http.StatusForbidden)
		return
	}

	members, err := memberRepo.ListMembers(r.Context(), serverID)
	if err != nil {
		jsonError(w, "failed to list members", http.StatusInternalServerError)
		return
	}
	if members == nil {
		members = []models.ServerMember{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(members)
}

func (s *Server) handleJoinServer(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var input struct {
		InviteCode string `json:"invite_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if input.InviteCode == "" {
		jsonError(w, "invite_code is required", http.StatusBadRequest)
		return
	}

	serverRepo := &database.ServerRepo{DB: s.db}
	srv, err := serverRepo.GetServerByInviteCode(r.Context(), input.InviteCode)
	if err == sql.ErrNoRows {
		jsonError(w, "invalid invite code", http.StatusNotFound)
		return
	}
	if err != nil {
		jsonError(w, "failed to look up invite code", http.StatusInternalServerError)
		return
	}

	// Check if already a member.
	memberRepo := &database.ServerMemberRepo{DB: s.db}
	isMember, err := memberRepo.IsMember(r.Context(), user.ID, srv.ID)
	if err != nil {
		jsonError(w, "failed to check membership", http.StatusInternalServerError)
		return
	}
	if isMember {
		jsonError(w, "already a member", http.StatusConflict)
		return
	}

	if err := memberRepo.AddMember(r.Context(), &models.ServerMember{
		UserID:   user.ID,
		ServerID: srv.ID,
	}); err != nil {
		jsonError(w, "failed to join server", http.StatusInternalServerError)
		return
	}

	// Broadcast member_joined to all clients so member sidebars refresh.
	out, err := json.Marshal(map[string]any{
		"type":      "member_joined",
		"server_id": srv.ID.String(),
		"member": map[string]any{
			"user_id":      user.ID.String(),
			"username":     user.Username,
			"display_name": user.DisplayName,
			"avatar_url":   user.AvatarPath,
			"role":         "member",
		},
	})
	if err == nil {
		s.hub.BroadcastAll(out)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(srv)
}

func (s *Server) handleRegenerateInviteCode(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	serverID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid server id", http.StatusBadRequest)
		return
	}

	// Only the owner can regenerate the invite code.
	serverRepo := &database.ServerRepo{DB: s.db}
	srv, err := serverRepo.GetServerByID(r.Context(), serverID)
	if err == sql.ErrNoRows {
		jsonError(w, "server not found", http.StatusNotFound)
		return
	}
	if err != nil {
		jsonError(w, "failed to get server", http.StatusInternalServerError)
		return
	}
	if srv.OwnerID != user.ID {
		jsonError(w, "only the server owner can regenerate the invite code", http.StatusForbidden)
		return
	}

	codeBytes := make([]byte, 8)
	rand.Read(codeBytes)
	newCode := hex.EncodeToString(codeBytes)

	updated, err := serverRepo.UpdateInviteCode(r.Context(), serverID, newCode)
	if err != nil {
		jsonError(w, "failed to regenerate invite code", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

func (s *Server) handleLeaveServer(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	serverID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid server id", http.StatusBadRequest)
		return
	}

	// Owner cannot leave.
	serverRepo := &database.ServerRepo{DB: s.db}
	srv, err := serverRepo.GetServerByID(r.Context(), serverID)
	if err == sql.ErrNoRows {
		jsonError(w, "server not found", http.StatusNotFound)
		return
	}
	if err != nil {
		jsonError(w, "failed to get server", http.StatusInternalServerError)
		return
	}
	if srv.OwnerID == user.ID {
		jsonError(w, "owner cannot leave the server", http.StatusForbidden)
		return
	}

	memberRepo := &database.ServerMemberRepo{DB: s.db}
	if err := memberRepo.RemoveMember(r.Context(), user.ID, serverID); err != nil {
		jsonError(w, "failed to leave server", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- DMs ---

func (s *Server) handleOpenDM(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var input struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	targetID, err := uuid.Parse(input.UserID)
	if err != nil {
		jsonError(w, "invalid user_id", http.StatusBadRequest)
		return
	}

	if targetID == user.ID {
		jsonError(w, "cannot open DM with yourself", http.StatusBadRequest)
		return
	}

	userRepo := &database.UserRepo{DB: s.db}
	if _, err := userRepo.GetByID(r.Context(), targetID); err == sql.ErrNoRows {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	} else if err != nil {
		jsonError(w, "failed to look up user", http.StatusInternalServerError)
		return
	}

	blockRepo := &database.BlockRepo{DB: s.db}
	blocked, err := blockRepo.IsBlocked(r.Context(), user.ID, targetID)
	if err != nil {
		jsonError(w, "failed to check blocks", http.StatusInternalServerError)
		return
	}
	if blocked {
		jsonError(w, "cannot open DM with this user", http.StatusForbidden)
		return
	}

	memberRepo := &database.ServerMemberRepo{DB: s.db}
	shared, err := memberRepo.ShareServer(r.Context(), user.ID, targetID)
	if err != nil {
		jsonError(w, "failed to check shared servers", http.StatusInternalServerError)
		return
	}
	if !shared {
		jsonError(w, "you must share a server to DM this user", http.StatusForbidden)
		return
	}

	dmRepo := &database.DMMemberRepo{DB: s.db}
	existing, err := dmRepo.FindDMChannel(r.Context(), user.ID, targetID)
	if err == nil {
		// Channel exists — reopen if closed for this user.
		if reopenErr := dmRepo.ReopenDM(r.Context(), existing.ID, user.ID); reopenErr != nil {
			jsonError(w, "failed to reopen DM", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(existing)

		// Notify the other user.
		out, _ := json.Marshal(map[string]any{
			"type":       "dm_opened",
			"channel_id": existing.ID.String(),
		})
		s.hub.SendToUser(targetID, out)
		return
	}
	if err != sql.ErrNoRows {
		jsonError(w, "failed to find DM channel", http.StatusInternalServerError)
		return
	}

	// Create new DM channel.
	ch := &models.Channel{Type: "dm"}
	channelRepo := &database.ChannelRepo{DB: s.db}
	if err := channelRepo.CreateChannel(r.Context(), ch); err != nil {
		jsonError(w, "failed to create DM channel", http.StatusInternalServerError)
		return
	}

	if err := dmRepo.AddMember(r.Context(), ch.ID, user.ID); err != nil {
		jsonError(w, "failed to add DM member", http.StatusInternalServerError)
		return
	}
	if err := dmRepo.AddMember(r.Context(), ch.ID, targetID); err != nil {
		jsonError(w, "failed to add DM member", http.StatusInternalServerError)
		return
	}

	// Notify the other user.
	out, _ := json.Marshal(map[string]any{
		"type":       "dm_opened",
		"channel_id": ch.ID.String(),
	})
	s.hub.SendToUser(targetID, out)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ch)
}

func (s *Server) handleListDMs(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	dmRepo := &database.DMMemberRepo{DB: s.db}
	dms, err := dmRepo.ListUserDMs(r.Context(), user.ID)
	if err != nil {
		jsonError(w, "failed to list DMs", http.StatusInternalServerError)
		return
	}
	if dms == nil {
		dms = []models.DMChannelView{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dms)
}

func (s *Server) handleCloseDM(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	channelID, err := uuid.Parse(r.PathValue("channelId"))
	if err != nil {
		jsonError(w, "invalid channel id", http.StatusBadRequest)
		return
	}

	dmRepo := &database.DMMemberRepo{DB: s.db}
	if err := dmRepo.CloseDM(r.Context(), channelID, user.ID); err != nil {
		jsonError(w, "failed to close DM", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Blocks ---

func (s *Server) handleBlock(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var input struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	targetID, err := uuid.Parse(input.UserID)
	if err != nil {
		jsonError(w, "invalid user_id", http.StatusBadRequest)
		return
	}

	if targetID == user.ID {
		jsonError(w, "cannot block yourself", http.StatusBadRequest)
		return
	}

	blockRepo := &database.BlockRepo{DB: s.db}
	if err := blockRepo.Block(r.Context(), user.ID, targetID); err != nil {
		jsonError(w, "failed to block user", http.StatusInternalServerError)
		return
	}

	// Close any open DM between the two users.
	dmRepo := &database.DMMemberRepo{DB: s.db}
	if ch, err := dmRepo.FindDMChannel(r.Context(), user.ID, targetID); err == nil {
		_ = dmRepo.CloseDM(r.Context(), ch.ID, user.ID)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnblock(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	targetID, err := uuid.Parse(r.PathValue("userId"))
	if err != nil {
		jsonError(w, "invalid user id", http.StatusBadRequest)
		return
	}

	blockRepo := &database.BlockRepo{DB: s.db}
	if err := blockRepo.Unblock(r.Context(), user.ID, targetID); err != nil {
		jsonError(w, "failed to unblock user", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListBlocks(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	blockRepo := &database.BlockRepo{DB: s.db}
	blocked, err := blockRepo.ListBlocked(r.Context(), user.ID)
	if err != nil {
		jsonError(w, "failed to list blocked users", http.StatusInternalServerError)
		return
	}
	if blocked == nil {
		blocked = []models.User{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(blocked)
}

// --- Presence ---

func (s *Server) handlePresence(w http.ResponseWriter, r *http.Request) {
	ids := s.hub.Presence().OnlineUserIDs()
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = id.String()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(strs)
}

// --- Messages ---

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	channelID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid channel id", http.StatusBadRequest)
		return
	}

	// Look up the channel to determine access.
	channelRepo := &database.ChannelRepo{DB: s.db}
	ch, err := channelRepo.GetChannelByID(r.Context(), channelID)
	if err == sql.ErrNoRows {
		jsonError(w, "channel not found", http.StatusNotFound)
		return
	}
	if err != nil {
		jsonError(w, "failed to get channel", http.StatusInternalServerError)
		return
	}

	// Access check: server channel vs DM channel.
	if ch.ServerID != nil {
		memberRepo := &database.ServerMemberRepo{DB: s.db}
		isMember, err := memberRepo.IsMember(r.Context(), user.ID, *ch.ServerID)
		if err != nil {
			jsonError(w, "failed to check membership", http.StatusInternalServerError)
			return
		}
		if !isMember {
			jsonError(w, "forbidden", http.StatusForbidden)
			return
		}
	} else {
		// DM channel: check dm_members.
		dmRepo := &database.DMMemberRepo{DB: s.db}
		isMember, err := dmRepo.IsMember(r.Context(), channelID, user.ID)
		if err != nil {
			jsonError(w, "failed to check DM membership", http.StatusInternalServerError)
			return
		}
		if !isMember {
			jsonError(w, "forbidden", http.StatusForbidden)
			return
		}
	}

	// Parse pagination params.
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 100 {
		limit = 100
	}

	var before *uuid.UUID
	if b := r.URL.Query().Get("before"); b != "" {
		parsed, err := uuid.Parse(b)
		if err != nil {
			jsonError(w, "invalid before cursor", http.StatusBadRequest)
			return
		}
		before = &parsed
	}

	msgRepo := &database.MessageRepo{DB: s.db}
	messages, err := msgRepo.ListByChannel(r.Context(), channelID, before, limit)
	if err != nil {
		jsonError(w, "failed to list messages", http.StatusInternalServerError)
		return
	}
	if messages == nil {
		messages = []models.Message{}
	}

	// Load attachments for each message.
	attachmentRepo := &database.AttachmentRepo{DB: s.db}
	for i := range messages {
		atts, err := attachmentRepo.ListByMessage(r.Context(), messages[i].ID)
		if err != nil {
			log.Printf("failed to load attachments for message %s: %v", messages[i].ID, err)
			continue
		}
		messages[i].Attachments = atts
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

// --- Create Message (multipart, with file uploads) ---

func (s *Server) handleCreateMessage(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	channelID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid channel id", http.StatusBadRequest)
		return
	}

	// Access check (same logic as handleListMessages).
	channelRepo := &database.ChannelRepo{DB: s.db}
	ch, err := channelRepo.GetChannelByID(r.Context(), channelID)
	if err == sql.ErrNoRows {
		jsonError(w, "channel not found", http.StatusNotFound)
		return
	}
	if err != nil {
		jsonError(w, "failed to get channel", http.StatusInternalServerError)
		return
	}

	if ch.ServerID != nil {
		memberRepo := &database.ServerMemberRepo{DB: s.db}
		isMember, err := memberRepo.IsMember(r.Context(), user.ID, *ch.ServerID)
		if err != nil {
			jsonError(w, "failed to check membership", http.StatusInternalServerError)
			return
		}
		if !isMember {
			jsonError(w, "forbidden", http.StatusForbidden)
			return
		}
	} else {
		dmRepo := &database.DMMemberRepo{DB: s.db}
		isMember, err := dmRepo.IsMember(r.Context(), channelID, user.ID)
		if err != nil {
			jsonError(w, "failed to check DM membership", http.StatusInternalServerError)
			return
		}
		if !isMember {
			jsonError(w, "forbidden", http.StatusForbidden)
			return
		}
	}

	// Parse multipart form — 10 MB max memory.
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		jsonError(w, "invalid multipart form", http.StatusBadRequest)
		return
	}

	content := r.FormValue("content")
	files := r.MultipartForm.File["files"]

	if content == "" && len(files) == 0 {
		jsonError(w, "message must have content or attachments", http.StatusBadRequest)
		return
	}
	if len(content) > 4000 {
		jsonError(w, "message content must be 4000 characters or less", http.StatusBadRequest)
		return
	}

	// Create the message.
	msg := &models.Message{
		ChannelID: channelID,
		AuthorID:  user.ID,
		Content:   content,
	}
	msgRepo := &database.MessageRepo{DB: s.db}
	if err := msgRepo.Create(r.Context(), msg); err != nil {
		jsonError(w, "failed to create message", http.StatusInternalServerError)
		return
	}

	// Process file uploads.
	attachmentRepo := &database.AttachmentRepo{DB: s.db}
	var attachments []models.Attachment

	for _, fh := range files {
		result, err := upload.ProcessFile(s.uploadDir, fh)
		if err != nil {
			log.Printf("upload error for %q: %v", fh.Filename, err)
			continue
		}

		att := models.Attachment{
			MessageID:    msg.ID,
			FilePath:     result.FilePath,
			OriginalName: result.OriginalName,
			MimeType:     &result.MimeType,
			FileSize:     &result.FileSize,
			Width:        result.Width,
			Height:       result.Height,
		}
		if err := attachmentRepo.Create(r.Context(), &att); err != nil {
			log.Printf("attachment db error for %q: %v", fh.Filename, err)
			continue
		}
		attachments = append(attachments, att)
	}

	msg.Attachments = attachments

	// Broadcast via WebSocket so other clients see it in real-time.
	out, err := json.Marshal(map[string]any{
		"type":    "message",
		"message": msg,
	})
	if err == nil {
		s.hub.BroadcastToChannel(channelID, out)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(msg)
}

// --- Edit / Delete Messages ---

func (s *Server) handleEditMessage(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	messageID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid message id", http.StatusBadRequest)
		return
	}

	var input struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	input.Content = strings.TrimSpace(input.Content)
	if input.Content == "" {
		jsonError(w, "content is required", http.StatusBadRequest)
		return
	}
	if len(input.Content) > 4000 {
		jsonError(w, "message content must be 4000 characters or less", http.StatusBadRequest)
		return
	}

	msgRepo := &database.MessageRepo{DB: s.db}
	updated, err := msgRepo.Update(r.Context(), messageID, user.ID, input.Content)
	if err == sql.ErrNoRows {
		jsonError(w, "message not found or not yours", http.StatusNotFound)
		return
	}
	if err != nil {
		jsonError(w, "failed to update message", http.StatusInternalServerError)
		return
	}

	// Broadcast edit via WebSocket.
	out, err := json.Marshal(map[string]any{
		"type":    "message_edit",
		"message": updated,
	})
	if err == nil {
		s.hub.BroadcastToChannel(updated.ChannelID, out)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

func (s *Server) handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	messageID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid message id", http.StatusBadRequest)
		return
	}

	msgRepo := &database.MessageRepo{DB: s.db}

	// Look up the message to get channel_id for WS broadcast.
	msg, err := msgRepo.GetByID(r.Context(), messageID)
	if err == sql.ErrNoRows {
		jsonError(w, "message not found", http.StatusNotFound)
		return
	}
	if err != nil {
		jsonError(w, "failed to get message", http.StatusInternalServerError)
		return
	}

	// Only the author can delete their own message.
	if msg.AuthorID != user.ID {
		jsonError(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := msgRepo.Delete(r.Context(), messageID, user.ID); err != nil {
		jsonError(w, "failed to delete message", http.StatusInternalServerError)
		return
	}

	// Broadcast delete via WebSocket.
	out, err := json.Marshal(map[string]any{
		"type":       "message_delete",
		"channel_id": msg.ChannelID.String(),
		"message_id": messageID.String(),
	})
	if err == nil {
		s.hub.BroadcastToChannel(msg.ChannelID, out)
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Read State ---

func (s *Server) handleMarkRead(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	channelID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid channel id", http.StatusBadRequest)
		return
	}

	rsRepo := &database.ReadStateRepo{DB: s.db}

	// Find the latest message in this channel.
	latestID, err := rsRepo.GetLatestMessageID(r.Context(), channelID)
	if err != nil {
		jsonError(w, "failed to get latest message", http.StatusInternalServerError)
		return
	}

	if err := rsRepo.UpdateReadState(r.Context(), user.ID, channelID, latestID); err != nil {
		jsonError(w, "failed to update read state", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnreadCounts(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	serverID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid server id", http.StatusBadRequest)
		return
	}

	// Check membership.
	memberRepo := &database.ServerMemberRepo{DB: s.db}
	isMember, err := memberRepo.IsMember(r.Context(), user.ID, serverID)
	if err != nil {
		jsonError(w, "failed to check membership", http.StatusInternalServerError)
		return
	}
	if !isMember {
		jsonError(w, "forbidden", http.StatusForbidden)
		return
	}

	rsRepo := &database.ReadStateRepo{DB: s.db}
	counts, err := rsRepo.GetUnreadCounts(r.Context(), user.ID, serverID)
	if err != nil {
		jsonError(w, "failed to get unread counts", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(counts)
}
