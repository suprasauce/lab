package models

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"
)

func TestPnLPointMarshalJSON(t *testing.T) {
	for _, test := range []struct {
		paise  Price
		rupees string
	}{
		{0, "0.00"},
		{9876, "98.76"},
		{-12345, "-123.45"},
		{math.MaxInt64, "92233720368547758.07"},
		{math.MinInt64, "-92233720368547758.08"},
	} {
		t.Run(test.rupees, func(t *testing.T) {
			point := PnLPoint{Timestamp: time.Date(2026, 1, 1, 9, 20, 0, 0, time.FixedZone("IST", 19800)), PnL: test.paise}
			data, err := json.Marshal(point)
			if err != nil {
				t.Fatal(err)
			}
			want := fmt.Sprintf(`{"timestamp":"2026-01-01T09:20:00+05:30","pnl":%s}`, test.rupees)
			if string(data) != want || point.PnL != test.paise {
				t.Fatalf("JSON=%s; want=%s; internal P&L=%d", data, want, point.PnL)
			}
		})
	}
}
