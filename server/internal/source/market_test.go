package source

import (
	"math"
	"testing"
)

const marketBody = `{
  "result": "success",
  "provider": "https://www.exchangerate-api.com",
  "time_last_update_utc": "Sun, 16 Aug 2026 00:02:31 +0000",
  "base_code": "USD",
  "rates": { "USD": 1, "ETB": 161.629287, "EUR": 0.8648, "GBP": 0.739, "XYZ": 0 }
}`

func closeTo(a, b float64) bool { return math.Abs(a-b) < 0.0001 }

func TestMarketParse(t *testing.T) {
	rates, err := NewMarket().Parse([]byte(marketBody), fetchedAt)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	got := map[string]float64{}
	for _, r := range rates {
		got[r.Currency] = r.Selling
		if r.Source != "market" {
			t.Errorf("%s: source = %q, want market", r.Currency, r.Source)
		}
		if r.Buying != r.Selling {
			t.Errorf("%s: a reference rate must carry one figure, got %v/%v", r.Currency, r.Buying, r.Selling)
		}
		// The payload's own timestamp wins over the fetch date.
		if r.Date != "2026-08-16" {
			t.Errorf("%s: date = %q, want 2026-08-16", r.Currency, r.Date)
		}
	}

	// USD falls out of the same expression as everything else: 161.629 / 1.
	if !closeTo(got["USD"], 161.629287) {
		t.Errorf("USD = %v, want 161.629287", got["USD"])
	}
	// Birr per euro = birr-per-USD / euros-per-USD.
	if want := 161.629287 / 0.8648; !closeTo(got["EUR"], want) {
		t.Errorf("EUR = %v, want %v", got["EUR"], want)
	}
	if want := 161.629287 / 0.739; !closeTo(got["GBP"], want) {
		t.Errorf("GBP = %v, want %v", got["GBP"], want)
	}
	if _, present := got["XYZ"]; present {
		t.Error("a zero rate was accepted; it should be skipped")
	}
}

func TestMarketRejectsNonUSDBase(t *testing.T) {
	body := `{"result":"success","base_code":"EUR","rates":{"ETB":180}}`
	if _, err := NewMarket().Parse([]byte(body), fetchedAt); err == nil {
		t.Fatal("expected an error when the base is not USD")
	}
}

// Without an ETB rate there is nothing to express in birr, so this must fail
// rather than silently record an empty day.
func TestMarketRequiresETB(t *testing.T) {
	body := `{"result":"success","base_code":"USD","rates":{"EUR":0.86}}`
	if _, err := NewMarket().Parse([]byte(body), fetchedAt); err == nil {
		t.Fatal("expected an error when ETB is absent")
	}
}

func TestMarketRejectsFailureResult(t *testing.T) {
	body := `{"result":"error","error-type":"unsupported-code"}`
	if _, err := NewMarket().Parse([]byte(body), fetchedAt); err == nil {
		t.Fatal("expected an error when the provider reports failure")
	}
}
