package source

import "testing"

// An operator who has recorded no quotes yet must not break the pipeline.
func TestCSVHeaderOnlyIsNotAnError(t *testing.T) {
	c := NewCSV("parallel", "memory")
	rates, err := c.Parse([]byte("date,currency,buying,selling\n"), fetchedAt)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(rates) != 0 {
		t.Errorf("got %d rates from a header-only file, want 0", len(rates))
	}
}

func TestCSVCommentsAreSkipped(t *testing.T) {
	body := "# where this came from\ndate,currency,buying,selling\n2026-08-14,USD,178.50,180.00\n"
	rates, err := NewCSV("parallel", "memory").Parse([]byte(body), fetchedAt)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(rates) != 1 || rates[0].Currency != "USD" || rates[0].Selling != 180.00 {
		t.Errorf("unexpected parse result: %+v", rates)
	}
}

func TestCSVRejectsMissingHeader(t *testing.T) {
	if _, err := NewCSV("parallel", "memory").Parse([]byte(""), fetchedAt); err == nil {
		t.Fatal("expected an error for a file with no header")
	}
}
