package selector

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"optionsbacktest/models"
	"optionsbacktest/repository"
)

func selectorRepository(t *testing.T) (*repository.Repository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		mock.ExpectClose()
		if err := db.Close(); err != nil {
			t.Error(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
	})
	return repository.NewRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil))), mock
}

func TestOffsetSelectorUsesCandleOpen(t *testing.T) {
	repo, mock := selectorRepository(t)
	expiry := time.Date(2026, 1, 27, 0, 0, 0, 0, time.UTC)
	at := expiry.AddDate(0, 0, -45).Add(9*time.Hour + 30*time.Minute)
	mock.ExpectQuery("FROM lab.option_contracts").WithArgs(int64(7), expiry, int64(2265000), "CALL").
		WillReturnRows(sqlmock.NewRows([]string{"id", "underlying_id", "expiry_id", "expiry_date", "strike_price", "option_type"}).
			AddRow(101, 7, 9, expiry, 2265000, "CALL")).RowsWillBeClosed()
	var s Selector = NewOffsetSelector(models.Call, 2)
	contract, err := s.Select(context.Background(), 7, &models.Candle{Timestamp: at, Open: 22530 * 100, Close: 22500 * 100}, expiry, repo)
	if err != nil || contract == nil || contract.ID != 101 || contract.StrikePrice != 2265000 {
		t.Fatalf("selection=%+v error=%v", contract, err)
	}
}

func TestOffsetSelectorWaitsForUnderlyingCandle(t *testing.T) {
	var s Selector = NewOffsetSelector(models.Call, 2)
	contract, err := s.Select(context.Background(), 7, nil, time.Time{}, nil)
	if err != nil || contract != nil {
		t.Fatalf("selection=%+v error=%v", contract, err)
	}
}

func TestOffsetSelectorRejectsInvalidType(t *testing.T) {
	_, err := NewOffsetSelector("CE", 0).Select(context.Background(), 7, nil, time.Time{}, nil)
	if err == nil || !strings.Contains(err.Error(), "option_type") {
		t.Fatalf("invalid option type accepted: %v", err)
	}
}

func TestOffsetSelectorPropagatesRepositoryError(t *testing.T) {
	repo, mock := selectorRepository(t)
	failure := errors.New("database unavailable")
	mock.ExpectQuery("FROM lab.option_contracts").WillReturnError(failure)
	_, err := NewOffsetSelector(models.Call, 0).Select(context.Background(), 7, &models.Candle{Open: 2250000}, time.Time{}, repo)
	if !errors.Is(err, failure) {
		t.Fatalf("lost database error: %v", err)
	}
}

func TestIDSelectorResolvesWithoutUnderlyingCandle(t *testing.T) {
	repo, mock := selectorRepository(t)
	expiry := time.Date(2026, 1, 27, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery("FROM lab.option_contracts").WithArgs(int64(101)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "underlying_id", "expiry_id", "expiry_date", "strike_price", "option_type"}).
			AddRow(101, 7, 9, expiry, 2265000, "CALL")).RowsWillBeClosed()
	var s Selector = NewIDSelector(101)
	contract, err := s.Select(context.Background(), 7, nil, expiry, repo)
	if err != nil || contract == nil || contract.ID != 101 {
		t.Fatalf("selection=%+v error=%v", contract, err)
	}
}

func TestIDSelectorPropagatesMissingContract(t *testing.T) {
	repo, mock := selectorRepository(t)
	mock.ExpectQuery("FROM lab.option_contracts").WithArgs(int64(404)).WillReturnError(sql.ErrNoRows)
	_, err := NewIDSelector(404).Select(context.Background(), 7, nil, time.Time{}, repo)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("lost missing contract error: %v", err)
	}
}

func TestIDSelectorRejectsInvalidID(t *testing.T) {
	_, err := NewIDSelector(0).Select(context.Background(), 7, nil, time.Time{}, nil)
	if err == nil || !strings.Contains(err.Error(), "positive") {
		t.Fatalf("invalid ID accepted: %v", err)
	}
}

func TestIDSelectorChecksUnderlyingAndExpiry(t *testing.T) {
	for _, mismatch := range []string{"underlying", "expiry"} {
		t.Run(mismatch, func(t *testing.T) {
			repo, mock := selectorRepository(t)
			expiry := time.Date(2026, 1, 27, 0, 0, 0, 0, time.UTC)
			actualExpiry, actualUnderlying := expiry, int64(7)
			if mismatch == "underlying" {
				actualUnderlying = 8
			} else {
				actualExpiry = expiry.AddDate(0, 1, 0)
			}
			mock.ExpectQuery("FROM lab.option_contracts").WithArgs(int64(101)).
				WillReturnRows(sqlmock.NewRows([]string{"id", "underlying_id", "expiry_id", "expiry_date", "strike_price", "option_type"}).
					AddRow(101, actualUnderlying, 9, actualExpiry, 2265000, "CALL")).RowsWillBeClosed()
			_, err := NewIDSelector(101).Select(context.Background(), 7, nil, expiry, repo)
			if err == nil || !strings.Contains(err.Error(), "current underlying and expiry") {
				t.Fatalf("foreign contract accepted: %v", err)
			}
		})
	}
}

func TestUnimplementedSelectors(t *testing.T) {
	for _, s := range []Selector{PremiumSelector{}} {
		if _, err := s.Select(context.Background(), 7, nil, time.Time{}, nil); err == nil || !strings.Contains(err.Error(), "not implemented") {
			t.Fatalf("expected not implemented, got %v", err)
		}
	}
}
