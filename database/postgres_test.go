package database

import (
	"context"
	"errors"
	"testing"
)

func TestNewPostgresConnectionFormats(t *testing.T) {
	for _, dsn := range []string{
		"postgres://localhost:5432/backtest",
		"host=localhost port=5432 dbname=backtest",
		"host=localhost port=5432 user=postgres password=password dbname=optionsbacktest sslmode=disable",
	} {
		t.Run(dsn, func(t *testing.T) {
			t.Setenv("DATABASE_URL", dsn)
			// Cancellation prevents network access while checking parsing and cleanup.
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			db, err := NewPostgres(ctx)
			if db != nil || !errors.Is(err, context.Canceled) {
				t.Fatalf("expected canceled connectivity check, got db=%v err=%v", db, err)
			}
		})
	}
}
