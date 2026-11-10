package engine

import (
	"context"
	"testing"
	"time"

	"optionsbacktest/models"
	"optionsbacktest/repository"
	"optionsbacktest/selector"
)

type trackedSelector struct {
	delegate selector.Selector
	calls    int
}

func (s *trackedSelector) Select(ctx context.Context, underlyingID int64, candle *models.Candle, expiry time.Time, repo *repository.Repository) (*models.Option, error) {
	s.calls++
	return s.delegate.Select(ctx, underlyingID, candle, expiry, repo)
}

func TestOrderExecutesSelectorOnlyWhenContractIsUnresolved(t *testing.T) {
	missing := map[time.Time]bool{marketTime(9, 15): true, marketTime(9, 20): true}
	e := executionFixture(t, missing, nil)
	if err := e.advance(marketTime(9, 15)); err != nil {
		t.Fatal(err)
	}
	selection := &trackedSelector{delegate: selector.NewOffsetSelector(models.Call, 2)}
	id, err := e.PlaceOrder(selection, "SELL", 75)
	if err != nil {
		t.Fatal(err)
	}
	if selection.calls != 0 {
		t.Fatal("placement invoked the selector")
	}
	order, _ := e.Order(id)
	if order.Selector != selection {
		t.Fatal("selector was not stored on the order")
	}
	for _, minute := range []int{20, 25, 30} {
		if err := e.advance(marketTime(9, minute)); err != nil {
			t.Fatal(err)
		}
		if selection.calls != 1 {
			t.Fatal("pending order reselected its resolved contract")
		}
		if minute == 20 {
			// Simulate a late backfill. Later advances must not revisit this bar.
			delete(missing, marketTime(9, 15))
		}
		if minute == 25 && order.Status != models.OrderPending {
			t.Fatal("filled using an older backfilled candle")
		}
	}
	if order.Status != models.OrderFilled || len(e.Positions) != 1 || !order.FilledAt.Equal(marketTime(9, 25)) {
		t.Fatal("incorrect delayed fill")
	}
}

func TestOrderFillExcludesCandleStartingAtCurrentTime(t *testing.T) {
	e := executionFixture(t, map[time.Time]bool{marketTime(9, 25): true}, nil)
	if err := e.advance(marketTime(9, 25)); err != nil {
		t.Fatal(err)
	}
	id, err := e.PlaceOrder(selector.NewOffsetSelector(models.Call, 2), "SELL", 75)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.advance(marketTime(9, 30)); err != nil {
		t.Fatal(err)
	}
	order, _ := e.Order(id)
	if order.Status != models.OrderPending || len(e.Positions) != 0 {
		t.Fatal("filled from an older candle or the unfinished 09:30 candle")
	}
	if err := e.advance(marketTime(9, 35)); err != nil {
		t.Fatal(err)
	}
	if order.Status != models.OrderFilled || !order.FilledAt.Equal(marketTime(9, 30)) || len(e.Positions) != 1 {
		t.Fatal("did not fill from the 09:30 candle once completed")
	}
}

func TestIDOrderFillsWithoutUnderlyingCandle(t *testing.T) {
	e := executionFixture(t, nil, map[time.Time]bool{marketTime(9, 15): true})
	if err := e.advance(marketTime(9, 15)); err != nil {
		t.Fatal(err)
	}
	id, err := e.PlaceOrder(selector.NewIDSelector(101), "BUY", 75)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.advance(marketTime(9, 20)); err != nil {
		t.Fatal(err)
	}
	order, _ := e.Order(id)
	if e.UnderlyingCandle != nil || order.Status != models.OrderFilled || !order.FilledAt.Equal(marketTime(9, 15)) {
		t.Fatal("ID selector changed fill timing or required underlying data")
	}
}

func TestPlaceOrderRejectsMissingSelector(t *testing.T) {
	e := executionFixture(t, nil, nil)
	if _, err := e.PlaceOrder(nil, "BUY", 75); err == nil || len(e.Orders) != 0 {
		t.Fatal("missing selector was accepted")
	}
}
