package repository

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"optionsbacktest/models"
)

// fetchOptionContract waits for Python to commit ingestion. PostgreSQL remains
// authoritative: the caller queries the contract again rather than trusting its ID here.
func (r *Repository) fetchOptionContract(ctx context.Context, symbol string, expiry time.Time, strike models.Price, optionType models.OptionType) error {
	return r.fetchOptionData(ctx, symbol, expiry, strike, optionType, nil)
}

func (r *Repository) fetchOptionData(ctx context.Context, symbol string, expiry time.Time, strike models.Price, optionType models.OptionType, day *time.Time) error {
	baseURL := strings.TrimSpace(r.fetcherURL)
	if baseURL == "" {
		return fmt.Errorf("DATA_FETCHER_URL is required to ingest missing option contracts")
	}
	dayValue := ""
	if day != nil {
		dayValue = day.Format(time.DateOnly)
	}
	payload, err := json.Marshal(struct {
		Underlying string            `json:"underlying"`
		Expiry     string            `json:"expiry"`
		Strike     json.Number       `json:"strike"`
		OptionType models.OptionType `json:"option_type"`
		Day        string            `json:"day,omitempty"`
	}{
		Underlying: symbol,
		Expiry:     expiry.Format(time.DateOnly),
		// Exact rational formatting supports all int64 paise values without floats.
		Strike:     json.Number(big.NewRat(int64(strike), 100).FloatString(2)),
		OptionType: optionType,
		Day:        dayValue,
	})
	if err != nil {
		return fmt.Errorf("encode option ingestion request: %w", err)
	}
	path := "/options/fetch"
	if day != nil {
		path = "/options/fetch-day"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create option ingestion request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request option ingestion: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return fmt.Errorf("read option ingestion response: %w", err)
	}
	var result struct {
		Status     string `json:"status"`
		ContractID int64  `json:"contract_id"`
		Error      string `json:"error"`
	}
	decodeErr := json.Unmarshal(body, &result)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if result.Error != "" {
			return fmt.Errorf("option ingestion API returned %s: %s", response.Status, result.Error)
		}
		return fmt.Errorf("option ingestion API returned %s", response.Status)
	}
	if decodeErr != nil {
		return fmt.Errorf("decode option ingestion response: %w", decodeErr)
	}
	if day != nil && result.Status == "no_data" && result.Error == "" {
		return fmt.Errorf("no option candles for %s: %w", day.Format(time.DateOnly), sql.ErrNoRows)
	}
	if result.Error != "" || result.ContractID <= 0 || (result.Status != "ingested" && result.Status != "already_exists") {
		return fmt.Errorf("option ingestion API did not confirm completed ingestion (status %q)", result.Status)
	}
	return nil
}
