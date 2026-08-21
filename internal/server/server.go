package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/google/uuid"
	gorillaWs "github.com/gorilla/websocket"

	"github.com/Stocist/discard/internal/auth"
	"github.com/Stocist/discard/internal/database"
	"github.com/Stocist/discard/internal/models"
	discardTurn "github.com/Stocist/discard/internal/turn"
	ws "github.com/Stocist/discard/internal/websocket"
)

var upgrader = gorillaWs.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type Server struct {
	db         *sql.DB
	hub        *ws.Hub
	router     *http.ServeMux
	uploadDir  string
	voiceMgr   ws.VoiceHandler
	turnSecret string
	blockMu    sync.Mutex
}

func NewServer(db *sql.DB, hub *ws.Hub, voiceMgr ws.VoiceHandler, turnSecret string) *Server {
	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "./uploads"
	}
	blockRepo := &database.BlockRepo{DB: db}
	hub.SetPresenceVisibilityChecker(func(ctx context.Context, viewerID, subjectID uuid.UUID) (bool, error) {
		blocked, err := blockRepo.IsBlocked(ctx, viewerID, subjectID)
		return !blocked, err
	})
	return &Server{
		db:         db,
		hub:        hub,
		router:     http.NewServeMux(),
		uploadDir:  uploadDir,
		voiceMgr:   voiceMgr,
		turnSecret: turnSecret,
	}
}

func (s *Server) Router() *http.ServeMux {
	return s.router
}

