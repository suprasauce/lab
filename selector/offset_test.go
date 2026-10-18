package selector

import (
	"math"
	"testing"

	"optionsbacktest/models"
)

func TestSelectStrike(t *testing.T) {
	for _, tt := range []struct {
		name   string
		open   models.Price
		offset int
		want   models.Price
	}{
		{"exact ATM", 22500 * 100, 0, 22500 * 100},
		{"below midpoint", 22524*100 + 99, 0, 22500 * 100},
		{"midpoint rounds up", 22525 * 100, 0, 22550 * 100},
		{"positive offset", 22530 * 100, 2, 22650 * 100},
		{"negative offset", 22520 * 100, -2, 22400 * 100},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectStrike(tt.open, tt.offset)
			if err != nil || got != tt.want {
				t.Fatalf("strike = %d, %v; want %d", got, err, tt.want)
			}
		})
	}
}

func TestSelectStrikeRejectsInvalidPrices(t *testing.T) {
	for _, tt := range []struct {
		open   models.Price
		offset int
	}{
		{-1, 0}, {5000, -2}, {models.Price(math.MaxInt64), 1},
	} {
		if _, err := selectStrike(tt.open, tt.offset); err == nil {
			t.Fatalf("expected error for %+v", tt)
		}
	}
}
