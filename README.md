# Birrwatch

Tracks exchange rates for the Ethiopian birr and charts the gap between the official
rate and the street rate.

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

No database to set up, no config file. For frontend work, `make serve` in one
shell and `make dev` in another.

## Sources

Three, each recorded under its own name so they can never be conflated:

| Source | What it is | Status |
|---|---|---|
| `nbe` | National Bank of Ethiopia indicative rate — the official figure | **Upstream broken.** See below. |
| `market` | Commercial FX aggregate. A reference rate, *not* an official one | working |
| `parallel` | Street quotes, hand-recorded in `data/parallel.csv` | empty until someone records one |

### The NBE source, and why it is currently dark

The published page at `/exchange/` ships an empty `<tbody id="exchangeData2q1">`
and fills it client-side. The script that does so calls an undocumented endpoint:

```
https://api.nbe.gov.et/api/filter-exchange-rates?date=YYYY-MM-DD
```

Reading that JSON directly beats driving a browser to watch it build a table, and
because the date is a parameter, history is reachable rather than only today —
`birrscrape -backfill 120` walks the calendar backwards.

As of 16 Aug 2026 that endpoint returns 404. `api.nbe.gov.et` resolves to the same
address as the main site but is not configured as a separate vhost, so requests
fall through to WordPress and 404. Their certificate covers `nbe.gov.et` only, so
the host fails TLS verification as well. NBE's own rates table is therefore blank
for every visitor, not just for this scraper. The code is written and tested and
resumes automatically when their endpoint returns.

The `market` source exists because of that outage. It is a commercial aggregate
and is labelled as such everywhere it appears — a dashboard with nothing on it
teaches the reader nothing, but a number under the wrong label is worse than no
number at all.

### The parallel market has no scraper

The sites quoting it disagree with each other and revise silently, so
`data/parallel.csv` is a file a human edits, where every number is attributable to
a commit. It implements the same `Source` interface, so a real feed can replace it.

**The file is empty on purpose.** Anything in it is published to a public
dashboard where a reader cannot tell an illustrative number from a real one, so
there is no such thing as harmless sample data here.

## How it works

```
server/cmd/birrscrape    one-shot scraper (what cron runs)
server/cmd/birrd         API + serves the dashboard
server/internal/source   where rates come from
server/internal/store    SQLite
web/                     React + TypeScript
data/rates.csv           the history, committed on every scrape
```

**Fetching and parsing are separate.** Every fetch is archived verbatim before
anything interprets it. So parsers are testable offline against fixtures — CI
never depends on a government website being up — and a parser bug is fixable
after the fact: `birrscrape -reparse` replays the archive through corrected code.
Without that, a bad week of rates is a permanent hole.

**The scraper is a separate binary from the server.** A failing source can't take
the dashboard down, and cron can retry safely because writes are keyed on
`(source, currency, date)` and upserted. Each source is independently
fault-tolerant in CI, and the export step runs with no source at all, so one dead
upstream never discards what the others collected.

**History lives in a CSV, not the .db file.** SQLite is right for querying and
wrong for git — every scrape would be a binary diff. As sorted text, `git log` on
`data/rates.csv` is a readable history of the birr.

## The dashboard

Hand-rolled SVG. React and ReactDOM are the only runtime dependencies.

Rates (birr) and premium (percent) are **two charts, not two y-axes** — arbitrary
axis alignment would imply a correlation that isn't in the data. The x-axis is
ordinal over observation dates, so weekends aren't drawn as gaps; genuine gaps
break the line instead of interpolating, and lone observations draw as dots.
Series colors are bound to the source rather than to row order, so filtering never
repaints the survivors. Colors are CVD-validated, both charts have a table view,
and dark mode is a selected palette rather than an inverted one.

It runs in two modes: against the API, or — for the GitHub Pages deploy — reading
`rates.csv` directly with no backend at all.

## Tests

```bash
make check     # gofmt, vet, go test -race, web build
```

## License

MIT
