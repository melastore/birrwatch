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

const OFFICIAL = "var(--series-official)";
const PARALLEL = "var(--series-parallel)";

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

  const { dates, series, latestOfficial, latestParallel, latestSpread } = useMemo(() => {
    const official = new Map<string, number>();
    const parallel = new Map<string, number>();
    for (const r of rates) {
      // The selling leg is what someone buying foreign currency actually pays.
      (r.source === "parallel" ? parallel : official).set(r.date, r.selling);
    }

    const dates = [...new Set([...official.keys(), ...parallel.keys()])].sort();
    const series: Series[] = [
      { key: "official", name: "Official (NBE)", color: OFFICIAL, values: official },
      { key: "parallel", name: "Parallel market", color: PARALLEL, values: parallel },
    ];

    const lastOf = (m: Map<string, number>) => {
      for (let i = dates.length - 1; i >= 0; i--) {
        const d = dates[i];
        if (d !== undefined) {
          const v = m.get(d);
          if (v !== undefined) return { date: d, value: v };
        }
      }
      return undefined;
    };

    return {
      dates,
      series,
      latestOfficial: lastOf(official),
      latestParallel: lastOf(parallel),
      latestSpread: spread.length > 0 ? spread[spread.length - 1] : undefined,
    };
  }, [rates, spread]);

  const spreadSeries: Series[] = useMemo(
    () => [
      {
        key: "premium",
        name: "Premium",
        color: PARALLEL,
        values: new Map(spread.map((p) => [p.date, p.spread_pct])),
      },
    ],
    [spread],
  );
  const spreadDates = useMemo(() => spread.map((p) => p.date), [spread]);

  const stale = loading && loaded ? "stale" : "";

  return (
    <div className="app">
      <header className="masthead">
        <h1>Birrwatch</h1>
        <p>
          The National Bank of Ethiopia&rsquo;s indicative rate against the parallel market,
          recorded daily. The gap between them is the number worth watching.
        </p>
      </header>

      {/* One filter row scoping every chart below it. */}
      <div className="filters">
        <div className="field">
          <label htmlFor="currency">Currency</label>
          <select
            id="currency"
            value={currency}
            onChange={(e) => setCurrency(e.target.value)}
          >
            {currencies.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
        </div>

        <div className="field">
          <label id="range-label">Range</label>
          <div className="range-group" role="group" aria-labelledby="range-label">
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

      {!error && loaded && (
        <div className={stale}>
          <div className="tiles">
            <StatTile
              label="Official"
              color={OFFICIAL}
              value={latestOfficial ? birr(latestOfficial.value) : "—"}
              sub={latestOfficial ? `${currency} · ${latestOfficial.date}` : "no data"}
            />
            <StatTile
              label="Parallel"
              color={PARALLEL}
              value={latestParallel ? birr(latestParallel.value) : "—"}
              sub={latestParallel ? `${currency} · ${latestParallel.date}` : "no data"}
            />
            <StatTile
              label="Premium"
              value={latestSpread ? pct(latestSpread.spread_pct) : "—"}
              sub={
                latestSpread
                  ? `${latestSpread.spread_birr.toFixed(2)} birr above official`
                  : "needs both sources on one day"
              }
            />
            <StatTile
              label="Observations"
              value={String(dates.length)}
              sub={dates.length > 0 ? `${dates[0]} → ${dates[dates.length - 1]}` : "—"}
            />
          </div>

          <section className="card">
            <header>
              <h2>{currency} against the birr</h2>
              {/* Two series, so a legend is always present. */}
              <div className="legend">
                <span>
                  <span className="swatch" style={{ background: OFFICIAL }} /> Official (NBE)
                </span>
                <span>
                  <span className="swatch" style={{ background: PARALLEL }} /> Parallel market
                </span>
              </div>
            </header>
            <p className="caption">
              Selling rate, birr per {currency}. Both series share one axis — they measure the
              same thing, so the distance between them is meaningful.
            </p>
            <div className="chart-scroll">
              <TimeSeriesChart
                dates={dates}
                series={series}
                formatValue={birr}
                yLabel={`Birr per ${currency}`}
                height={260}
              />
            </div>
            <TableView
              summary="View as table"
              columns={["Date", "Official", "Parallel"]}
              rows={dates.map((d) => [
                d,
                series[0]?.values.get(d)?.toFixed(4) ?? "—",
                series[1]?.values.get(d)?.toFixed(4) ?? "—",
              ])}
            />
          </section>

          <section className="card">
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
                height={200}
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
        </div>
      )}

      <footer className="foot">
        Official rates scraped from the National Bank of Ethiopia. Parallel-market figures are
        hand-curated in <code>data/parallel.csv</code> and carry no official standing — check
        the file before citing any of them.
      </footer>
    </div>
  );
}
