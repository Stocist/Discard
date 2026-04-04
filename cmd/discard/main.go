package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"

	"github.com/Stocist/discard/internal/database"
	"github.com/Stocist/discard/internal/frontend"
	"github.com/Stocist/discard/internal/server"
	discardTurn "github.com/Stocist/discard/internal/turn"
	"github.com/Stocist/discard/internal/voice"
	"github.com/Stocist/discard/internal/websocket"
)

func main() {
	devMode := strings.EqualFold(os.Getenv("DISCARD_DEV"), "true")
	prodMode := strings.EqualFold(os.Getenv("DISCARD_PRODUCTION"), "true")

	if devMode && prodMode {
		log.Fatal("DISCARD_DEV and DISCARD_PRODUCTION cannot both be true. Refusing to start.")
	}
	if devMode {
		log.Println("WARNING: Running in dev mode — authentication is disabled. Do NOT use in production.")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://localhost:5432/discard?sslmode=disable"
	}

	db, err := database.Connect(dbURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	hub := websocket.NewHub()
	go hub.Run()

	sendToUser := func(userID uuid.UUID, data []byte) {
		hub.SendToUser(userID, data)
	}
	voiceMgr := voice.NewManager(hub.BroadcastAll, sendToUser)
	voiceMgr.StartSweeper()
	voiceAdapter := voice.NewAdapter(voiceMgr)

	turnServer, err := discardTurn.Start("discard")
	if err != nil {
		log.Fatalf("failed to start TURN server: %v", err)
	}
	defer turnServer.Close()

	turnSecret := os.Getenv("TURN_SECRET")

	srv := server.NewServer(db, hub, voiceAdapter, turnSecret)
	srv.SetupRoutes()

	// Serve embedded frontend with SPA fallback
	frontendFS, err := frontend.FS()
	if err != nil {
		log.Fatalf("failed to load embedded frontend: %v", err)
	}
	srv.Router().Handle("/", frontend.SPAHandler(frontendFS))

	addr := ":" + os.Getenv("PORT")
	if addr == ":" {
		addr = ":4000"
	}
	log.Printf("discard listening on %s", addr)
	if err := http.ListenAndServe(addr, srv.Router()); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
