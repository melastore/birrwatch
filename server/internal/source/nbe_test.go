package source

import (
	"errors"
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

var fetchedAt = time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)

func TestParseGoldenFixture(t *testing.T) {
	rates, err := NewNBE().Parse(fixture(t, "nbe_sample.html"), fetchedAt)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got, want := len(rates), 10; got != want {
		t.Fatalf("got %d rates, want %d: %+v", got, want, rates)
	}

	// The footnote row must not have become a rate.
	byCode := map[string]float64{}
	for _, r := range rates {
		byCode[r.Currency] = r.Selling
		if r.Source != "nbe" {
			t.Errorf("%s: source = %q, want nbe", r.Currency, r.Source)
		}
		if r.Date != "2026-08-14" {
			t.Errorf("%s: date = %q, want 2026-08-14", r.Currency, r.Date)
		}
		if r.Buying <= 0 || r.Selling < r.Buying {
			t.Errorf("%s: implausible buy/sell %v/%v", r.Currency, r.Buying, r.Selling)
		}
	}

	for code, want := range map[string]float64{
		"USD": 158.5297,
		"EUR": 172.2825,
		"GBP": 201.6164,
		"JPY": 1.0722,
		"KES": 1.2274,
	} {
		if got := byCode[code]; got != want {
			t.Errorf("%s selling = %v, want %v", code, got, want)
		}
	}
}

// The decoy table appears first in the document. A parser that grabbed the
// first table, or the largest one, would fail here.
func TestParseSkipsDecoyTable(t *testing.T) {
	rates, err := NewNBE().Parse(fixture(t, "nbe_sample.html"), fetchedAt)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, r := range rates {
		if r.Currency == "" {
			t.Fatalf("decoy row leaked into results: %+v", r)
		}
	}
}

func TestParseReportsMissingTable(t *testing.T) {
	html := `<html><body><h1>Site under maintenance</h1>
	         <table><tr><td>Home</td></tr><tr><td>About</td></tr></table></body></html>`
	_, err := NewNBE().Parse([]byte(html), fetchedAt)
	if !errors.Is(err, ErrNoRateTable) {
		t.Fatalf("err = %v, want ErrNoRateTable", err)
	}
}

// A layout change that renames wrappers but keeps the column headers should
// still parse. This is the whole point of scoring headers instead of pinning
// CSS selectors.
func TestParseSurvivesMarkupChange(t *testing.T) {
	html := `<html><body>
	  <div class="brand-new-wrapper-2027">
	    <table>
	      <tr><th>Curr.</th><th>Transactional Buying</th><th>Transactional Selling</th></tr>
	      <tr><td>US Dollar (USD)</td><td>155.4213</td><td>158.5297</td></tr>
	      <tr><td>Euro</td><td>168.9044</td><td>172.2825</td></tr>
	    </table>
	  </div></body></html>`

	rates, err := NewNBE().Parse([]byte(html), fetchedAt)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(rates) != 2 {
		t.Fatalf("got %d rates, want 2: %+v", len(rates), rates)
	}
	if rates[0].Currency != "USD" || rates[0].Buying != 155.4213 {
		t.Errorf("first rate = %+v", rates[0])
	}
}

func TestParseFallsBackToFetchDate(t *testing.T) {
	html := `<html><body><table>
	  <tr><th>Currency</th><th>Buying</th><th>Selling</th></tr>
	  <tr><td>USD</td><td>155.00</td><td>158.00</td></tr>
	  </table></body></html>`

	rates, err := NewNBE().Parse([]byte(html), fetchedAt)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if rates[0].Date != "2026-08-14" {
		t.Errorf("date = %q, want the fetch date 2026-08-14", rates[0].Date)
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
		{"US DOLLAR", "USD", true},
		{"  US   Dollar  ", "USD", true},
		{"US Dollar (USD)", "USD", true},
		{"Pound Sterling", "GBP", true},
		{"EURO", "EUR", true},
		{"Chinese Yuan Renminbi", "CNY", true},
		{"Rates are indicative and subject to change.", "", false},
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

func TestParseAmount(t *testing.T) {
	tests := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"155.4213", 155.4213, true},
		{"1,234.56", 1234.56, true},
		{" 158.53 ", 158.53, true},
		{"ETB 158.53", 158.53, true},
		{"", 0, false},
		{"-", 0, false},
		{"N/A", 0, false},
		{"0", 0, false},
		{"12.34.56", 0, false},
	}
	for _, tt := range tests {
		got, ok := parseAmount(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("parseAmount(%q) = (%v, %v), want (%v, %v)", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}
