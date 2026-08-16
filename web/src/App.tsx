import { useEffect, useMemo, useState } from "react";
import {
  dataMode,
  fetchCurrencies,
  fetchRates,
  fetchSpread,
  type Rate,
  type SpreadPoint,
} from "./api";
import TimeSeriesChart, { type Series } from "./components/TimeSeriesChart";
import StatTile from "./components/StatTile";
import TableView from "./components/TableView";
import ThemeToggle from "./components/ThemeToggle";
import EmptyState from "./components/EmptyState";

/**
 * Every source the dashboard knows about, in display order.
 *
 * Color is bound to the source, not to its position in the current result, so
 * filtering a series out never repaints the ones that remain. `note` is what
 * keeps the market reference from being mistaken for NBE's official figure.
 */
const SOURCES = [
  {
    key: "nbe",
    name: "Official (NBE)",
    short: "Official",
    color: "var(--series-official)",
    note: "National Bank of Ethiopia indicative rate",
  },
  {
    key: "market",
    name: "Market reference",
    short: "Market",
    color: "var(--series-market)",
    note: "Commercial FX aggregate — not an official rate",
  },
  {
    key: "parallel",
    name: "Parallel market",
    short: "Parallel",
    color: "var(--series-parallel)",
    note: "Hand-recorded quotes",
  },
] as const;

const RANGES = [
  { label: "30d", days: 30 },
  { label: "90d", days: 90 },
  { label: "1y", days: 365 },
  { label: "All", days: 0 },
] as const;

function isoDaysAgo(days: number) {
  const d = new Date();
  d.setUTCDate(d.getUTCDate() - days);
  return d.toISOString().slice(0, 10);
}

const birr = (v: number) => v.toFixed(2);
const pct = (v: number) => `${v.toFixed(1)}%`;

/** Percent change across the visible window, or undefined if not computable. */
function changePct(values: number[]) {
  const first = values[0];
  const last = values[values.length - 1];
  if (first === undefined || last === undefined || values.length < 2 || first === 0) return undefined;
  return ((last - first) / first) * 100;
}

