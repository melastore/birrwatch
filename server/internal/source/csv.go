package source

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/melastore/birrwatch/internal/model"
)

// CSV reads rates from a local file instead of the network.
//
// The parallel market has no authoritative publisher — no institution posts it,
// and the aggregator sites that do are unstable and disagree with each other.
// Rather than pretend otherwise, this source reads a file that a human curates
// and commits. It implements the same interface as the scraper, so swapping in
// a real scraped source later is a registry change and nothing else.
//
// Expected columns (header required): date,currency,buying,selling
// A single-figure quote may omit selling.
type CSV struct {
	source string
	path   string
}

// NewCSV returns a CSV-backed source recorded under the given source name.
func NewCSV(sourceName, path string) *CSV {
	return &CSV{source: sourceName, path: path}
}

// Name implements Source.
func (c *CSV) Name() string { return c.source }

// Fetch implements Source by reading the file. Returning the bytes rather than
// parsed rows means the file is archived as a snapshot exactly like an HTTP
// payload, and gets the same replay behaviour.
func (c *CSV) Fetch(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(c.path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", c.path, err)
	}
	return b, nil
}

// Parse implements Source.
func (c *CSV) Parse(raw []byte, _ time.Time) ([]model.Rate, error) {
	r := csv.NewReader(bytes.NewReader(raw))
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = -1
	// The file is hand-maintained, so allow comments recording where a quote
	// came from. A rate with no provenance is not worth much.
	r.Comment = '#'

	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse csv %s: %w", c.path, err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("parse csv %s: need a header and at least one row", c.path)
	}

	idx := map[string]int{}
	for i, h := range records[0] {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	for _, required := range []string{"date", "currency", "buying"} {
		if _, ok := idx[required]; !ok {
			return nil, fmt.Errorf("parse csv %s: missing %q column", c.path, required)
		}
	}

	get := func(row []string, key string) string {
		i, ok := idx[key]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}

	var rates []model.Rate
	for n, row := range records[1:] {
		line := n + 2 // 1-indexed, plus the header
		if len(row) == 0 || (len(row) == 1 && row[0] == "") {
			continue
		}

		date, err := time.Parse("2006-01-02", get(row, "date"))
		if err != nil {
			return nil, fmt.Errorf("parse csv %s line %d: bad date: %w", c.path, line, err)
		}

		code, ok := normalizeCurrency(get(row, "currency"))
		if !ok {
			return nil, fmt.Errorf("parse csv %s line %d: unrecognised currency %q", c.path, line, get(row, "currency"))
		}

		buy, err := strconv.ParseFloat(get(row, "buying"), 64)
		if err != nil || buy <= 0 {
			return nil, fmt.Errorf("parse csv %s line %d: bad buying value %q", c.path, line, get(row, "buying"))
		}

		sell := buy
		if s := get(row, "selling"); s != "" {
			sell, err = strconv.ParseFloat(s, 64)
			if err != nil || sell <= 0 {
				return nil, fmt.Errorf("parse csv %s line %d: bad selling value %q", c.path, line, s)
			}
		}

		rates = append(rates, model.Rate{
			Source:   c.source,
			Currency: code,
			Date:     date.Format("2006-01-02"),
			Buying:   buy,
			Selling:  sell,
		})
	}
	return rates, nil
}
