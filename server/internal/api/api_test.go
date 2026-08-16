package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/melastore/birrwatch/internal/model"
	"github.com/melastore/birrwatch/internal/store"
)

func newTestServer(t *testing.T) http.Handler {
	t.Helper()

	db, err := store.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	seed := []model.Rate{
		{Source: model.SourceNBE, Currency: "USD", Date: "2026-08-13", Buying: 155, Selling: 158},
		{Source: model.SourceNBE, Currency: "USD", Date: "2026-08-14", Buying: 156, Selling: 160},
		{Source: model.SourceNBE, Currency: "EUR", Date: "2026-08-14", Buying: 168, Selling: 172},
		{Source: model.SourceParallel, Currency: "USD", Date: "2026-08-13", Buying: 170, Selling: 170},
		{Source: model.SourceParallel, Currency: "USD", Date: "2026-08-14", Buying: 180, Selling: 180},
	}
	if _, err := db.UpsertRates(context.Background(), seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	return New(db, slog.New(slog.NewTextHandler(io.Discard, nil)), "").Routes()
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestHealthz(t *testing.T) {
	rec := get(t, newTestServer(t), "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestRatesEndpoint(t *testing.T) {
	h := newTestServer(t)

	rec := get(t, h, "/api/rates?currency=USD&source=nbe")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}

	var rates []model.Rate
	if err := json.Unmarshal(rec.Body.Bytes(), &rates); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(rates) != 2 {
		t.Fatalf("got %d rates, want 2", len(rates))
	}
	for _, r := range rates {
		if r.Currency != "USD" || r.Source != model.SourceNBE {
			t.Errorf("filter leaked: %+v", r)
		}
	}
}

// An empty result must serialise as [] and not null, or the dashboard has to
// special-case it on every fetch.
func TestEmptyResultIsEmptyArray(t *testing.T) {
	rec := get(t, newTestServer(t), "/api/rates?currency=JPY")
	if got := rec.Body.String(); got != "[]\n" {
		t.Errorf("body = %q, want %q", got, "[]\n")
	}
}

func TestSpreadEndpoint(t *testing.T) {
	rec := get(t, newTestServer(t), "/api/spread?currency=USD")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}

	var points []model.SpreadPoint
	if err := json.Unmarshal(rec.Body.Bytes(), &points); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("got %d points, want 2", len(points))
	}
	if points[1].SpreadPct != 12.5 {
		t.Errorf("SpreadPct = %v, want 12.5", points[1].SpreadPct)
	}
}

func TestSpreadDefaultsToUSD(t *testing.T) {
	rec := get(t, newTestServer(t), "/api/spread")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var points []model.SpreadPoint
	if err := json.Unmarshal(rec.Body.Bytes(), &points); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(points) != 2 {
		t.Errorf("got %d points, want the USD series", len(points))
	}
}

func TestCurrenciesEndpoint(t *testing.T) {
	rec := get(t, newTestServer(t), "/api/currencies")
	var codes []string
	if err := json.Unmarshal(rec.Body.Bytes(), &codes); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(codes) != 2 || codes[0] != "EUR" {
		t.Errorf("codes = %v, want [EUR USD]", codes)
	}
}

func TestBadInputIsRejected(t *testing.T) {
	h := newTestServer(t)
	for _, path := range []string{
		"/api/rates?currency=DOLLAR",
		"/api/rates?currency=US",
		"/api/rates?from=14-08-2026",
		"/api/rates?to=nonsense",
		"/api/rates?limit=0",
		"/api/rates?limit=99999",
		"/api/rates?limit=abc",
		"/api/spread?currency=1234",
	} {
		if rec := get(t, h, path); rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s: status = %d, want 400", path, rec.Code)
		}
	}
}

func TestCORSHeaders(t *testing.T) {
	rec := get(t, newTestServer(t), "/api/currencies")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", got)
	}
}
