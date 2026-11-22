package repository

import (
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"optionsbacktest/models"
)

type Repository struct {
	// candles is reserved for indicator data, keyed by trading symbol.
	candles    map[string][]models.Candle
	candlesMu  sync.RWMutex
	db         *sql.DB
	logger     *slog.Logger
	fetcherURL string
	httpClient *http.Client
}

func NewRepository(db *sql.DB, logger *slog.Logger) *Repository {
	return &Repository{
		candles:    make(map[string][]models.Candle),
		db:         db,
		logger:     logger.With("component", "repository"),
		fetcherURL: os.Getenv("DATA_FETCHER_URL"),
		httpClient: &http.Client{
			Timeout:       30 * time.Minute,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}
