// Package source defines where rate data comes from and how raw payloads become
// structured rates.
//
// Fetching and parsing are deliberately separate interfaces. A Source that
// couples them cannot be tested without network access, and cannot re-derive
// history when the parser is fixed. Every Source here fetches bytes, hands them
// to the store, and parses them as an independent step.
package source

import (
	"context"
	"fmt"
	"time"

	"github.com/melastore/birrwatch/internal/model"
)

// Source fetches and parses one upstream publisher of exchange rates.
type Source interface {
	// Name is the stable identifier written to the database.
	Name() string

	// Fetch returns the raw upstream payload. It performs no parsing so that
	// the bytes can be archived before any interpretation happens.
	Fetch(ctx context.Context) ([]byte, error)

	// Parse turns a raw payload into rates. fetchedAt is used only as a
	// fallback when the payload carries no date of its own.
	Parse(raw []byte, fetchedAt time.Time) ([]model.Rate, error)
}

// Registry maps source names to implementations.
type Registry map[string]Source

// New builds the default registry. parallelCSV is the path to the
// manually-maintained parallel-market file; it may be empty.
func New(parallelCSV string) Registry {
	r := Registry{
		model.SourceNBE:    NewNBE(),
		model.SourceMarket: NewMarket(),
	}
	if parallelCSV != "" {
		r[model.SourceParallel] = NewCSV(model.SourceParallel, parallelCSV)
	}
	return r
}

// Get returns the named source or an error listing what is available.
func (r Registry) Get(name string) (Source, error) {
	s, ok := r[name]
	if !ok {
		return nil, fmt.Errorf("unknown source %q (have %v)", name, r.Names())
	}
	return s, nil
}

// Names lists registered source names in a stable order.
func (r Registry) Names() []string {
	out := make([]string, 0, len(r))
	for _, n := range []string{model.SourceNBE, model.SourceMarket, model.SourceParallel} {
		if _, ok := r[n]; ok {
			out = append(out, n)
		}
	}
	return out
}
