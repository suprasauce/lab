package repository

import (
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"time"
)

type Repository struct {
	db         *sql.DB
	logger     *slog.Logger
	fetcherURL string
	httpClient *http.Client
}

func NewRepository(db *sql.DB, logger *slog.Logger) *Repository {
	return &Repository{
		db:         db,
		logger:     logger.With("component", "repository"),
		fetcherURL: os.Getenv("DATA_FETCHER_URL"),
		httpClient: &http.Client{
			Timeout:       30 * time.Minute,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}
