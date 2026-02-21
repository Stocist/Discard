package main

import (
	"log"
	"net/http"
	"os"

	"github.com/Stocist/discard/internal/database"
	"github.com/Stocist/discard/internal/server"
	"github.com/Stocist/discard/internal/websocket"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://localhost:5432/discard?sslmode=disable"
	}

	db, err := database.Connect(dbURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	hub := websocket.NewHub()
	go hub.Run()

	srv := server.NewServer(db, hub)
	srv.SetupRoutes()

	addr := ":" + os.Getenv("PORT")
	if addr == ":" {
		addr = ":4000"
	}
	log.Printf("discard listening on %s", addr)
	if err := http.ListenAndServe(addr, srv.Router()); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
