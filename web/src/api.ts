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

const BASE = import.meta.env["VITE_API_BASE"] ?? "";

async function get<T>(path: string, params: Record<string, string> = {}, signal?: AbortSignal): Promise<T> {
  const query = new URLSearchParams(
    Object.entries(params).filter(([, v]) => v !== ""),
  ).toString();

  const res = await fetch(`${BASE}${path}${query ? `?${query}` : ""}`, { signal });
  if (!res.ok) {
    // The API returns {"error": "..."} on failure; fall back to the status text
    // when the body is not the shape we expect.
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

export const fetchCurrencies = (signal?: AbortSignal) =>
  get<string[]>("/api/currencies", {}, signal);

export const fetchRates = (
  params: { currency?: string; source?: string; from?: string; to?: string },
  signal?: AbortSignal,
) =>
  get<Rate[]>(
    "/api/rates",
    {
      currency: params.currency ?? "",
      source: params.source ?? "",
      from: params.from ?? "",
      to: params.to ?? "",
    },
    signal,
  );

export const fetchSpread = (
  params: { currency: string; from?: string; to?: string },
  signal?: AbortSignal,
) =>
  get<SpreadPoint[]>(
    "/api/spread",
    { currency: params.currency, from: params.from ?? "", to: params.to ?? "" },
    signal,
  );
