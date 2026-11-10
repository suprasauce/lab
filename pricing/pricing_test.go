package pricing

import (
	"math"
	"strings"
	"testing"
	"time"

	"optionsbacktest/models"
)

func TestKnownPriceAndDelta(t *testing.T) {
	// S=K=100, T=1, sigma=20%, r=6%, q=0%.
	for _, test := range []struct {
		kind         models.OptionType
		price, delta float64
	}{
		{models.Call, 10.989549152625983, 0.6554217416103241},
		{models.Put, 5.166002511050856, -0.3445782583896759},
	} {
		t.Run(string(test.kind), func(t *testing.T) {
			price, err := BlackScholesPrice(100, 100, 1, 0.2, test.kind)
			if err != nil || math.Abs(price-test.price) > 1e-10 {
				t.Fatalf("price=%g want=%g err=%v", price, test.price, err)
			}
			delta, err := Delta(100, 100, 1, 0.2, test.kind)
			if err != nil || math.Abs(delta-test.delta) > 1e-12 {
				t.Fatalf("delta=%g want=%g err=%v", delta, test.delta, err)
			}
		})
	}
}

func TestPutCallParity(t *testing.T) {
	call, err := BlackScholesPrice(22500, 23000, 45.0/365, 0.25, models.Call)
	if err != nil {
		t.Fatal(err)
	}
	put, err := BlackScholesPrice(22500, 23000, 45.0/365, 0.25, models.Put)
	if err != nil {
		t.Fatal(err)
	}
	want := 22500 - 23000*math.Exp(-RiskFreeRate*45/365)
	if math.Abs(call-put-want) > 1e-9 {
		t.Fatalf("put-call parity: got=%g want=%g", call-put, want)
	}
}

func TestIVRecoveryAndDeltaFromPremium(t *testing.T) {
	for _, kind := range []models.OptionType{models.Call, models.Put} {
		for _, test := range []struct{ spot, strike, years, volatility float64 }{
			{100, 100, 1, 0.2},
			{22500, 23000, 45.0 / 365, 0.3},
			{22500, 22000, 21.0 / 365, 0.15},
			{100, 100 * math.Exp(RiskFreeRate), 1, 0.00001},
			{100, 100, 1, maxVolatility},
		} {
			price, err := BlackScholesPrice(test.spot, test.strike, test.years, test.volatility, kind)
			if err != nil {
				t.Fatal(err)
			}
			iv, err := ImpliedVolatility(test.spot, test.strike, price, test.years, kind)
			if err != nil || math.Abs(iv-test.volatility) > 1e-8 {
				t.Fatalf("%s IV=%g want=%g err=%v", kind, iv, test.volatility, err)
			}
			want, err := Delta(test.spot, test.strike, test.years, test.volatility, kind)
			if err != nil {
				t.Fatal(err)
			}
			got, err := DeltaFromPremium(test.spot, test.strike, price, test.years, kind)
			if err != nil || math.Abs(got-want) > 1e-8 {
				t.Fatalf("%s delta=%g want=%g err=%v", kind, got, want, err)
			}
		}
	}
}

func TestInvalidInputs(t *testing.T) {
	for _, test := range []struct {
		name                            string
		spot, strike, years, volatility float64
		kind                            models.OptionType
	}{
		{"option type", 100, 100, 1, 0.2, "CE"},
		{"zero spot", 0, 100, 1, 0.2, models.Call},
		{"negative spot", -1, 100, 1, 0.2, models.Put},
		{"zero strike", 100, 0, 1, 0.2, models.Call},
		{"negative strike", 100, -1, 1, 0.2, models.Put},
		{"zero time", 100, 100, 0, 0.2, models.Call},
		{"negative time", 100, 100, -1, 0.2, models.Put},
		{"zero volatility", 100, 100, 1, 0, models.Call},
		{"negative volatility", 100, 100, 1, -0.2, models.Put},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := BlackScholesPrice(test.spot, test.strike, test.years, test.volatility, test.kind); err == nil {
				t.Fatal("price accepted invalid input")
			}
			if _, err := Delta(test.spot, test.strike, test.years, test.volatility, test.kind); err == nil {
				t.Fatal("delta accepted invalid input")
			}
			if test.volatility > 0 {
				if _, err := ImpliedVolatility(test.spot, test.strike, 10, test.years, test.kind); err == nil {
					t.Fatal("IV accepted invalid input")
				}
				if _, err := DeltaFromPremium(test.spot, test.strike, 10, test.years, test.kind); err == nil {
					t.Fatal("delta from premium accepted invalid input")
				}
			}
		})
	}
}

