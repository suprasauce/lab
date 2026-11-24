package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// EnsureOptionChain checks database availability without triggering ingestion.
func (r *Repository) EnsureOptionChain(ctx context.Context, underlying string, expiry time.Time) error {
	r.logger.DebugContext(ctx, "checking option chain", "underlying", underlying, "expiry", expiry)
	var expiryID int64
	var fetched bool
	err := r.db.QueryRowContext(ctx, queryGetOptionChainStatus, underlying, expiry).Scan(&expiryID, &fetched)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !fetched) {
		return fmt.Errorf("option chain not available for %s expiry %s", underlying, expiry.Format(time.DateOnly))
	}
	if err != nil {
		return fmt.Errorf("get option chain status for %s expiry %s: %w", underlying, expiry.Format(time.DateOnly), err)
	}
	return nil
}