export default function App() {
  const [currency, setCurrency] = useState("USD");
  const [currencies, setCurrencies] = useState<string[]>(["USD"]);
  const [rangeDays, setRangeDays] = useState<number>(90);

  const [rates, setRates] = useState<Rate[]>([]);
  const [spread, setSpread] = useState<SpreadPoint[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    const ac = new AbortController();
    fetchCurrencies(ac.signal)
      .then((c) => c.length > 0 && setCurrencies(c))
      .catch(() => {
        /* the currency picker falls back to USD */
      });
    return () => ac.abort();
  }, []);

  useEffect(() => {
    const ac = new AbortController();
    const from = rangeDays === 0 ? undefined : isoDaysAgo(rangeDays);

    setLoading(true);
    Promise.all([
      fetchRates({ currency, ...(from ? { from } : {}) }, ac.signal),
      fetchSpread({ currency, ...(from ? { from } : {}) }, ac.signal),
    ])
      .then(([r, s]) => {
        setRates(r);
        setSpread(s);
        setError(null);
        setLoaded(true);
      })
      .catch((e: unknown) => {
        if (e instanceof DOMException && e.name === "AbortError") return;
        setError(e instanceof Error ? e.message : String(e));
      })
      .finally(() => {
        if (!ac.signal.aborted) setLoading(false);
      });

    return () => ac.abort();
  }, [currency, rangeDays]);

  const view = useMemo(() => {
    const bySource = new Map<string, Map<string, number>>();
    for (const s of SOURCES) bySource.set(s.key, new Map());
    for (const r of rates) {
      // The selling leg is what someone buying foreign currency pays. Sources
      // publishing a single figure carry it in both legs.
      bySource.get(r.source)?.set(r.date, r.selling);
    }

    const dates = [
      ...new Set(SOURCES.flatMap((s) => [...(bySource.get(s.key)?.keys() ?? [])])),
    ].sort();

    // Only sources that actually reported get a series, a legend entry and a
    // tile. An empty series would advertise data that is not there.
    const present = SOURCES.filter((s) => (bySource.get(s.key)?.size ?? 0) > 0);

    const series: Series[] = present.map((s) => ({
      key: s.key,
      name: s.name,
      color: s.color,
      values: bySource.get(s.key)!,
    }));

    const tiles = present.map((s) => {
      const m = bySource.get(s.key)!;
      const ordered = dates.map((d) => m.get(d)).filter((v): v is number => v !== undefined);
      let latest: { date: string; value: number } | undefined;
      for (let i = dates.length - 1; i >= 0 && !latest; i--) {
        const d = dates[i];
        const v = d === undefined ? undefined : m.get(d);
        if (d !== undefined && v !== undefined) latest = { date: d, value: v };
      }
      return { meta: s, ordered, latest };
    });

    return {
      dates,
      series,
      tiles,
      latestSpread: spread.length > 0 ? spread[spread.length - 1] : undefined,
    };
  }, [rates, spread]);

  const spreadSeries: Series[] = useMemo(
    () => [
      {
        key: "premium",
        name: "Premium",
        color: "var(--series-parallel)",
        values: new Map(spread.map((p) => [p.date, p.spread_pct])),
      },
    ],
    [spread],
  );
  const spreadDates = useMemo(() => spread.map((p) => p.date), [spread]);

  const { dates, series, tiles, latestSpread } = view;
  const stale = loading && loaded ? "stale" : "";
  const hasData = dates.length > 0;
  const newestDate = dates.length > 0 ? dates[dates.length - 1] : undefined;

  return (
    <div className="app">
      <header className="masthead">
        <div className="titles">
          <h1>Birrwatch</h1>
          <p>
            Exchange rates for the Ethiopian birr, recorded daily. Where the official rate and
            the street rate diverge, the gap between them is the number worth watching.
          </p>
        </div>
        <ThemeToggle />
      </header>

      {/* One filter row scoping every chart below it. */}
      <div className="filters">
        <div className="field">
          <label htmlFor="currency">Currency</label>
          <select id="currency" value={currency} onChange={(e) => setCurrency(e.target.value)}>
            {currencies.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
        </div>

        <div className="field">
          <span className="label" id="range-label">
            Range
          </span>
          <div className="segmented" role="group" aria-labelledby="range-label">
            {RANGES.map((r) => (
              <button
                key={r.label}
                type="button"
                aria-pressed={rangeDays === r.days}
                onClick={() => setRangeDays(r.days)}
              >
                {r.label}
              </button>
            ))}
          </div>
        </div>

        <div className="spacer" />

        {hasData && newestDate && (
          <span className="live-dot">
            <i />
            Updated {newestDate}
          </span>
        )}
      </div>

      {error && (
        <div className="state error">
          Could not load rates: {error}
          <div style={{ fontSize: 13, marginTop: 8, color: "var(--text-secondary)" }}>
            {dataMode === "api" ? (
              <>
                Is <code>birrd</code> running?
              </>
            ) : (
              <>
                <code>rates.csv</code> could not be read.
              </>
            )}
          </div>
        </div>
      )}

      {!error && !loaded && loading && <div className="state">Loading&hellip;</div>}

      {!error && loaded && !hasData && (
        <div className="card reveal">
          <EmptyState title="No rates recorded yet">
            Nothing has been collected for this currency and range. The scheduled job records
            rates each weekday; a fresh clone starts empty until it runs.
          </EmptyState>
        </div>
      )}

      {!error && loaded && hasData && (
        <div className={stale}>
          <div className="tiles reveal">
            {tiles.map(({ meta, ordered, latest }) => (
              <StatTile
                key={meta.key}
                label={meta.short}
                color={meta.color}
                value={latest ? birr(latest.value) : "—"}
                sub={latest ? `${currency} · ${latest.date}` : "no data"}
                delta={changePct(ordered)}
                spark={ordered}
              />
            ))}
            <StatTile
              label="Observations"
              value={String(dates.length)}
              sub={dates.length > 0 ? `${dates[0]} → ${dates[dates.length - 1]}` : "—"}
            />
          </div>

          <section className="card reveal">
            <header>
              <h2>{currency} against the birr</h2>
              {/* A legend is always present once there is more than one series. */}
              {series.length > 1 && (
                <div className="legend">
                  {series.map((s) => (
                    <span key={s.key}>
                      <span className="swatch" style={{ background: s.color }} /> {s.name}
                    </span>
                  ))}
                </div>
              )}
            </header>
            <p className="caption">
              Birr per {currency}. Every series shares one axis — they measure the same thing,
              so the distance between them is meaningful.
            </p>
            <div className="chart-scroll">
              <TimeSeriesChart
                dates={dates}
                series={series}
                formatValue={birr}
                yLabel={`Birr per ${currency}`}
                height={280}
              />
            </div>
            <TableView
              summary="View as table"
              columns={["Date", ...series.map((s) => s.name)]}
              rows={dates.map((d) => [d, ...series.map((s) => s.values.get(d)?.toFixed(4) ?? "—")])}
            />
          </section>

          {spread.length > 0 && (
            <section className="card reveal">
              <header>
                <h2>Parallel-market premium</h2>
              </header>
              <p className="caption">
                How far above the official rate the parallel market sits, in percent. Only days
                where both sources reported are plotted — carrying a stale quote forward would
                invent a number nobody actually quoted.
              </p>
              <div className="chart-scroll">
                <TimeSeriesChart
                  dates={spreadDates}
                  series={spreadSeries}
                  formatValue={pct}
                  yLabel="Premium over the official rate, percent"
                  height={220}
                  area
                  baselineAt={0}
                />
              </div>
              <TableView
                summary="View as table"
                columns={["Date", "Official", "Parallel", "Gap (birr)", "Premium"]}
                rows={spread.map((p) => [
                  p.date,
                  p.official.toFixed(4),
                  p.parallel.toFixed(4),
                  p.spread_birr.toFixed(4),
                  pct(p.spread_pct),
                ])}
              />
            </section>
          )}

          {latestSpread === undefined && (
            <section className="card reveal">
              <p className="caption" style={{ margin: 0 }}>
                <strong>No premium shown.</strong> It needs NBE&rsquo;s official rate and a
                parallel-market quote on the same day. NBE&rsquo;s own rates endpoint is
                currently returning 404, and <code>data/parallel.csv</code> is empty by design —
                only observed quotes belong in it.
              </p>
            </section>
          )}
        </div>
      )}

      <footer className="foot">
        <span>
          {SOURCES.map((s) => `${s.name}: ${s.note}`).join(" · ")}
        </span>
        <a href="https://github.com/melastore/birrwatch">Source on GitHub</a>
      </footer>
    </div>
  );
}
