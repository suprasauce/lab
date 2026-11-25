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
	for minute := 0; minute < 20; minute++ {
		if minute == 12 { // Skip this entire 5-minute window, despite having its final close.
			continue
		}
		close := []int{10000, 10500, 99900, 11000}[minute/5]
		if minute%5 != 4 {
			close = 999999 // Only the last minute's close belongs in the EMA.
		}
		rows.AddRow(start.Add(time.Duration(minute)*time.Minute), "1m", close, close, close, close, 0)
	}
	mock.ExpectQuery("SELECT c.start_time").WithArgs("NIFTY").WillReturnRows(rows)
	for _, tt := range []struct {
		minutes, index int
		want           float64
	}{
		{0, 0, 0}, {5, 5, 100}, {5, 5, 100}, {6, 5, 100}, {9, 5, 100},
		{10, 10, 101}, {15, 14, 101}, {20, 19, 102.8}, {25, 19, 102.8},
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

func TestEMAUsesTimeframe(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewChartService(repository.NewRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil))))
	start := time.Date(2026, 1, 5, 9, 15, 0, 0, marketLocation)
	rows := sqlmock.NewRows([]string{"start_time", "timeframe", "open", "high", "low", "close", "volume"})
	for minute := 0; minute < 30; minute++ {
		close := 10000 + minute*100
		rows.AddRow(start.Add(time.Duration(minute)*time.Minute), "1m", close, close, close, close, 0)
	}
	mock.ExpectQuery("SELECT c.start_time").WithArgs("NIFTY").WillReturnRows(rows)
	for _, tt := range []struct {
		timeframe models.Timeframe
		minutes   int
		want      float64
	}{
		{models.Timeframe15m, 14, 0},
		{models.Timeframe15m, 15, 114},
		{models.Timeframe15m, 16, 114},
		{models.Timeframe15m, 30, 129},
		{models.Timeframe5m, 15, 114},
		{models.Timeframe5m, 20, 119},
		{models.Timeframe1m, 16, 115},
	} {
		value, err := s.EMA(context.Background(), "NIFTY", 1, tt.timeframe, start.Add(time.Duration(tt.minutes)*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if tt.want == 0 {
			if value != nil {
				t.Fatal("unfinished timeframe exposed")
			}
		} else if value == nil || *value != tt.want {
			t.Fatalf("%s at minute %d: %v; want %v", tt.timeframe, tt.minutes, value, tt.want)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEMAIncompleteSeedAndSessionBoundaries(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewChartService(repository.NewRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil))))
	day := time.Date(2026, 1, 5, 0, 0, 0, 0, time.FixedZone("IST", 19800))
	rows := sqlmock.NewRows([]string{"start_time", "timeframe", "open", "high", "low", "close", "volume"})
	for _, window := range []struct {
		start time.Time
		count int
		close int
	}{
		{day.Add(9*time.Hour + 16*time.Minute), 4, 99900}, // Missing 09:15: no seed.
		{day.Add(15*time.Hour + 25*time.Minute), 5, 10000},
		{day.Add(15*time.Hour + 30*time.Minute), 5, 99900}, // Outside the session.
		{day.AddDate(0, 0, 1).Add(9*time.Hour + 15*time.Minute), 5, 11000},
	} {
		for minute := 0; minute < window.count; minute++ {
			// Database timestamps may be returned in UTC.
			rows.AddRow(window.start.Add(time.Duration(minute)*time.Minute).UTC(), "1m", window.close, window.close, window.close, window.close, 0)
		}
	}
	mock.ExpectQuery("SELECT c.start_time").WithArgs("NIFTY").WillReturnRows(rows)
	for _, tt := range []struct {
		at   time.Time
		want float64
	}{
		{day.Add(9*time.Hour + 20*time.Minute), 0},
		{day.Add(15*time.Hour + 29*time.Minute), 0},
		{day.Add(15*time.Hour + 30*time.Minute), 100},
		{day.AddDate(0, 0, 1).Add(9*time.Hour + 19*time.Minute), 100},
		{day.AddDate(0, 0, 1).Add(9*time.Hour + 20*time.Minute), 102},
	} {
		value, err := s.EMA(context.Background(), "NIFTY", 9, models.Timeframe5m, tt.at)
		if err != nil {
			t.Fatal(err)
		}
		if tt.want == 0 {
			if value != nil {
				t.Fatalf("incomplete seed at %s: %v", tt.at, *value)
			}
		} else if value == nil || math.Abs(*value-tt.want) > 1e-9 {
			t.Fatalf("at %s: %v; want %v", tt.at, value, tt.want)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
