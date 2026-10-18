package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"optionsbacktest/models"
)

type recordingSelector struct {
	input models.SelectionInput
	err   error
}

func (s *recordingSelector) Select(ctx context.Context, input models.SelectionInput) (models.Option, error) {
	s.input = input
	return models.Option{ID: 10, StrikePrice: 22500 * 100, OptionType: input.OptionType}, s.err
}

func TestPendingLegEntryTarget(t *testing.T) {
	tests := []struct {
		name, expiry, want string
		dte, hour, minute  int
	}{
		{"calendar days include weekends", "2024-10-31", "2024-09-16T09:20:00+05:30", 45, 9, 20},
		{"expiry day", "2024-10-31", "2024-10-31T15:00:00+05:30", 0, 15, 0},
		{"leap day", "2024-03-01", "2024-02-29T09:20:00+05:30", 1, 9, 20},
		{"year boundary", "2025-01-01", "2024-12-31T00:00:00+05:30", 1, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expiry, err := time.Parse(time.DateOnly, tt.expiry)
			if err != nil {
				t.Fatal(err)
			}
			selector := &recordingSelector{}
			execution := models.LegExecution{
				Leg: &models.OptionLeg{EntryDTE: tt.dte, EntryHour: tt.hour, EntryMinute: tt.minute,
					OptionType: models.Call, InstrumentSelector: selector},
				State: models.LegState{Status: models.LegPending},
			}
			if err := new(Engine).backtestOptionLeg(context.Background(), execution.Leg.(*models.OptionLeg), &execution.State, expiry, 1); err != nil {
				t.Fatal(err)
			}
			if got := execution.State.EntryTargetTime.Format(time.RFC3339); got != tt.want {
				t.Fatalf("target = %s; want %s", got, tt.want)
			}
			if selector.input.UnderlyingID != 1 || !selector.input.Expiry.Equal(expiry) ||
				!selector.input.EntryTime.Equal(execution.State.EntryTargetTime) || selector.input.Timeframe != models.Timeframe1m || selector.input.OptionType != models.Call {
				t.Fatalf("unexpected selection input: %+v", selector.input)
			}
			if execution.State.Status != models.LegPending || execution.State.SelectedContract == nil || execution.State.SelectedContract.ID != 10 {
				t.Fatalf("unexpected runtime state: %+v", execution.State)
			}
		})
	}
}

func TestPendingLegSelectorError(t *testing.T) {
	failure := errors.New("selection failed")
	leg := &models.OptionLeg{
		EntryDTE: 1, EntryHour: 9, EntryMinute: 20,
		OptionType: models.Put, InstrumentSelector: &recordingSelector{err: failure},
	}
	state := models.LegState{Status: models.LegPending}
	err := new(Engine).backtestOptionLeg(context.Background(), leg, &state, time.Date(2024, 10, 31, 0, 0, 0, 0, time.UTC), 1)
	if !errors.Is(err, failure) {
		t.Fatalf("error lost selection cause: %v", err)
	}
	if state.SelectedContract != nil || state.Status != models.LegPending {
		t.Fatal("failure changed execution state")
	}
}

func TestOptionsExpiryRejectsOtherLegTypes(t *testing.T) {
	expiry := time.Date(2024, 10, 31, 0, 0, 0, 0, time.UTC)
	var nilOption *models.OptionLeg
	for _, leg := range []models.Leg{&models.FutureLeg{}, &models.EquityLeg{}, nil, nilOption} {
		strategy := &models.Strategy{Legs: []models.Leg{leg}}
		err := new(Engine).backtestExpiry(context.Background(), strategy, expiry)
		if err == nil || !strings.Contains(err.Error(), "2024-10-31") || !strings.Contains(err.Error(), "leg 0") {
			t.Fatalf("expected dated leg error for %T, got %v", leg, err)
		}
	}
}

func TestOptionsExpiryPropagatesLegFailure(t *testing.T) {
	failure := errors.New("selection failed")
	strategy := &models.Strategy{UnderlyingID: 1, Legs: []models.Leg{
		&models.OptionLeg{EntryHour: 9, EntryMinute: 20, OptionType: models.Call, InstrumentSelector: &recordingSelector{err: failure}},
		&models.EquityLeg{}, // Must not execute after the first leg fails.
	}}
	err := new(Engine).backtestExpiry(context.Background(), strategy, time.Date(2024, 10, 31, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "leg 0") {
		t.Fatalf("expected first leg selection failure, got %v", err)
	}
}
