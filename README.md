# Birrwatch

Tracks Ethiopia's official exchange rate against the parallel market, and charts the gap.

**[Live dashboard →](https://melastore.github.io/birrwatch/)**

Since the birr was floated in 2024 there are two prices for a dollar: the official
NBE rate and the one people actually trade at. Nobody publishes the distance
between them as a time series. This does.

<!-- TODO: record a GIF of the dashboard and put it here. It matters more than the text. -->

Go scraper · SQLite · JSON API · React dashboard with no charting library.

## Run it

Needs Go 1.26+ and Node 22+.

```bash
make scrape    # fetch today's rates
make serve     # http://localhost:8080
```

No database to set up, no config file.

For frontend work, `make serve` in one shell and `make dev` in another.

## How it works

```
server/cmd/birrscrape    one-shot scraper (what cron runs)
server/cmd/birrd         API + serves the dashboard
server/internal/source   where rates come from
server/internal/store    SQLite
web/                     React + TypeScript
data/rates.csv           the history, committed on every scrape
```

**The parser scores tables instead of pinning selectors.** NBE's site is
WordPress and its markup moves. Rather than target `.tablepress-9 tbody tr`,
the parser reads every table on the page and picks the one whose header row
best matches what a rate table needs. A redesign has to rename "Currency",
"Buying" and "Selling" all at once to break it. If nothing matches, the scrape
fails loudly instead of writing zero rows.

**Raw pages are archived before parsing.** Every fetch is stored verbatim, so
the parser can be tested offline against a fixture, and a parser bug is
fixable after the fact — `birrscrape -reparse` replays the archive through the
corrected code. Without that, a bad week of rates is a permanent hole.

**The scraper is a separate binary from the server.** Bad markup can't take the
dashboard down, and cron can retry safely because writes are keyed on
`(source, currency, date)` and upserted.

**History lives in a CSV, not the .db file.** SQLite is right for querying and
wrong for git — every scrape would be a binary diff. As sorted text, `git log`
on `data/rates.csv` is a readable history of the birr.

**The parallel market has no scraper.** The sites quoting it disagree and revise
silently, so `data/parallel.csv` is a file a human edits, where every number is
attributable to a commit. It implements the same interface, so a real source can
replace it later.

> The parallel figures in the repo right now are placeholders so the dashboard
> renders on a fresh clone. Replace them before citing anything.

## The dashboard

Hand-rolled SVG. React and ReactDOM are the only runtime dependencies.

Rates (birr) and premium (percent) are **two charts, not two y-axes** — arbitrary
axis alignment would imply a correlation that isn't in the data. The x-axis is
ordinal over observation dates, so weekends aren't drawn as gaps; genuine gaps
break the line instead of interpolating, and lone observations draw as dots.
Colors are CVD-validated, both charts have a table view, and dark mode is a
selected palette rather than an inverted one.

It runs in two modes. Against the API, or — for the GitHub Pages deploy — reading
`rates.csv` directly with no backend at all.

## Tests

```bash
make check     # gofmt, vet, go test -race, web build
```

The parser tests assert that a decoy table placed before the real one isn't
picked, that a fabricated redesign still parses, and that a missing table is an
error rather than silence.

## License

MIT
