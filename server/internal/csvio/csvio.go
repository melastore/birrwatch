// Package csvio moves rates between the database and a flat CSV file.
//
// The CSV is what gets committed to git. A SQLite file would work as a store of
// record but every scrape would land in the history as an opaque binary diff;
// as text, each day's commit shows exactly which numbers moved. The database is
// a derived artifact, rebuildable from the CSV at any time.
package csvio

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/melastore/birrwatch/internal/model"
)

var header = []string{"source", "currency", "date", "buying", "selling"}

// Export writes rates as CSV in a deterministic order, so re-exporting
// unchanged data produces a byte-identical file and no empty commit.
func Export(w io.Writer, rates []model.Rate) error {
	sorted := make([]model.Rate, len(rates))
	copy(sorted, rates)
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.Date != b.Date {
			return a.Date < b.Date
		}
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		return a.Currency < b.Currency
	})

	cw := csv.NewWriter(w)
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, r := range sorted {
		rec := []string{
			r.Source,
			r.Currency,
			r.Date,
			strconv.FormatFloat(r.Buying, 'f', 4, 64),
			strconv.FormatFloat(r.Selling, 'f', 4, 64),
		}
		if err := cw.Write(rec); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// Import reads rates previously written by Export.
func Import(r io.Reader) ([]model.Rate, error) {
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true

	records, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read csv: %w", err)
	}
	if len(records) == 0 {
		return nil, nil
	}

	idx := map[string]int{}
	for i, h := range records[0] {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	for _, want := range header {
		if _, ok := idx[want]; !ok {
			return nil, fmt.Errorf("missing %q column", want)
		}
	}

	out := make([]model.Rate, 0, len(records)-1)
	for n, row := range records[1:] {
		line := n + 2
		get := func(key string) string {
			i := idx[key]
			if i >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[i])
		}

		buy, err := strconv.ParseFloat(get("buying"), 64)
		if err != nil {
			return nil, fmt.Errorf("line %d: bad buying: %w", line, err)
		}
		sell, err := strconv.ParseFloat(get("selling"), 64)
		if err != nil {
			return nil, fmt.Errorf("line %d: bad selling: %w", line, err)
		}

		out = append(out, model.Rate{
			Source:   get("source"),
			Currency: get("currency"),
			Date:     get("date"),
			Buying:   buy,
			Selling:  sell,
		})
	}
	return out, nil
}
