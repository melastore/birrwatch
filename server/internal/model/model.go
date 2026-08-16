// Package model holds the core value types shared by the scraper, store and API.
package model

import "time"

// Source identifiers. These are stored in the database, so treat them as stable.
const (
	SourceNBE      = "nbe"
	SourceParallel = "parallel"
	// SourceMarket is a commercial FX aggregate, kept distinct from NBE's
	// official indicative rate so the two are never conflated.
	SourceMarket = "market"
)

// Rate is one currency quote from one source on one day.
//
// Date is the day the rate is *effective*, which is not necessarily the day it
// was fetched: NBE publishes on business days and the value stands over the
// weekend. Keying on Date rather than fetch time is what makes re-scraping
// idempotent.
type Rate struct {
	Source   string    `json:"source"`
	Currency string    `json:"currency"` // ISO 4217, e.g. "USD"
	Date     string    `json:"date"`     // YYYY-MM-DD
	Buying   float64   `json:"buying"`
	Selling  float64   `json:"selling"`
	Updated  time.Time `json:"updated,omitempty"`
}

// Mid is the midpoint of the buy/sell spread. When a source publishes a single
// figure (the parallel market usually does) both legs carry it and Mid is that
// figure.
func (r Rate) Mid() float64 { return (r.Buying + r.Selling) / 2 }

// Snapshot is the raw bytes of a single fetch, stored verbatim.
//
// Keeping the original payload means a parser bug is recoverable: fix the
// parser, replay the snapshots, and the history is corrected without needing
// the upstream site to still be serving that day's page.
type Snapshot struct {
	ID        int64     `json:"id"`
	Source    string    `json:"source"`
	FetchedAt time.Time `json:"fetched_at"`
	SHA256    string    `json:"sha256"`
	Body      []byte    `json:"-"`
}

// SpreadPoint is one day of official-vs-parallel comparison for a currency.
type SpreadPoint struct {
	Date       string  `json:"date"`
	Official   float64 `json:"official"`
	Parallel   float64 `json:"parallel"`
	SpreadBirr float64 `json:"spread_birr"`
	SpreadPct  float64 `json:"spread_pct"`
}
