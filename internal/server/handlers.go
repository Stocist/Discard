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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(srv)
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

// --- Friends ---

func (s *Server) handleSendFriendRequest(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var input struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if input.Username == "" {
		jsonError(w, "username is required", http.StatusBadRequest)
		return
	}
	if len(input.Username) > 32 {
		jsonError(w, "username must be 32 characters or less", http.StatusBadRequest)
		return
	}

	// Cannot friend yourself.
	if input.Username == user.Username {
		jsonError(w, "cannot send friend request to yourself", http.StatusBadRequest)
		return
	}

	userRepo := &database.UserRepo{DB: s.db}
	target, err := userRepo.GetByUsername(r.Context(), input.Username)
	if err == sql.ErrNoRows {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}
	if err != nil {
		jsonError(w, "failed to look up user", http.StatusInternalServerError)
		return
	}

	friendRepo := &database.FriendshipRepo{DB: s.db}
	f := &models.Friendship{
		UserA:       user.ID,
		UserB:       target.ID,
		InitiatedBy: user.ID,
	}
	if err := friendRepo.CreateFriendRequest(r.Context(), f); err != nil {
		jsonError(w, "failed to send friend request", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(f)
}

func (s *Server) handleAcceptFriend(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	friendshipID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid friendship id", http.StatusBadRequest)
		return
	}

	friendRepo := &database.FriendshipRepo{DB: s.db}
	f, err := friendRepo.GetByID(r.Context(), friendshipID)
	if err == sql.ErrNoRows {
		jsonError(w, "friend request not found", http.StatusNotFound)
		return
	}
	if err != nil {
		jsonError(w, "failed to get friend request", http.StatusInternalServerError)
		return
	}

	// Only the non-initiator can accept.
	if f.InitiatedBy == user.ID {
		jsonError(w, "cannot accept your own friend request", http.StatusForbidden)
		return
	}
	// Verify the current user is part of this friendship.
	if f.UserA != user.ID && f.UserB != user.ID {
		jsonError(w, "forbidden", http.StatusForbidden)
		return
	}
	if f.Status != "pending" {
		jsonError(w, "friend request is not pending", http.StatusConflict)
		return
	}

	// Accept the friendship.
	if err := friendRepo.AcceptFriend(r.Context(), friendshipID); err != nil {
		jsonError(w, "failed to accept friend request", http.StatusInternalServerError)
		return
	}

	// Create a DM channel between the two users.
	dmChannel := &models.Channel{
		Type: "dm",
	}
	channelRepo := &database.ChannelRepo{DB: s.db}
	if err := channelRepo.CreateChannel(r.Context(), dmChannel); err != nil {
		jsonError(w, "failed to create DM channel", http.StatusInternalServerError)
		return
	}

	// Add both users to the DM channel.
	dmMemberRepo := &database.DMMemberRepo{DB: s.db}
	if err := dmMemberRepo.AddMember(r.Context(), dmChannel.ID, f.UserA); err != nil {
		jsonError(w, "failed to add DM member", http.StatusInternalServerError)
		return
	}
	if err := dmMemberRepo.AddMember(r.Context(), dmChannel.ID, f.UserB); err != nil {
		jsonError(w, "failed to add DM member", http.StatusInternalServerError)
		return
	}

	// Link the DM channel to the friendship.
	if err := friendRepo.SetDMChannelID(r.Context(), friendshipID, dmChannel.ID); err != nil {
		jsonError(w, "failed to link DM channel", http.StatusInternalServerError)
		return
	}

	f.Status = "accepted"
	f.DMChannelID = &dmChannel.ID

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(f)
}

func (s *Server) handleListFriends(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	friendRepo := &database.FriendshipRepo{DB: s.db}
	friends, err := friendRepo.ListFriends(r.Context(), user.ID)
	if err != nil {
		jsonError(w, "failed to list friends", http.StatusInternalServerError)
		return
	}
	if friends == nil {
		friends = []models.Friendship{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(friends)
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
