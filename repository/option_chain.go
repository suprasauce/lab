package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// EnsureOptionChain waits for full-chain ingestion only when the expiry is not
// already marked complete. Python owns all writes and the completion flag.
func (r *Repository) EnsureOptionChain(ctx context.Context, underlying string, expiry time.Time) error {
	r.logger.DebugContext(ctx, "checking option chain", "underlying", underlying, "expiry", expiry)
	var expiryID int64
	var fetched bool
	if err := r.db.QueryRowContext(ctx, queryGetOptionChainStatus, underlying, expiry).Scan(&expiryID, &fetched); err != nil {
		return fmt.Errorf("get option chain status for %s expiry %s: %w", underlying, expiry.Format(time.DateOnly), err)
	}
	if fetched {
		return nil
	}
	if err := r.fetchOptionChain(ctx, underlying, expiry); err != nil {
		return fmt.Errorf("fetch option chain for %s expiry %s: %w", underlying, expiry.Format(time.DateOnly), err)
	}
	if err := r.db.QueryRowContext(ctx, queryGetOptionChainStatus, underlying, expiry).Scan(&expiryID, &fetched); err != nil {
		return fmt.Errorf("verify option chain after ingestion: %w", err)
	}
	if !fetched {
		return fmt.Errorf("option chain for %s expiry %s remains incomplete after ingestion", underlying, expiry.Format(time.DateOnly))
	}
	return nil
}

func (r *Repository) fetchOptionChain(ctx context.Context, underlying string, expiry time.Time) error {
	baseURL := strings.TrimSpace(r.fetcherURL)
	if baseURL == "" {
		return fmt.Errorf("DATA_FETCHER_URL is required to ingest option chains")
	}
	payload, err := json.Marshal(struct {
		Underlying string `json:"underlying"`
		Expiry     string `json:"expiry"`
	}{Underlying: underlying, Expiry: expiry.Format(time.DateOnly)})
	if err != nil {
		return fmt.Errorf("encode option chain request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/option-chain/fetch", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create option chain request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request option chain ingestion: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return fmt.Errorf("read option chain response: %w", err)
	}
	var result struct {
		Status   string `json:"status"`
		ExpiryID int64  `json:"expiry_id"`
		Error    string `json:"error"`
	}
	decodeErr := json.Unmarshal(body, &result)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if result.Error != "" {
			return fmt.Errorf("option chain API returned %s: %s", response.Status, result.Error)
		}
		return fmt.Errorf("option chain API returned %s", response.Status)
	}
	if decodeErr != nil {
		return fmt.Errorf("decode option chain response: %w", decodeErr)
	}
	if result.Error != "" || result.ExpiryID <= 0 || (result.Status != "ingested" && result.Status != "already_exists") {
		return fmt.Errorf("option chain API did not confirm completed ingestion (status %q)", result.Status)
	}
	return nil
}
