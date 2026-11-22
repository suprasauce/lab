package chart

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"optionsbacktest/models"
	"optionsbacktest/repository"
)

func TestEMAIncremental(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := repository.NewRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s := NewChartService(repo)
	start := time.Date(2026, 1, 5, 9, 15, 0, 0, time.FixedZone("IST", 19800))
	rows := sqlmock.NewRows([]string{"start_time", "timeframe", "open", "high", "low", "close", "volume"})
	for i, close := range []int{10000, 10500, 11000} {
		// The third bar follows a missing interval.
		minutes := []int{0, 5, 15}[i]
		rows.AddRow(start.Add(time.Duration(minutes)*time.Minute), "5m", close, close, close, close, 0)
	}
	mock.ExpectQuery("SELECT c.start_time").WithArgs("NIFTY").WillReturnRows(rows)
	for _, tt := range []struct {
		minutes, index int
		want           float64
	}{
		{0, 0, 0}, {5, 1, 100}, {5, 1, 100}, {9, 1, 100},
		{10, 2, 101}, {15, 2, 101}, {20, 3, 102.8}, {25, 3, 102.8},
	} {
		value, err := s.EMA(context.Background(), "NIFTY", 9, models.Timeframe5m, start.Add(time.Duration(tt.minutes)*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if tt.index == 0 {
			if value != nil {
				t.Fatal("unfinished seed candle exposed")
			}
		} else if value == nil || math.Abs(*value-tt.want) > 1e-9 {
			t.Fatalf("at %d: %v; want %v", tt.minutes, value, tt.want)
		}
		if s.indicators["ema:NIFTY:9:5m"].(*emaMetadata).NextIndex != tt.index {
			t.Fatal("incorrect incremental index")
		}
	}
	// A different period has independent metadata, but shares repository candles.
	value, err := s.EMA(context.Background(), "NIFTY", 1, models.Timeframe5m, start.Add(20*time.Minute))
	if err != nil || value == nil || *value != 110 {
		t.Fatalf("period 1: %v, %v", value, err)
	}
	other := NewChartService(repo)
	value, err = other.EMA(context.Background(), "NIFTY", 9, models.Timeframe5m, start.Add(5*time.Minute))
	if err != nil || value == nil || *value != 100 {
		t.Fatal("metadata leaked across executions")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEMAEmptyAndDatabaseError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewChartService(repository.NewRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil))))
	failure := errors.New("database unavailable")
	mock.ExpectQuery("SELECT c.start_time").WithArgs("NIFTY").WillReturnError(failure)
	if _, err := s.EMA(context.Background(), "NIFTY", 9, models.Timeframe5m, time.Now()); !errors.Is(err, failure) {
		t.Fatalf("error: %v", err)
	}
	mock.ExpectQuery("SELECT c.start_time").WithArgs("NIFTY").WillReturnRows(sqlmock.NewRows([]string{"start_time", "timeframe", "open", "high", "low", "close", "volume"}))
	value, err := s.EMA(context.Background(), "NIFTY", 9, models.Timeframe5m, time.Now())
	if err != nil || value != nil {
		t.Fatalf("empty history: %v, %v", value, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
