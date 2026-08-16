package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/melastore/birrwatch/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func rate(source, currency, date string, buy, sell float64) model.Rate {
	return model.Rate{Source: source, Currency: currency, Date: date, Buying: buy, Selling: sell}
}

// Re-running a scrape must not duplicate or drift. This is what lets the cron
// job retry safely.
func TestUpsertRatesIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	in := []model.Rate{
		rate(model.SourceNBE, "USD", "2026-08-14", 155.42, 158.53),
		rate(model.SourceNBE, "EUR", "2026-08-14", 168.90, 172.28),
	}
	for i := 0; i < 3; i++ {
		if _, err := s.UpsertRates(ctx, in); err != nil {
			t.Fatalf("UpsertRates run %d: %v", i, err)
		}
	}

	got, err := s.Rates(ctx, RatesQuery{})
	if err != nil {
		t.Fatalf("Rates: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows after 3 identical upserts, want 2", len(got))
	}
}

func TestUpsertRatesOverwritesSameKey(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if _, err := s.UpsertRates(ctx, []model.Rate{rate(model.SourceNBE, "USD", "2026-08-14", 155.42, 158.53)}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	// A corrected re-parse of the same day should replace, not append.
	if _, err := s.UpsertRates(ctx, []model.Rate{rate(model.SourceNBE, "USD", "2026-08-14", 156.00, 159.00)}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := s.Rates(ctx, RatesQuery{Currency: "USD"})
	if err != nil {
		t.Fatalf("Rates: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	if got[0].Selling != 159.00 {
		t.Errorf("selling = %v, want 159.00 (the corrected value)", got[0].Selling)
	}
}

func TestSnapshotDeduplication(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	body := []byte("<html>same weekend page</html>")

	isNew, err := s.SaveSnapshot(ctx, model.SourceNBE, time.Now(), body)
	if err != nil || !isNew {
		t.Fatalf("first save: isNew=%v err=%v, want true/nil", isNew, err)
	}
	isNew, err = s.SaveSnapshot(ctx, model.SourceNBE, time.Now().Add(time.Hour), body)
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if isNew {
		t.Error("identical payload reported as new; dedup index is not working")
	}

	snaps, err := s.Snapshots(ctx, model.SourceNBE)
	if err != nil {
		t.Fatalf("Snapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("got %d snapshots, want 1", len(snaps))
	}
	if string(snaps[0].Body) != string(body) {
		t.Error("archived body does not round-trip")
	}
}

func TestRatesFiltering(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	in := []model.Rate{
		rate(model.SourceNBE, "USD", "2026-08-12", 154.00, 157.00),
		rate(model.SourceNBE, "USD", "2026-08-13", 155.00, 158.00),
		rate(model.SourceNBE, "USD", "2026-08-14", 156.00, 159.00),
		rate(model.SourceNBE, "EUR", "2026-08-14", 168.00, 172.00),
		rate(model.SourceParallel, "USD", "2026-08-14", 172.00, 172.00),
	}
	if _, err := s.UpsertRates(ctx, in); err != nil {
		t.Fatalf("UpsertRates: %v", err)
	}

	tests := []struct {
		name string
		q    RatesQuery
		want int
	}{
		{"all", RatesQuery{}, 5},
		{"by currency", RatesQuery{Currency: "USD"}, 4},
		{"by source", RatesQuery{Source: model.SourceParallel}, 1},
		{"date window", RatesQuery{Currency: "USD", From: "2026-08-13", To: "2026-08-14"}, 3},
		{"limit", RatesQuery{Limit: 2}, 2},
		{"no match", RatesQuery{Currency: "JPY"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.Rates(ctx, tt.q)
			if err != nil {
				t.Fatalf("Rates: %v", err)
			}
			if len(got) != tt.want {
				t.Errorf("got %d rows, want %d", len(got), tt.want)
			}
		})
	}
}

func TestRatesAreNewestFirst(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if _, err := s.UpsertRates(ctx, []model.Rate{
		rate(model.SourceNBE, "USD", "2026-08-12", 154, 157),
		rate(model.SourceNBE, "USD", "2026-08-14", 156, 159),
		rate(model.SourceNBE, "USD", "2026-08-13", 155, 158),
	}); err != nil {
		t.Fatalf("UpsertRates: %v", err)
	}

	got, err := s.Rates(ctx, RatesQuery{Currency: "USD"})
	if err != nil {
		t.Fatalf("Rates: %v", err)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Date < got[i].Date {
			t.Fatalf("not descending by date: %v", got)
		}
	}
}

func TestSpreadJoinsBothSources(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if _, err := s.UpsertRates(ctx, []model.Rate{
		rate(model.SourceNBE, "USD", "2026-08-13", 155.00, 158.00),
		rate(model.SourceNBE, "USD", "2026-08-14", 156.00, 160.00),
		// Official-only day: no parallel quote, so it must not appear.
		rate(model.SourceNBE, "USD", "2026-08-15", 156.50, 160.50),
		rate(model.SourceParallel, "USD", "2026-08-13", 170.00, 170.00),
		rate(model.SourceParallel, "USD", "2026-08-14", 180.00, 180.00),
	}); err != nil {
		t.Fatalf("UpsertRates: %v", err)
	}

	points, err := s.Spread(ctx, "USD", "", "")
	if err != nil {
		t.Fatalf("Spread: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("got %d points, want 2 (the day without a parallel quote must be dropped): %+v", len(points), points)
	}
	if points[0].Date != "2026-08-13" {
		t.Errorf("points are not ascending by date: %+v", points)
	}

	// 180 against an official 160 is a 20 birr, 12.5% premium.
	p := points[1]
	if p.SpreadBirr != 20 {
		t.Errorf("SpreadBirr = %v, want 20", p.SpreadBirr)
	}
	if p.SpreadPct != 12.5 {
		t.Errorf("SpreadPct = %v, want 12.5", p.SpreadPct)
	}
}

func TestCurrencies(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if _, err := s.UpsertRates(ctx, []model.Rate{
		rate(model.SourceNBE, "USD", "2026-08-14", 156, 159),
		rate(model.SourceNBE, "EUR", "2026-08-14", 168, 172),
		rate(model.SourceParallel, "USD", "2026-08-14", 172, 172),
	}); err != nil {
		t.Fatalf("UpsertRates: %v", err)
	}

	got, err := s.Currencies(ctx)
	if err != nil {
		t.Fatalf("Currencies: %v", err)
	}
	if len(got) != 2 || got[0] != "EUR" || got[1] != "USD" {
		t.Errorf("Currencies() = %v, want [EUR USD]", got)
	}
}
