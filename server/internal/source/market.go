package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/melastore/birrwatch/internal/model"
)

// DefaultMarketAPI returns every rate against a USD base in one request.
const DefaultMarketAPI = "https://open.er-api.com/v6/latest/USD"

// marketCurrencies are the currencies recorded — ten, chosen for relevance to
// Ethiopia rather than for coverage.
//
// All ten are on NBE's own list, so the two series stay directly comparable
// once their endpoint is available again. It is a fixed list rather than
// "everything the provider returns" so the dataset does not silently change
// shape when the upstream adds a currency.
var marketCurrencies = []string{
	"USD", // the rate everything else is judged against
	"EUR",
	"GBP",
	"AED", // UAE — the largest remittance and re-export corridor
	"SAR", // Saudi Arabia — the largest labour-migration corridor
	"CNY", // China — Ethiopia's biggest trade partner
	"DJF", // Djibouti — effectively all seaborne imports transit here
	"KES", // Kenya — the main regional neighbour for trade
	"JPY",
	"CHF",
}

// Market reads reference rates from a public FX aggregator.
//
// This is NOT the National Bank of Ethiopia's indicative rate. It is a
// commercial aggregate of interbank quotes, and it is recorded under its own
// source name so the two can never be confused in the data or on the chart.
// It exists because NBE's own endpoint is intermittently unavailable, and a
// dashboard with nothing on it teaches the reader nothing.
type Market struct {
	URL    string
	Client *http.Client
}

// NewMarket returns a Market source pointed at the default endpoint.
func NewMarket() *Market {
	return &Market{
		URL:    DefaultMarketAPI,
		Client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Name implements Source.
func (m *Market) Name() string { return model.SourceMarket }

// Fetch implements Source.
func (m *Market) Fetch(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "birrwatch/1.0 (+https://github.com/melastore/birrwatch)")

	resp, err := m.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", m.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: status %d", m.URL, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

type marketResponse struct {
	Result     string             `json:"result"`
	Provider   string             `json:"provider"`
	TimeUpdate string             `json:"time_last_update_utc"`
	BaseCode   string             `json:"base_code"`
	Rates      map[string]float64 `json:"rates"`
}

// Parse implements Source.
//
// The payload is quoted against USD, but this project measures everything in
// birr per unit of foreign currency. With a USD base, rates[X] is "X per USD",
// so birr per X is rates[ETB] / rates[X] — and rates[USD] is 1, which makes USD
// fall out of the same expression rather than needing a special case.
func (m *Market) Parse(raw []byte, fetchedAt time.Time) ([]model.Rate, error) {
	var resp marketResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}
	if resp.Result != "" && resp.Result != "success" {
		return nil, fmt.Errorf("provider reported result %q", resp.Result)
	}
	if resp.BaseCode != "" && resp.BaseCode != "USD" {
		return nil, fmt.Errorf("expected a USD base, got %q", resp.BaseCode)
	}

	birrPerUSD, ok := resp.Rates["ETB"]
	if !ok || birrPerUSD <= 0 {
		return nil, fmt.Errorf("%w: no ETB rate in response", ErrNoRates)
	}

	date := fetchedAt.UTC().Format("2006-01-02")
	if t, err := time.Parse(time.RFC1123, resp.TimeUpdate); err == nil {
		date = t.UTC().Format("2006-01-02")
	}

	var rates []model.Rate
	for _, code := range marketCurrencies {
		perUSD, ok := resp.Rates[code]
		if !ok || perUSD <= 0 {
			continue
		}
		birrPerUnit := birrPerUSD / perUSD
		rates = append(rates, model.Rate{
			Source:   model.SourceMarket,
			Currency: code,
			Date:     date,
			// A reference rate is a single figure, not a buy/sell pair.
			Buying:  birrPerUnit,
			Selling: birrPerUnit,
		})
	}

	if len(rates) == 0 {
		return nil, fmt.Errorf("%w: no requested currencies present", ErrNoRates)
	}
	return rates, nil
}
