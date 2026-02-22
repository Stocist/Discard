package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	"github.com/google/uuid"
	gorillaWs "github.com/gorilla/websocket"

	"github.com/Stocist/discard/internal/auth"
	"github.com/Stocist/discard/internal/database"
	"github.com/Stocist/discard/internal/models"
	ws "github.com/Stocist/discard/internal/websocket"
)

var upgrader = gorillaWs.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type Server struct {
	db     *sql.DB
	hub    *ws.Hub
	router *http.ServeMux
}

func NewServer(db *sql.DB, hub *ws.Hub) *Server {
	return &Server{
		db:     db,
		hub:    hub,
		router: http.NewServeMux(),
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

	// Public
	s.router.HandleFunc("GET /api/health", s.handleHealth)

	// Me
	a("GET /api/me", s.handleMe)

	// Servers
	a("POST /api/servers", s.handleCreateServer)
	a("GET /api/servers", s.handleListServers)
	a("POST /api/servers/join", s.handleJoinServer)
	a("GET /api/servers/{id}", s.handleGetServer)

	// Channels
	a("POST /api/servers/{id}/channels", s.handleCreateChannel)
	a("GET /api/servers/{id}/channels", s.handleListChannels)

	// Members
	a("GET /api/servers/{id}/members", s.handleListMembers)
	a("DELETE /api/servers/{id}/members/me", s.handleLeaveServer)

	// Friends
	a("POST /api/friends/requests", s.handleSendFriendRequest)
	a("POST /api/friends/requests/{id}/accept", s.handleAcceptFriend)
	a("GET /api/friends", s.handleListFriends)

	// Messages
	a("GET /api/channels/{id}/messages", s.handleListMessages)

	// WebSocket
	a("GET /api/ws", s.handleWebSocket)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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

	client := ws.NewClient(conn, user.ID, handler, checker)
	s.hub.Register(client)
	go client.WritePump()
	go client.ReadPump()
}
