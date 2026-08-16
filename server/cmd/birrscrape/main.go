// Command birrscrape fetches exchange rates, archives the raw payload and
// upserts the parsed values.
//
// It is a separate binary from the API server on purpose. A scrape that panics
// on unexpected markup should not take the dashboard down with it, and a
// one-shot process is trivially schedulable from cron, systemd or GitHub
// Actions without any in-process scheduler to reason about.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/melastore/birrwatch/internal/csvio"
	"github.com/melastore/birrwatch/internal/source"
	"github.com/melastore/birrwatch/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("scrape failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		dbPath      = flag.String("db", "birrwatch.db", "path to the SQLite database")
		sourceName  = flag.String("source", "nbe", "source to scrape")
		parallelCSV = flag.String("parallel-csv", "data/parallel.csv", "path to the curated parallel-market CSV")
		nbeURL      = flag.String("nbe-url", source.DefaultNBEURL, "override the NBE page URL")
		reparse     = flag.Bool("reparse", false, "re-parse archived snapshots instead of fetching")
		importCSV   = flag.String("import", "", "load rates from a CSV into the database, then exit")
		exportCSV   = flag.String("export", "", "write all rates to a CSV after running")
		timeout     = flag.Duration("timeout", 60*time.Second, "overall timeout")
	)
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ctx, cancelTimeout := context.WithTimeout(ctx, *timeout)
	defer cancelTimeout()

	db, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	// Import rebuilds the database from committed history and does nothing
	// else, so a fresh CI runner can restore state before scraping.
	if *importCSV != "" {
		return importRates(ctx, db, *importCSV, log)
	}

	registry := source.New(*parallelCSV)
	if nbe, ok := registry["nbe"].(*source.NBE); ok {
		nbe.URL = *nbeURL
	}

	src, err := registry.Get(*sourceName)
	if err != nil {
		return err
	}

	if *reparse {
		err = reparseSnapshots(ctx, db, src, log)
	} else {
		err = scrapeOnce(ctx, db, src, log)
	}
	if err != nil {
		return err
	}

	if *exportCSV != "" {
		return exportRates(ctx, db, *exportCSV, log)
	}
	return nil
}

func importRates(ctx context.Context, db *store.Store, path string, log *slog.Logger) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Info("no history to import", "path", path)
			return nil
		}
		return err
	}
	defer f.Close()

	rates, err := csvio.Import(f)
	if err != nil {
		return fmt.Errorf("import %s: %w", path, err)
	}
	n, err := db.UpsertRates(ctx, rates)
	if err != nil {
		return err
	}
	log.Info("history imported", "path", path, "rates", n)
	return nil
}

func exportRates(ctx context.Context, db *store.Store, path string, log *slog.Logger) error {
	rates, err := db.Rates(ctx, store.RatesQuery{})
	if err != nil {
		return err
	}

	// Write to a temporary file and rename, so an interrupted run cannot leave
	// a half-written history behind.
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := csvio.Export(f, rates); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}

	log.Info("history exported", "path", path, "rates", len(rates))
	return nil
}

func scrapeOnce(ctx context.Context, db *store.Store, src source.Source, log *slog.Logger) error {
	fetchedAt := time.Now().UTC()

	raw, err := src.Fetch(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", src.Name(), err)
	}
	log.Info("fetched", "source", src.Name(), "bytes", len(raw))

	// Archive before parsing. If the parser fails on today's markup the bytes
	// are still on disk, and a fixed parser can recover the day with -reparse.
	isNew, err := db.SaveSnapshot(ctx, src.Name(), fetchedAt, raw)
	if err != nil {
		return err
	}
	log.Info("snapshot stored", "source", src.Name(), "new", isNew)

	rates, err := src.Parse(raw, fetchedAt)
	if err != nil {
		return fmt.Errorf("%s: %w", src.Name(), err)
	}

	n, err := db.UpsertRates(ctx, rates)
	if err != nil {
		return err
	}
	log.Info("rates upserted", "source", src.Name(), "count", n)
	return nil
}

// reparseSnapshots replays the archive through the current parser. This is the
// payoff for storing raw payloads: a parser fix repairs history retroactively
// instead of leaving a permanent gap in the series.
func reparseSnapshots(ctx context.Context, db *store.Store, src source.Source, log *slog.Logger) error {
	snaps, err := db.Snapshots(ctx, src.Name())
	if err != nil {
		return err
	}
	if len(snaps) == 0 {
		return fmt.Errorf("no archived snapshots for source %q", src.Name())
	}

	var total, failed int
	for _, snap := range snaps {
		rates, err := src.Parse(snap.Body, snap.FetchedAt)
		if err != nil {
			failed++
			log.Warn("reparse failed", "snapshot", snap.ID, "fetched_at", snap.FetchedAt, "err", err)
			continue
		}
		n, err := db.UpsertRates(ctx, rates)
		if err != nil {
			return err
		}
		total += n
	}
	log.Info("reparse complete", "snapshots", len(snaps), "rates", total, "failed", failed)
	if failed == len(snaps) {
		return fmt.Errorf("every snapshot failed to parse")
	}
	return nil
}
