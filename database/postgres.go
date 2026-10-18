package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// NewPostgres reads DATABASE_URL and opens and verifies a PostgreSQL connection pool.
// The caller owns the returned pool and must close it on shutdown.
func NewPostgres(ctx context.Context) (*sql.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL connection pool: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return db, nil
}
