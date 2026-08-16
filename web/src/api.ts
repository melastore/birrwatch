export interface Rate {
  source: string;
  currency: string;
  date: string;
  buying: number;
  selling: number;
  updated?: string;
}

export interface SpreadPoint {
  date: string;
  official: number;
  parallel: number;
  spread_birr: number;
  spread_pct: number;
}

export interface Query {
  currency?: string;
  source?: string;
  from?: string;
  to?: string;
}

/**
 * Two ways to get the same data.
 *
 * "api" talks to birrd. "static" reads the committed CSV directly and does the
 * filtering in the browser, which is what lets the dashboard deploy to GitHub
 * Pages with no backend at all. The dataset is a few thousand rows — small
 * enough that shipping all of it costs less than a round trip would.
 */
const MODE = import.meta.env["VITE_DATA_MODE"] === "static" ? "static" : "api";
const API_BASE = import.meta.env["VITE_API_BASE"] ?? "";
const CSV_URL = `${import.meta.env.BASE_URL}rates.csv`;

export const dataMode = MODE;

/* ---------------------------------------------------------------- api mode */

async function getJSON<T>(path: string, params: Query, signal?: AbortSignal): Promise<T> {
  const query = new URLSearchParams(
    Object.entries(params).filter(([, v]) => v !== undefined && v !== ""),
  ).toString();

  const res = await fetch(`${API_BASE}${path}${query ? `?${query}` : ""}`, { signal });
  if (!res.ok) {
    let detail = res.statusText;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) detail = body.error;
    } catch {
      /* keep statusText */
    }
    throw new Error(`${path}: ${detail}`);
  }
  return (await res.json()) as T;
}

/* ------------------------------------------------------------- static mode */

let cache: Promise<Rate[]> | null = null;

function loadCSV(signal?: AbortSignal): Promise<Rate[]> {
  // Fetched once per page load; every filter runs against the same rows.
  // GitHub Pages serves this with max-age=600 at a URL that never changes, so a
  // browser will happily show yesterday's rates on a page whose whole point is
  // today's. "no-cache" still uses the cache, but revalidates against the ETag
  // first — one cheap 304 instead of a stale dashboard.
  cache ??= fetch(CSV_URL, { signal, cache: "no-cache" })
    .then((res) => {
      if (!res.ok) throw new Error(`${CSV_URL}: ${res.status} ${res.statusText}`);
      return res.text();
    })
    .then(parseCSV)
    .catch((err: unknown) => {
      cache = null; // let a later attempt retry rather than replay the failure
      throw err;
    });
  return cache;
}

export function parseCSV(text: string): Rate[] {
  const lines = text.trim().split(/\r?\n/);
  if (lines.length < 2) return [];

  const header = (lines[0] ?? "").split(",").map((h) => h.trim().toLowerCase());
  const col = (name: string) => header.indexOf(name);
  const [iSource, iCurrency, iDate, iBuying, iSelling] = [
    col("source"),
    col("currency"),
    col("date"),
    col("buying"),
    col("selling"),
  ];
  if ([iSource, iCurrency, iDate, iBuying, iSelling].some((i) => i < 0)) {
    throw new Error("rates.csv is missing a required column");
  }

  const out: Rate[] = [];
  for (const line of lines.slice(1)) {
    if (!line.trim()) continue;
    const f = line.split(",");
    const buying = Number(f[iBuying]);
    const selling = Number(f[iSelling]);
    if (!Number.isFinite(buying) || !Number.isFinite(selling)) continue;
    out.push({
      source: (f[iSource] ?? "").trim(),
      currency: (f[iCurrency] ?? "").trim(),
      date: (f[iDate] ?? "").trim(),
      buying,
      selling,
    });
  }
  return out;
}

function applyFilters(rows: Rate[], q: Query): Rate[] {
  return rows.filter(
    (r) =>
      (!q.currency || r.currency === q.currency) &&
      (!q.source || r.source === q.source) &&
      (!q.from || r.date >= q.from) &&
      (!q.to || r.date <= q.to),
  );
}

/**
 * Mirrors store.Spread: an inner join on date, so a day is only comparable when
 * both sources reported. The Go implementation is the reference and is the one
 * with tests; this exists because a static deploy has no server to ask.
 */
function computeSpread(rows: Rate[], q: Query & { currency: string }): SpreadPoint[] {
  const official = new Map<string, number>();
  const parallel = new Map<string, number>();
  for (const r of applyFilters(rows, { ...q, source: undefined })) {
    (r.source === "parallel" ? parallel : official).set(r.date, r.selling);
  }

  const points: SpreadPoint[] = [];
  for (const [date, off] of official) {
    const par = parallel.get(date);
    if (par === undefined) continue;
    const spread_birr = par - off;
    points.push({
      date,
      official: off,
      parallel: par,
      spread_birr,
      spread_pct: off > 0 ? (spread_birr / off) * 100 : 0,
    });
  }
  return points.sort((a, b) => a.date.localeCompare(b.date));
}

/* ---------------------------------------------------------------- dispatch */

export async function fetchCurrencies(signal?: AbortSignal): Promise<string[]> {
  if (MODE === "api") return getJSON<string[]>("/api/currencies", {}, signal);
  const rows = await loadCSV(signal);
  return [...new Set(rows.map((r) => r.currency))].sort();
}

export async function fetchRates(q: Query, signal?: AbortSignal): Promise<Rate[]> {
  if (MODE === "api") return getJSON<Rate[]>("/api/rates", q, signal);
  const rows = await loadCSV(signal);
  return applyFilters(rows, q).sort((a, b) => b.date.localeCompare(a.date));
}

export async function fetchSpread(
  q: Query & { currency: string },
  signal?: AbortSignal,
): Promise<SpreadPoint[]> {
  if (MODE === "api") return getJSON<SpreadPoint[]>("/api/spread", q, signal);
  return computeSpread(await loadCSV(signal), q);
}
