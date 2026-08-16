package source

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

var fetchedAt = time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)

func TestParseAPIFixture(t *testing.T) {
	rates, err := NewNBE().Parse(fixture(t, "nbe_api.json"), fetchedAt)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Eight rows in, five usable: one has no currency, one is zero-valued, and
	// one repeats USD.
	if got, want := len(rates), 5; got != want {
		t.Fatalf("got %d rates, want %d: %+v", got, want, rates)
	}

	byCode := map[string]float64{}
	for _, r := range rates {
		byCode[r.Currency] = r.Selling
		if r.Source != "nbe" {
			t.Errorf("%s: source = %q, want nbe", r.Currency, r.Source)
		}
		// NBE publishes one weighted average, so both legs must match.
		if r.Buying != r.Selling {
			t.Errorf("%s: buying %v != selling %v", r.Currency, r.Buying, r.Selling)
		}
		if r.Date != "2026-08-14" {
			t.Errorf("%s: date = %q, want the date in the payload", r.Currency, r.Date)
		}
	}

	for code, want := range map[string]float64{
		"USD": 155.4213, // the first USD row wins, not the 999 duplicate
		"EUR": 168.9044,
		"GBP": 197.6631, // arrived as a JSON number, not a string
		"JPY": 1.0512,
		"KES": 1.2033,
	} {
		if got := byCode[code]; got != want {
			t.Errorf("%s = %v, want %v", code, got, want)
		}
	}

	if _, present := byCode["CHF"]; present {
		t.Error("a zero weighted_average was accepted; it should be dropped")
	}
}

// A weekend or holiday returns an empty list. That must be an error, not a
// silent success, or a scrape that quietly records nothing looks healthy.
func TestParseEmptyDayIsAnError(t *testing.T) {
	_, err := NewNBE().Parse([]byte(`{"data":[]}`), fetchedAt)
	if !errors.Is(err, ErrNoRates) {
		t.Fatalf("err = %v, want ErrNoRates", err)
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := NewNBE().Parse([]byte("<html>not json</html>"), fetchedAt); err == nil {
		t.Fatal("expected an error for a non-JSON body")
	}
}

// When the payload carries no date, the fetch date stands in rather than the
// row being dropped.
func TestParseFallsBackToFetchDate(t *testing.T) {
	body := `{"data":[{"date":"","currency":{"name":"US Dollar","code":"USD"},"weighted_average":"155.00"}]}`
	rates, err := NewNBE().Parse([]byte(body), fetchedAt)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if rates[0].Date != "2026-08-16" {
		t.Errorf("date = %q, want the fetch date 2026-08-16", rates[0].Date)
	}
}

func TestParseResolvesCurrencyByNameWhenCodeMissing(t *testing.T) {
	body := `{"data":[{"date":"2026-08-14","currency":{"name":"Pound Sterling","code":""},"weighted_average":"197.66"}]}`
	rates, err := NewNBE().Parse([]byte(body), fetchedAt)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if rates[0].Currency != "GBP" {
		t.Errorf("currency = %q, want GBP", rates[0].Currency)
	}
}

func TestFetchURLCarriesTheDate(t *testing.T) {
	n := NewNBE()
	n.Date = time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)

	// Point at a local server that records the request URI, so the date
	// parameter can be asserted without touching the network.
	var requested string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = r.URL.RequestURI()
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	n.URL = srv.URL + "/api/filter-exchange-rates"
	n.Client = srv.Client()

	if _, err := n.Fetch(context.Background()); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if want := "/api/filter-exchange-rates?date=2026-03-09"; requested != want {
		t.Errorf("requested %q, want %q", requested, want)
	}
}

func TestNormalizeCurrency(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"USD", "USD", true},
		{"usd", "USD", true},
		{"US Dollar", "USD", true},
		{"  US   Dollar  ", "USD", true},
		{"US Dollar (USD)", "USD", true},
		{"Pound Sterling", "GBP", true},
		{"EURO", "EUR", true},
		{"Chinese Yuan Renminbi", "CNY", true},
		{"", "", false},
		{"—", "", false},
	}
	for _, tt := range tests {
		got, ok := normalizeCurrency(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("normalizeCurrency(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}