func TestNonFiniteInputs(t *testing.T) {
	for _, invalid := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		for i := range 4 {
			values := []float64{100, 100, 1, 0.2}
			values[i] = invalid
			if _, err := BlackScholesPrice(values[0], values[1], values[2], values[3], models.Call); err == nil {
				t.Fatalf("price accepted nonfinite input %d", i)
			}
			if _, err := Delta(values[0], values[1], values[2], values[3], models.Put); err == nil {
				t.Fatalf("delta accepted nonfinite input %d", i)
			}
			// Reuse slot 3 for premium when testing the inversion entry points.
			if _, err := ImpliedVolatility(values[0], values[1], values[3], values[2], models.Call); err == nil {
				t.Fatalf("IV accepted nonfinite input %d", i)
			}
			if _, err := DeltaFromPremium(values[0], values[1], values[3], values[2], models.Put); err == nil {
				t.Fatalf("premium delta accepted nonfinite input %d", i)
			}
		}
	}
}

func TestPremiumBounds(t *testing.T) {
	for _, kind := range []models.OptionType{models.Call, models.Put} {
		s, k := 120.0, 100.0*math.Exp(-RiskFreeRate)
		lower, upper := math.Max(s-k, 0), s
		if kind == models.Put {
			lower, upper = math.Max(k-s, 0), k
		}
		for _, premium := range []float64{-1, lower - 0.01, lower, upper, upper + 0.01} {
			if _, err := ImpliedVolatility(120, 100, premium, 1, kind); err == nil {
				t.Fatalf("%s accepted premium %g outside (%g,%g)", kind, premium, lower, upper)
			}
		}
	}
	// An ITM put has a positive discounted intrinsic lower bound as well.
	if _, err := ImpliedVolatility(80, 100, 10, 1, models.Put); err == nil {
		t.Fatal("accepted put below its lower bound")
	}
}

func TestUnsupportedVolatilityRange(t *testing.T) {
	for _, kind := range []models.OptionType{models.Call, models.Put} {
		for _, volatility := range []float64{1e-9, 8} {
			strike := 100 * math.Exp(RiskFreeRate)
			premium, err := BlackScholesPrice(100, strike, 1, volatility, kind)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ImpliedVolatility(100, strike, premium, 1, kind); err == nil || !strings.Contains(err.Error(), "supported range") {
				t.Fatalf("%s accepted unsupported IV %g: %v", kind, volatility, err)
			}
		}
	}
}

func TestNumericRangeAndExtremeTails(t *testing.T) {
	if _, err := BlackScholesPrice(100, 100, 1, math.SmallestNonzeroFloat64, models.Call); err == nil {
		t.Fatal("accepted overflowing d1")
	}
	if _, err := ImpliedVolatility(100, 100, 10, math.MaxFloat64, models.Call); err == nil {
		t.Fatal("accepted discount underflow")
	}
	for _, kind := range []models.OptionType{models.Call, models.Put} {
		price, err := BlackScholesPrice(1, 1000000, 0.01, 0.01, kind)
		if err != nil || !finite(price) || price < 0 {
			t.Fatalf("tail price=%g err=%v", price, err)
		}
		delta, err := Delta(1, 1000000, 0.01, 0.01, kind)
		if err != nil || !finite(delta) || delta < -1 || delta > 1 {
			t.Fatalf("tail delta=%g err=%v", delta, err)
		}
	}
}

func TestTimeToExpiry(t *testing.T) {
	location, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}
	expiry := time.Date(2026, 1, 27, 15, 30, 0, 0, location)
	for _, test := range []struct {
		name        string
		observation time.Time
		want        float64
	}{
		{"45 days", expiry.AddDate(0, 0, -45), 45.0 / DaysPerYear},
		{"expiry morning", time.Date(2026, 1, 27, 9, 15, 0, 0, location), 6.25 / (24 * DaysPerYear)},
		{"same instant in UTC", expiry.Add(-time.Hour).UTC(), 1.0 / (24 * DaysPerYear)},
		{"one second", expiry.Add(-time.Second), 1.0 / (86400 * DaysPerYear)},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := TimeToExpiry(test.observation, expiry)
			if err != nil || math.Abs(got-test.want) > 1e-15 {
				t.Fatalf("years=%g want=%g err=%v", got, test.want, err)
			}
		})
	}
	for _, at := range []time.Time{expiry, expiry.Add(time.Minute)} {
		if _, err := TimeToExpiry(at, expiry); err == nil {
			t.Fatal("accepted nonpositive time remaining")
		}
	}
	if _, err := TimeToExpiry(time.Date(1000, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("accepted saturated duration")
	}
}
