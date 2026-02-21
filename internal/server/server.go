package server

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/Stocist/discard/internal/websocket"
)

type Server struct {
	db     *sql.DB
	hub    *websocket.Hub
	router *http.ServeMux
}

func NewServer(db *sql.DB, hub *websocket.Hub) *Server {
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
	s.router.HandleFunc("GET /api/health", s.handleHealth)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
