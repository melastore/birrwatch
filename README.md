# Birrwatch

Tracks the National Bank of Ethiopia's daily indicative exchange rate against the
parallel market, and charts the gap between them.

Ethiopia floated the birr in 2024. Since then there are two prices for a dollar —
the official one and the one people actually trade at — and the distance between
them is the single most-watched number in the Ethiopian economy. Nobody publishes
that distance as a time series. This does.

<!-- TODO before sharing this repo: record a GIF of the dashboard and put it here.
     It is the first thing anyone sees and it does more work than the whole README.
       make scrape && make serve   # then record http://localhost:8080 -->

A Go scraper, a SQLite store, a JSON API, and a React dashboard with no charting
library.

---

## Quickstart

Needs Go 1.26+ and Node 22+.

```bash
make scrape    # fetch today's rates into birrwatch.db
make serve     # http://localhost:8080
```

That is the whole setup. There is no database to provision and no config file —
`birrwatch.db` is created on first run.

For frontend work, run the API in one shell and Vite in another:

```bash
make serve     # API on :8080
make dev       # dashboard on :5173, proxying /api to :8080
```

## Layout

```
server/           Go module
  cmd/birrd/          API server + static dashboard
  cmd/birrscrape/     one-shot scraper (this is what cron runs)
  internal/source/    where rates come from, and how payloads are parsed
  internal/store/     SQLite persistence
  internal/api/       HTTP handlers
  internal/csvio/     the committed-history format
web/              React + TypeScript dashboard
data/
  rates.csv           the store of record, committed on every scrape
  parallel.csv        hand-maintained parallel-market quotes
```

## API

| Endpoint | Returns |
|---|---|
| `GET /api/rates?currency=USD&source=nbe&from=&to=` | raw rate rows, newest first |
| `GET /api/spread?currency=USD&from=&to=` | official vs parallel, with the gap in birr and percent |
| `GET /api/currencies` | every currency code on record |
| `GET /healthz` | liveness |

---

## Architecture

Four decisions did most of the work here.

### Fetching and parsing are separate, and raw payloads are archived

`Source` is two methods, `Fetch` and `Parse`, never one. Every scrape writes the
original bytes to a `snapshots` table *before* anything tries to interpret them.

This costs a little disk and buys two things. Parsing is testable without a
network — the parser tests run against a checked-in HTML fixture, so CI never
depends on a government website being up. And a parser bug becomes recoverable:
fix the parser, run `birrscrape -reparse`, and the archive is replayed through
the new code. Without the archive, a bug that silently mangles a week of rates
leaves a permanent hole, because you cannot ask a site for last Tuesday's page.

Identical payloads are stored once, keyed by SHA-256. NBE serves the same page
all weekend, so the deduplication index keeps the archive proportional to the
information in it rather than to the number of times cron fired.

### The parser scores tables instead of pinning selectors

NBE's site is WordPress. Wrapper classes, table IDs, and page structure change
without notice, and a scraper built on `.tablepress-9 tbody tr` breaks the first
time someone touches the theme.

So `Parse` extracts *every* table in the document, scores each one on how well
its header row matches the columns a rate table must have — a currency column
plus at least one of buying/selling — and takes the highest scorer. A redesign
that moves the table, renames its wrapper, or adds three decorative tables above
it changes nothing. Renaming "Currency", "Buying" *and* "Selling" simultaneously
would break it, and that is a much rarer event.

When no table scores high enough the scraper fails loudly with `ErrNoRateTable`
rather than writing zero rows and reporting success. A monitoring job that goes
quiet is worse than one that goes red.

Currency cells are resolved through a name table, so `USD`, `US DOLLAR`,
`US Dollar (USD)`, and `us dollar` all land on the same ISO code. Footnote rows
and separators fail to resolve and are skipped.

### The scraper is a separate binary from the server

`birrscrape` is one-shot: it runs, writes, exits. There is no scheduler inside
the API server.

A goroutine ticking away inside `birrd` would mean a panic on unexpected markup
takes the dashboard down with it, and that "re-run yesterday's scrape" requires
a special code path. As a separate process it is schedulable by cron, systemd,
or a GitHub Action with no in-process machinery, retryable by running it again,
and its failures are isolated from serving traffic.

Retrying is safe because writes are idempotent: rates are keyed on
`(source, currency, date)` and upserted. Running the scraper five times in a day
produces exactly the same table as running it once.

### The store of record is a CSV in git, not the database

`data/rates.csv` is committed; `birrwatch.db` is gitignored and derived.

SQLite is the right thing to *query* and the wrong thing to keep in version
control — every scrape would land as an opaque binary diff. As sorted text, each
day's commit shows precisely which numbers moved, `git log` on the file is a
readable history of the birr, and a corrupted database is one `-import` away
from being fixed. Export is deterministic, so a day where nothing changed
produces a byte-identical file and no empty commit.

### One thing deliberately *not* built

There is no scraper for the parallel market. The aggregator sites that quote it
disagree with each other, revise silently, and cite no methodology; pointing a
scraper at one would produce a confident-looking series with no defensible
provenance. Instead `data/parallel.csv` is a file a human edits, in version
control, where every number is attributable to a commit and can carry a comment
saying where it came from.

It implements the same `Source` interface as the real scraper, so if a
trustworthy publisher ever appears, swapping it in is a change to the registry
and nothing else.

**The parallel figures currently in the repo are placeholders.** They exist so
the dashboard renders on a fresh clone. They are not observed quotes and should
not be cited — replace them before you show this to anyone.

---

## The dashboard

Hand-rolled SVG, no charting dependency. React and ReactDOM are the only runtime
packages.

Two things are worth pointing at:

**Two charts, not two y-axes.** Rates in birr and the premium in percent are
different scales. Putting them on one plot with two axes would let the alignment
of those axes imply a correlation that isn't in the data, so they are two charts
sharing one filter row instead.

**Gaps are drawn as gaps.** The x-axis is ordinal over observation dates rather
than linear in time, because NBE publishes on business days and a time-linear
axis renders every weekend as a hole. Where a series genuinely has no data, the
line breaks instead of interpolating across it, and an isolated observation is
drawn as a dot — sources publish on different calendars, so lone points are
normal rather than an edge case. The premium chart drops any day where only one
side reported; carrying a stale quote forward would invent a number nobody quoted.

Colors are the two lead slots of a CVD-validated categorical palette (adjacent
ΔE 24.7 light / 26.8 dark against an ≥8 threshold). Both charts have a table
view, keyboard-navigable crosshairs, and a selected dark mode.

## Tests

```bash
make check     # gofmt, vet, go test -race, and the web build — what CI runs
```

The parser tests are the interesting ones. Beyond the golden fixture they assert
that a decoy table placed *before* the real one is not selected, that a fabricated
2027-style redesign still parses, and that a missing table is reported as an error
rather than as zero rows.

## Deployment

`CGO_ENABLED=0` builds a static binary — the SQLite driver is pure Go
(`modernc.org/sqlite`), so there is no libc to match on the target image.
`birrd` reads `PORT` if set, and serves the built dashboard when given `-web`.

The included GitHub Action scrapes on weekday mornings, exports the history, and
commits it only when a rate actually moved.

## License

MIT