func (s *Server) SetupRoutes() {
	authed := auth.Middleware(&database.UserRepo{DB: s.db})
	a := func(pattern string, h http.HandlerFunc) {
		s.router.Handle(pattern, authed(h))
	}

	// Public (no auth required)
	s.router.HandleFunc("GET /api/health", s.handleHealth)
	s.router.HandleFunc("GET /api/tailscale/status", s.handleTailscaleStatus)

	// Me
	a("GET /api/me", s.handleMe)
	a("PUT /api/me", s.handleUpdateMe)

	// Servers
	a("POST /api/servers", s.handleCreateServer)
	a("GET /api/servers", s.handleListServers)
	a("POST /api/servers/join", s.handleJoinServer)
	a("GET /api/servers/{id}", s.handleGetServer)
	a("PUT /api/servers/{id}", s.handleUpdateServer)
	a("DELETE /api/servers/{id}", s.handleDeleteServer)

	a("POST /api/servers/{id}/invite-code/regenerate", s.handleRegenerateInviteCode)

	// Channels
	a("POST /api/servers/{id}/channels", s.handleCreateChannel)
	a("GET /api/servers/{id}/channels", s.handleListChannels)
	a("PUT /api/servers/{id}/channels/{channelId}", s.handleUpdateChannel)
	a("DELETE /api/servers/{id}/channels/{channelId}", s.handleDeleteChannel)

	// Members
	a("GET /api/servers/{id}/members", s.handleListMembers)
	a("DELETE /api/servers/{id}/members/me", s.handleLeaveServer)

	// DMs
	a("POST /api/dm/open", s.handleOpenDM)
	a("GET /api/dm", s.handleListDMs)
	a("GET /api/dm/{channelId}", s.handleGetDM)
	a("PUT /api/dm/{channelId}/close", s.handleCloseDM)

	// Blocks
	a("POST /api/blocks", s.handleBlock)
	a("DELETE /api/blocks/{userId}", s.handleUnblock)
	a("GET /api/blocks", s.handleListBlocks)

	// Messages
	a("GET /api/channels/{id}/messages", s.handleListMessages)
	a("POST /api/channels/{id}/messages", s.handleCreateMessage)
	a("PUT /api/messages/{id}", s.handleEditMessage)
	a("DELETE /api/messages/{id}", s.handleDeleteMessage)

	// Read state / unread
	a("PUT /api/channels/{id}/read", s.handleMarkRead)
	a("GET /api/servers/{id}/unread", s.handleUnreadCounts)

	// Uploads — static file server
	s.router.Handle("GET /uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(s.uploadDir))))

	// TURN
	a("GET /api/turn/credentials", s.handleTurnCredentials)

	// Presence
	a("GET /api/presence", s.handlePresence)

	// WebSocket
	a("GET /api/ws", s.handleWebSocket)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleTailscaleStatus is a public (no-auth) endpoint that checks whether the
// requester is on the Tailscale network and, if so, returns their user info.
func (s *Server) handleTailscaleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	devMode := strings.EqualFold(os.Getenv("DISCARD_DEV"), "true")
	if devMode {
		json.NewEncoder(w).Encode(map[string]any{
			"on_tailscale":  true,
			"authenticated": true,
			"dev_mode":      true,
		})
		return
	}

	// Try to authenticate via auth middleware's exported function.
	userRepo := &database.UserRepo{DB: s.db}
	user, err := auth.TailscaleStatus(r, userRepo)
	if err != nil {
		// Not on Tailscale or whois failed
		json.NewEncoder(w).Encode(map[string]any{
			"on_tailscale":  false,
			"authenticated": false,
			"error":         "Not connected to Tailscale network",
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]any{
		"on_tailscale":  true,
		"authenticated": true,
		"user": map[string]any{
			"id":           user.ID,
			"username":     user.Username,
			"display_name": user.DisplayName,
			"avatar_path":  user.AvatarPath,
		},
	})
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}

	msgRepo := &database.MessageRepo{DB: s.db}
	handler := func(ctx context.Context, channelID, authorID uuid.UUID, content string) (*models.Message, error) {
		msg := &models.Message{
			ChannelID: channelID,
			AuthorID:  authorID,
			Content:   content,
		}
		if err := msgRepo.Create(ctx, msg); err != nil {
			if errors.Is(err, database.ErrBlocked) {
				return nil, ws.ErrMessageForbidden
			}
			return nil, err
		}
		return msg, nil
	}

	channelRepo := &database.ChannelRepo{DB: s.db}
	memberRepo := &database.ServerMemberRepo{DB: s.db}
	dmRepo := &database.DMMemberRepo{DB: s.db}
	checker := func(ctx context.Context, userID, channelID uuid.UUID) (bool, error) {
		ch, err := channelRepo.GetChannelByID(ctx, channelID)
		if err != nil {
			return false, err
		}
		if ch.ServerID != nil {
			return memberRepo.IsMember(ctx, userID, *ch.ServerID)
		}
		return dmRepo.IsMember(ctx, channelID, userID)
	}
	messageChecker := func(ctx context.Context, userID, channelID uuid.UUID) (bool, error) {
		ch, err := channelRepo.GetChannelByID(ctx, channelID)
		if err != nil {
			return false, err
		}
		if ch.ServerID != nil {
			return true, nil
		}
		otherID, err := dmRepo.GetOtherUserID(ctx, channelID, userID)
		if err != nil {
			return false, err
		}
		blocked, err := (&database.BlockRepo{DB: s.db}).IsBlocked(ctx, userID, otherID)
		return !blocked, err
	}

	client := ws.NewClient(conn, user.ID, handler, checker)
	client.CanSendMessage = messageChecker
	if user.DisplayName != nil {
		client.Username = *user.DisplayName
	} else {
		client.Username = user.Username
	}
	if user.AvatarPath != nil {
		client.AvatarPath = *user.AvatarPath
	}
	client.OnVoice = s.voiceMgr
	s.hub.Register(client)
	go client.WritePump()
	go client.ReadPump()
}

func (s *Server) handleTurnCredentials(w http.ResponseWriter, r *http.Request) {
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	username, credential, ttl := discardTurn.GenerateCredentials(s.turnSecret)
	turnPort, err := discardTurn.Port()
	if err != nil {
		http.Error(w, `{"error":"invalid TURN configuration"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"urls": []string{
			fmt.Sprintf("turn:%s:%d?transport=udp", host, turnPort),
		},
		"username":   username,
		"credential": credential,
		"ttl":        ttl,
	})
}
