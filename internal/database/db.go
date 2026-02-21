package database

import (
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Connect opens a connection pool to the PostgreSQL database.
func Connect(connStr string) (*sql.DB, error) {
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
