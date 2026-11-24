package repository

import (
	"database/sql"
	"log/slog"
	"sync"

	"optionsbacktest/models"
)

type Repository struct {
	// candles is reserved for indicator data, keyed by trading symbol.
	candles   map[string][]models.Candle
	candlesMu sync.RWMutex
	db        *sql.DB
	logger    *slog.Logger
}

func NewRepository(db *sql.DB, logger *slog.Logger) *Repository {
	return &Repository{
		candles: make(map[string][]models.Candle),
		db:      db,
		logger:  logger.With("component", "repository"),
	}
}
