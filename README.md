<h1 align="center">Birrwatch</h1>

<p align="center">
  Daily exchange rates for the Ethiopian birr — official, market, and street —
  and the gap between them.
</p>

<p align="center">
  <a href="https://melastore.github.io/birrwatch/"><strong>Live dashboard →</strong></a>
</p>

<p align="center">
  <a href="https://github.com/melastore/birrwatch/actions/workflows/ci.yml">
    <img src="https://github.com/melastore/birrwatch/actions/workflows/ci.yml/badge.svg" alt="CI">
  </a>
  <a href="https://github.com/melastore/birrwatch/actions/workflows/scrape.yml">
    <img src="https://github.com/melastore/birrwatch/actions/workflows/scrape.yml/badge.svg" alt="Scrape">
  </a>
  <img src="https://img.shields.io/badge/go-1.26-00ADD8" alt="Go 1.26">
  <img src="https://img.shields.io/badge/license-MIT-black" alt="MIT">
</p>

<!-- TODO: record a GIF of the dashboard and drop it here. It will do more work
     than every paragraph below it. -->

---

Ethiopia floated the birr in 2024. Since then there are two prices for a dollar —
the official one and the one people actually trade at — and the distance between
them is among the most watched numbers in the Ethiopian economy. Nobody publishes
that distance as a time series.

This collects it daily, keeps the history in git, and charts it.

**Go** scraper · **SQLite** store · **JSON API** · **React** dashboard with no charting library.

## Quickstart

Needs Go 1.26+ and Node 22+.

```bash
make scrape    # fetch today's rates
make serve     # http://localhost:8080
```

No database to provision, no config file, no API keys. For frontend work run
`make serve` in one shell and `make dev` in another.

```bash
make check     # gofmt, vet, go test -race, web build — what CI runs
```

## What it records

Ten currencies — USD, EUR, GBP, AED, SAR, CNY, DJF, KES, JPY, CHF — chosen for
relevance to Ethiopia rather than for coverage: the Gulf remittance corridors,
Djibouti (through which effectively all seaborne imports pass), and the majors
used for import pricing.

Three sources, each recorded under its own name so they can never be conflated:

| Source | What it is | Status |
|---|---|---|
| `nbe` | National Bank of Ethiopia indicative rate — the official figure | dark, see below |
| `market` | Commercial FX aggregate. A reference rate, **not** an official one | working |
| `parallel` | Street quotes, hand-recorded in `data/parallel.csv` | empty by design |

## The NBE source

Their published page ships an empty `<tbody id="exchangeData2q1">` and fills it
client-side. Reading the minified bundle turns up the call behind it:

```
GET https://api.nbe.gov.et/api/filter-exchange-rates?date=YYYY-MM-DD
```

Reading that JSON directly beats driving a headless browser to watch it build a
table — and because the date is a parameter, history is reachable rather than
only today. `birrscrape -backfill 120` walks the calendar backwards.

**As of 16 Aug 2026 that endpoint returns 404.** `api.nbe.gov.et` resolves to the
same address as the main site but is not configured as a separate vhost, so
requests fall through to WordPress. Their certificate covers `nbe.gov.et` only,
so the host fails TLS verification too. NBE's own rates table is blank for every
visitor, not just for this scraper. The code is written, tested, and resumes on
its own when their endpoint returns.

The `market` source exists because of that outage. A dashboard with nothing on it
teaches the reader nothing — but a number under the wrong label is worse than no
number, so it is labelled as an aggregate everywhere it appears.

### Why the parallel market has no scraper

The sites quoting it disagree with each other and revise silently, so
`data/parallel.csv` is a file a human edits, where every number is attributable
to a commit. It implements the same `Source` interface, so a real feed can
replace it without touching anything else.

The file is **empty on purpose**. Anything in it is published to a public page
where a reader cannot tell an illustrative number from a real one, so there is no
such thing as harmless sample data here.

## Design notes

**Fetching and parsing are separate, and raw payloads are archived.** Every fetch
is stored verbatim before anything interprets it. Parsers are therefore testable
offline against fixtures — CI never depends on a government website being up —
and a parser bug is fixable after the fact: `birrscrape -reparse` replays the
archive through corrected code. Without the archive, a bad week is a permanent
hole, because you cannot ask a server for last Tuesday's response.

**The scraper is a separate binary from the server.** A failing source can't take
the dashboard down, and cron can retry safely because writes are keyed on
`(source, currency, date)` and upserted. Each source is independently
fault-tolerant in CI, and the export step runs with no source at all — so one
dead upstream never discards what the others collected.

**History lives in a CSV, not the `.db` file.** SQLite is right for querying and
wrong for version control: every scrape would land as an opaque binary diff. As
sorted text, `git log data/rates.csv` is a readable history of the birr, and
export is deterministic, so a day where nothing moved produces no commit at all.

**The dashboard is hand-rolled SVG.** React and ReactDOM are the only runtime
dependencies. Rates (birr) and premium (percent) are two charts rather than two
y-axes — arbitrary axis alignment would imply a correlation that isn't in the
data. The x-axis is ordinal over observation dates, so weekends aren't drawn as
gaps; genuine gaps break the line instead of interpolating, and lone observations
draw as dots. Series colors bind to the source, not to row order, so filtering
never repaints the survivors. Colors are CVD-validated, both charts have a table
view, and dark mode is a selected palette rather than an inverted one.

**It runs with or without a backend.** Against the API, or — for the GitHub Pages
deploy — reading `rates.csv` directly, with no server at all.

## Layout

```
server/cmd/birrscrape    one-shot scraper (what cron runs)
server/cmd/birrd         API + serves the dashboard
server/internal/source   where rates come from
server/internal/store    SQLite persistence
server/internal/csvio    the committed-history format
web/                     React + TypeScript dashboard
data/rates.csv           the history, committed on every scrape
data/parallel.csv        hand-recorded street quotes
```

## API

| Endpoint | Returns |
|---|---|
| `GET /api/rates?currency=USD&source=nbe&from=&to=` | rate rows, newest first |
| `GET /api/spread?currency=USD&from=&to=` | official vs parallel, gap in birr and percent |
| `GET /api/currencies` | every currency on record |
| `GET /healthz` | liveness |

## Automation

Three GitHub Actions workflows:

- **CI** — gofmt, vet, `go test -race`, web build, on every push
- **Scrape** — daily at 07:00 UTC; records rates, exports, and commits only when a rate actually moved
- **Deploy** — publishes the dashboard to Pages, chained off the scrape so new rates reach the site

## License

MIT
