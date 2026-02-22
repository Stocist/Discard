package database

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate runs all pending SQL migrations against the database.
// It tracks applied migrations in a schema_migrations table.
func Migrate(db *sql.DB) error {
	if err := ensureMigrationsTable(db); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	applied, err := getAppliedMigrations(db)
	if err != nil {
		return fmt.Errorf("get applied migrations: %w", err)
	}

	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	// Sort by filename to ensure order.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		if applied[name] {
			continue
		}

		content, err := migrationFiles.ReadFile(filepath.Join("migrations", name))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", name, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("execute migration %s: %w", name, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (name) VALUES ($1)", name); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}

		log.Printf("applied migration: %s", name)
	}

	return nil
}

func ensureMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name       VARCHAR(256) PRIMARY KEY,
			applied_at TIMESTAMPTZ DEFAULT NOW()
		)
	`)
	return err
}

func getAppliedMigrations(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query("SELECT name FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		applied[name] = true
	}
	return applied, rows.Err()
}
