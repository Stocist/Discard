package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/Stocist/discard/internal/database"
	"github.com/Stocist/discard/internal/frontend"
	"github.com/Stocist/discard/internal/server"
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

	srv := server.NewServer(db, hub)
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
