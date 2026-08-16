// Package store persists snapshots and rates in SQLite.
//
// SQLite is chosen over a server database because the whole dataset is a few
// thousand rows a year: one file, no daemon, and the scheduled scraper can
// commit it straight back to the repository.
package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver, keeps CGO_ENABLED=0 builds working

	"github.com/melastore/birrwatch/internal/model"
)

// Store is a handle to the rates database.
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS snapshots (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    source      TEXT NOT NULL,
    fetched_at  TEXT NOT NULL,
    sha256      TEXT NOT NULL,
    body        BLOB NOT NULL
);

-- Identical payloads are stored once. NBE serves the same page all weekend, so
-- without this the archive triples in size for no added information.
CREATE UNIQUE INDEX IF NOT EXISTS snapshots_source_sha ON snapshots(source, sha256);

CREATE TABLE IF NOT EXISTS rates (
    source     TEXT NOT NULL,
    currency   TEXT NOT NULL,
    date       TEXT NOT NULL,
    buying     REAL NOT NULL,
    selling    REAL NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (source, currency, date)
);

CREATE INDEX IF NOT EXISTS rates_currency_date ON rates(currency, date);
`

// Open opens (and migrates) the database at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	// The scraper writes while the API reads; WAL keeps them from blocking.
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("%s: %w", pragma, err)
		}
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// SaveSnapshot archives a raw payload and reports whether it was new.
func (s *Store) SaveSnapshot(ctx context.Context, sourceName string, fetchedAt time.Time, body []byte) (isNew bool, err error) {
	sum := sha256.Sum256(body)
	digest := hex.EncodeToString(sum[:])

	res, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO snapshots (source, fetched_at, sha256, body) VALUES (?, ?, ?, ?)`,
		sourceName, fetchedAt.UTC().Format(time.RFC3339), digest, body)
	if err != nil {
		return false, fmt.Errorf("save snapshot: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("save snapshot: %w", err)
	}
	return n > 0, nil
}

// Snapshots returns archived payloads for a source, oldest first. Passing an
// empty source returns every snapshot.
func (s *Store) Snapshots(ctx context.Context, sourceName string) ([]model.Snapshot, error) {
	query := `SELECT id, source, fetched_at, sha256, body FROM snapshots`
	var args []any
	if sourceName != "" {
		query += ` WHERE source = ?`
		args = append(args, sourceName)
	}
	query += ` ORDER BY fetched_at, id`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	defer rows.Close()

	var out []model.Snapshot
	for rows.Next() {
		var snap model.Snapshot
		var fetchedAt string
		if err := rows.Scan(&snap.ID, &snap.Source, &fetchedAt, &snap.SHA256, &snap.Body); err != nil {
			return nil, fmt.Errorf("scan snapshot: %w", err)
		}
		snap.FetchedAt, _ = time.Parse(time.RFC3339, fetchedAt)
		out = append(out, snap)
	}
	return out, rows.Err()
}

// UpsertRates writes rates idempotently, keyed on (source, currency, date).
// Re-running the scraper on the same day is therefore free of side effects,
// which is what makes the cron schedule safe to retry.
func (s *Store) UpsertRates(ctx context.Context, rates []model.Rate) (int, error) {
	if len(rates) == 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
        INSERT INTO rates (source, currency, date, buying, selling, updated_at)
        VALUES (?, ?, ?, ?, ?, ?)
        ON CONFLICT(source, currency, date) DO UPDATE SET
            buying     = excluded.buying,
            selling    = excluded.selling,
            updated_at = excluded.updated_at`)
	if err != nil {
		return 0, fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, r := range rates {
		if _, err := stmt.ExecContext(ctx, r.Source, r.Currency, r.Date, r.Buying, r.Selling, now); err != nil {
			return 0, fmt.Errorf("upsert %s/%s/%s: %w", r.Source, r.Currency, r.Date, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return len(rates), nil
}

// RatesQuery filters a rate lookup. Zero values mean "no filter".
type RatesQuery struct {
	Source   string
	Currency string
	From     string // inclusive, YYYY-MM-DD
	To       string // inclusive, YYYY-MM-DD
	Limit    int
}

// Rates returns matching rates, newest first.
func (s *Store) Rates(ctx context.Context, q RatesQuery) ([]model.Rate, error) {
	query := `SELECT source, currency, date, buying, selling, updated_at FROM rates WHERE 1=1`
	var args []any
	if q.Source != "" {
		query += ` AND source = ?`
		args = append(args, q.Source)
	}
	if q.Currency != "" {
		query += ` AND currency = ?`
		args = append(args, q.Currency)
	}
	if q.From != "" {
		query += ` AND date >= ?`
		args = append(args, q.From)
	}
	if q.To != "" {
		query += ` AND date <= ?`
		args = append(args, q.To)
	}
	query += ` ORDER BY date DESC, source, currency`
	if q.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, q.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query rates: %w", err)
	}
	defer rows.Close()

	out := []model.Rate{}
	for rows.Next() {
		var r model.Rate
		var updated string
		if err := rows.Scan(&r.Source, &r.Currency, &r.Date, &r.Buying, &r.Selling, &updated); err != nil {
			return nil, fmt.Errorf("scan rate: %w", err)
		}
		r.Updated, _ = time.Parse(time.RFC3339, updated)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Currencies lists every currency code present, alphabetically.
func (s *Store) Currencies(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT currency FROM rates ORDER BY currency`)
	if err != nil {
		return nil, fmt.Errorf("query currencies: %w", err)
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, fmt.Errorf("scan currency: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Spread joins the official and parallel series for a currency.
//
// The join is an inner join on date: a day is only comparable if both sources
// reported. Carrying the last known parallel rate forward would invent data on
// days nobody quoted, and the resulting chart would look more certain than the
// underlying information actually is.
func (s *Store) Spread(ctx context.Context, currency, from, to string) ([]model.SpreadPoint, error) {
	query := `
        SELECT o.date, o.selling, p.selling
        FROM rates o
        JOIN rates p ON p.date = o.date AND p.currency = o.currency AND p.source = ?
        WHERE o.source = ? AND o.currency = ?`
	args := []any{model.SourceParallel, model.SourceNBE, currency}
	if from != "" {
		query += ` AND o.date >= ?`
		args = append(args, from)
	}
	if to != "" {
		query += ` AND o.date <= ?`
		args = append(args, to)
	}
	query += ` ORDER BY o.date`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query spread: %w", err)
	}
	defer rows.Close()

	out := []model.SpreadPoint{}
	for rows.Next() {
		var p model.SpreadPoint
		if err := rows.Scan(&p.Date, &p.Official, &p.Parallel); err != nil {
			return nil, fmt.Errorf("scan spread: %w", err)
		}
		p.SpreadBirr = p.Parallel - p.Official
		if p.Official > 0 {
			p.SpreadPct = p.SpreadBirr / p.Official * 100
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
