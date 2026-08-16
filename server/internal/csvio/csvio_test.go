package csvio

import (
	"bytes"
	"strings"
	"testing"

	"github.com/melastore/birrwatch/internal/model"
)

func TestRoundTrip(t *testing.T) {
	in := []model.Rate{
		{Source: "nbe", Currency: "USD", Date: "2026-08-14", Buying: 155.4213, Selling: 158.5297},
		{Source: "parallel", Currency: "USD", Date: "2026-08-14", Buying: 178.5, Selling: 180},
		{Source: "nbe", Currency: "EUR", Date: "2026-08-13", Buying: 168.9044, Selling: 172.2825},
	}

	var buf bytes.Buffer
	if err := Export(&buf, in); err != nil {
		t.Fatalf("Export: %v", err)
	}

	out, err := Import(&buf)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(out) != len(in) {
		t.Fatalf("got %d rates, want %d", len(out), len(in))
	}

	// Export sorts by date, then source, then currency.
	if out[0].Currency != "EUR" || out[0].Date != "2026-08-13" {
		t.Errorf("first row = %+v, want the 2026-08-13 EUR row", out[0])
	}
	for _, r := range out {
		if r.Buying <= 0 || r.Selling <= 0 {
			t.Errorf("values did not survive the round trip: %+v", r)
		}
	}
}

// A scrape that changes nothing must produce a byte-identical file, or the
// scheduled job commits noise every day.
func TestExportIsDeterministic(t *testing.T) {
	rates := []model.Rate{
		{Source: "parallel", Currency: "USD", Date: "2026-08-14", Buying: 178.5, Selling: 180},
		{Source: "nbe", Currency: "USD", Date: "2026-08-14", Buying: 155.4213, Selling: 158.5297},
		{Source: "nbe", Currency: "EUR", Date: "2026-08-13", Buying: 168.9044, Selling: 172.2825},
	}
	shuffled := []model.Rate{rates[2], rates[0], rates[1]}

	var a, b bytes.Buffer
	if err := Export(&a, rates); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if err := Export(&b, shuffled); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if a.String() != b.String() {
		t.Errorf("export depends on input order:\n%s\nvs\n%s", a.String(), b.String())
	}
}

func TestExportDoesNotMutateInput(t *testing.T) {
	rates := []model.Rate{
		{Source: "parallel", Currency: "USD", Date: "2026-08-14"},
		{Source: "nbe", Currency: "EUR", Date: "2026-08-13"},
	}
	first := rates[0]

	if err := Export(&bytes.Buffer{}, rates); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if rates[0] != first {
		t.Errorf("Export reordered the caller's slice: %+v", rates)
	}
}

func TestImportRejectsBadHeader(t *testing.T) {
	_, err := Import(strings.NewReader("source,currency,date\nnbe,USD,2026-08-14\n"))
	if err == nil {
		t.Fatal("expected an error for a CSV missing the buying/selling columns")
	}
}

func TestImportEmpty(t *testing.T) {
	out, err := Import(strings.NewReader(""))
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("got %d rates from an empty file, want 0", len(out))
	}
}
