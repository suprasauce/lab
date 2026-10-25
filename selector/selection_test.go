package selector

import (
	"strings"
	"testing"

	"optionsbacktest/models"
)

func TestOffsetSelectorUsesCandleOpen(t *testing.T) {
	var s models.StrikeSelector = NewOffsetSelector(2)
	strike, err := s.SelectStrike(models.Candle{Open: 22530 * 100, High: 22600 * 100, Low: 22400 * 100, Close: 22500 * 100})
	if err != nil || strike != 22650*100 {
		t.Fatalf("strike = %d, %v; want 2265000", strike, err)
	}
	if _, err := s.SelectStrike(models.Candle{Open: -1}); err == nil {
		t.Fatal("expected invalid price error")
	}
}

func TestUnimplementedSelectors(t *testing.T) {
	for _, s := range []models.StrikeSelector{PremiumSelector{}, DeltaSelector{}} {
		if _, err := s.SelectStrike(models.Candle{}); err == nil || !strings.Contains(err.Error(), "not implemented") {
			t.Fatalf("expected not implemented, got %v", err)
		}
	}
}
