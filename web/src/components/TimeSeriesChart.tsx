import { useCallback, useId, useLayoutEffect, useMemo, useRef, useState } from "react";

export interface Series {
  key: string;
  name: string;
  color: string;
  /** Sparse by design: a series may be missing days the others have. */
  values: Map<string, number>;
}

interface Props {
  dates: string[];
  series: Series[];
  height?: number;
  area?: boolean;
  formatValue: (v: number) => string;
  /** Drawn as a hairline reference, e.g. zero on a spread chart. */
  baselineAt?: number;
  yLabel: string;
}

const M = { top: 14, right: 18, bottom: 30, left: 54 };

/**
 * A line/area chart drawn as plain SVG.
 *
 * The x scale is ordinal over the union of dates rather than linear in time.
 * Rates are published on business days, so a time-linear axis would render
 * every weekend as a visual gap and imply the rate did something over it. The
 * series is a sequence of observations, and the axis says so.
 */
export default function TimeSeriesChart({
  dates,
  series,
  height = 260,
  area = false,
  formatValue,
  baselineAt,
  yLabel,
}: Props) {
  const wrapRef = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(720);
  const [hover, setHover] = useState<number | null>(null);
  const uid = useId().replace(/:/g, "");

  useLayoutEffect(() => {
    const el = wrapRef.current;
    if (!el) return;

    // Measure synchronously before the browser paints. A ResizeObserver's first
    // callback is async, so relying on it alone means the chart paints once at
    // its default width and then jumps to the real one.
    const measure = (w: number) => setWidth(Math.max(300, Math.round(w)));
    measure(el.getBoundingClientRect().width);

    const ro = new ResizeObserver(([entry]) => {
      if (entry) measure(entry.contentRect.width);
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const plotW = Math.max(1, width - M.left - M.right);
  const plotH = Math.max(1, height - M.top - M.bottom);

  const { yMin, yMax, ticks } = useMemo(() => {
    const all: number[] = [];
    for (const s of series) for (const v of s.values.values()) all.push(v);
    if (baselineAt !== undefined) all.push(baselineAt);
    if (all.length === 0) return { yMin: 0, yMax: 1, ticks: [0, 1] };
    return niceScale(Math.min(...all), Math.max(...all));
  }, [series, baselineAt]);

  const x = useCallback(
    (i: number) => (dates.length <= 1 ? plotW / 2 : (i / (dates.length - 1)) * plotW),
    [dates.length, plotW],
  );
  const y = useCallback(
    (v: number) => plotH - ((v - yMin) / (yMax - yMin || 1)) * plotH,
    [plotH, yMin, yMax],
  );

  // Runs of consecutive present days become separate path segments, so a gap in
  // the data reads as a gap rather than a straight line across missing days.
  const paths = useMemo(
    () =>
      series.map((s) => {
        const runs: Array<Array<[number, number]>> = [];
        let run: Array<[number, number]> = [];
        dates.forEach((d, i) => {
          const v = s.values.get(d);
          if (v === undefined) {
            if (run.length) runs.push(run);
            run = [];
          } else {
            run.push([x(i), y(v)]);
          }
        });
        if (run.length) runs.push(run);

        const draw = (r: Array<[number, number]>) =>
          r.map(([px, py], i) => `${i === 0 ? "M" : "L"}${px.toFixed(2)},${py.toFixed(2)}`).join("");

        const line = runs.filter((r) => r.length > 1).map(draw).join(" ");

        const fill = area
          ? runs
              .filter((r) => r.length > 1)
              .map((r) => {
                const first = r[0]!;
                const last = r[r.length - 1]!;
                return `${draw(r)}L${last[0].toFixed(2)},${plotH}L${first[0].toFixed(2)},${plotH}Z`;
              })
              .join(" ")
          : "";

        // A run of one has no segment to stroke, so a lone observation would
        // render as nothing at all. Sources publish on different calendars, so
        // isolated points are normal, not an edge case.
        const dots = runs.filter((r) => r.length === 1).map((r) => r[0]!);

        return { series: s, line, fill, dots };
      }),
    [series, dates, x, y, area, plotH],
  );

  const nearestIndex = (clientX: number) => {
    const svg = wrapRef.current?.querySelector("svg");
    if (!svg) return null;
    const rect = svg.getBoundingClientRect();
    const px = clientX - rect.left - M.left;
    if (dates.length <= 1) return 0;
    return Math.min(dates.length - 1, Math.max(0, Math.round((px / plotW) * (dates.length - 1))));
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
    e.preventDefault();
    const step = e.key === "ArrowLeft" ? -1 : 1;
    setHover((h) => {
      const next = (h ?? dates.length - 1) + step;
      return Math.min(dates.length - 1, Math.max(0, next));
    });
  };

  const labelEvery = Math.max(1, Math.ceil(dates.length / (width < 480 ? 3 : 6)));
  const hoverDate = hover !== null ? dates[hover] : undefined;

  if (dates.length === 0) {
    return <div className="state">No data in this range.</div>;
  }

  return (
    <div ref={wrapRef} style={{ position: "relative" }}>
      <svg
        width={width}
        height={height}
        role="img"
        aria-label={`${yLabel}. ${series.map((s) => s.name).join(" and ")} from ${dates[0]} to ${dates[dates.length - 1]}. Full values are in the table below.`}
        tabIndex={0}
        onKeyDown={onKeyDown}
        onMouseMove={(e) => setHover(nearestIndex(e.clientX))}
        onMouseLeave={() => setHover(null)}
        onBlur={() => setHover(null)}
        onTouchStart={(e) => {
          const t = e.touches[0];
          if (t) setHover(nearestIndex(t.clientX));
        }}
        onTouchMove={(e) => {
          const t = e.touches[0];
          if (t) setHover(nearestIndex(t.clientX));
        }}
        style={{ display: "block", touchAction: "pan-y", outline: "none" }}
      >
        <defs>
          {series.map((s) => (
            <linearGradient key={s.key} id={`${uid}-${s.key}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={s.color} stopOpacity="0.26" />
              <stop offset="100%" stopColor={s.color} stopOpacity="0.02" />
            </linearGradient>
          ))}
        </defs>

        <g transform={`translate(${M.left},${M.top})`}>
          {/* Recessive hairline grid — solid, one shade off the surface. */}
          {ticks.map((t) => (
            <g key={t}>
              <line x1={0} x2={plotW} y1={y(t)} y2={y(t)} stroke="var(--gridline)" strokeWidth={1} />
              <text
                x={-12}
                y={y(t)}
                textAnchor="end"
                dominantBaseline="middle"
                fontSize={11}
                fill="var(--text-muted)"
                style={{ fontVariantNumeric: "tabular-nums" }}
              >
                {formatValue(t)}
              </text>
            </g>
          ))}

          {baselineAt !== undefined && (
            <line
              x1={0}
              x2={plotW}
              y1={y(baselineAt)}
              y2={y(baselineAt)}
              stroke="var(--baseline)"
              strokeWidth={1}
            />
          )}

          {area &&
            paths.map(({ series: s, fill }) =>
              fill ? (
                <path key={`f-${s.key}`} d={fill} fill={`url(#${uid}-${s.key})`} className="fade-in" />
              ) : null,
            )}

          {paths.map(({ series: s, line }) =>
            line ? (
              <path
                key={s.key}
                className="draw"
                pathLength={1}
                d={line}
                fill="none"
                stroke={s.color}
                strokeWidth={2}
                strokeLinejoin="round"
                strokeLinecap="round"
              />
            ) : null,
          )}

          {paths.map(({ series: s, dots }) =>
            dots.map(([px, py]) => (
              <circle
                key={`${s.key}-${px}`}
                className="fade-in"
                cx={px}
                cy={py}
                r={4}
                fill={s.color}
              />
            )),
          )}

          {hover !== null && (
            <g>
              <line
                x1={x(hover)}
                x2={x(hover)}
                y1={0}
                y2={plotH}
                stroke="var(--baseline)"
                strokeWidth={1}
              />
              {series.map((s) => {
                const v = hoverDate === undefined ? undefined : s.values.get(hoverDate);
                if (v === undefined) return null;
                return (
                  <circle
                    key={s.key}
                    cx={x(hover)}
                    cy={y(v)}
                    r={5}
                    fill={s.color}
                    /* 2px surface ring instead of a border around the mark. */
                    stroke="var(--surface-1)"
                    strokeWidth={2}
                  />
                );
              })}
            </g>
          )}

          <line x1={0} x2={plotW} y1={plotH} y2={plotH} stroke="var(--baseline)" strokeWidth={1} />

          {dates.map((d, i) =>
            i % labelEvery === 0 || i === dates.length - 1 ? (
              <text
                key={d}
                x={x(i)}
                y={plotH + 18}
                textAnchor={i === 0 ? "start" : i === dates.length - 1 ? "end" : "middle"}
                fontSize={11}
                fill="var(--text-muted)"
                style={{ fontVariantNumeric: "tabular-nums" }}
              >
                {shortDate(d)}
              </text>
            ) : null,
          )}
        </g>
      </svg>

      {hover !== null && hoverDate !== undefined && (
        <Tooltip
          x={M.left + x(hover)}
          chartWidth={width}
          date={hoverDate}
          series={series}
          formatValue={formatValue}
        />
      )}
    </div>
  );
}

function Tooltip({
  x,
  chartWidth,
  date,
  series,
  formatValue,
}: {
  x: number;
  chartWidth: number;
  date: string;
  series: Series[];
  formatValue: (v: number) => string;
}) {
  const rows = series
    .map((s) => ({ s, v: s.values.get(date) }))
    .filter((r): r is { s: Series; v: number } => r.v !== undefined);
  if (rows.length === 0) return null;

  // Flip to the left of the crosshair near the right edge so the tooltip is
  // never clipped by the card.
  const flip = x > chartWidth - 190;

  return (
    <div
      className="tooltip"
      style={{
        left: flip ? undefined : x + 14,
        right: flip ? chartWidth - x + 14 : undefined,
        top: 6,
      }}
    >
      <div className="t-date">{longDate(date)}</div>
      {rows.map(({ s, v }) => (
        <div className="t-row" key={s.key}>
          <span className="swatch" style={{ background: s.color }} />
          <span className="name">{s.name}</span>
          <span className="val">{formatValue(v)}</span>
        </div>
      ))}
    </div>
  );
}

/** Ticks on round numbers, with the domain padded outward to reach them. */
function niceScale(lo: number, hi: number, count = 5) {
  if (lo === hi) {
    const pad = Math.abs(lo) * 0.1 || 1;
    lo -= pad;
    hi += pad;
  }
  const raw = (hi - lo) / count;
  const mag = Math.pow(10, Math.floor(Math.log10(raw)));
  const norm = raw / mag;
  const step = (norm >= 5 ? 10 : norm >= 2 ? 5 : norm >= 1 ? 2 : 1) * mag;

  const yMin = Math.floor(lo / step) * step;
  const yMax = Math.ceil(hi / step) * step;

  const ticks: number[] = [];
  // Nudge the bound to survive floating-point accumulation on the last tick.
  for (let t = yMin; t <= yMax + step * 1e-6; t += step) {
    ticks.push(Number(t.toFixed(10)));
  }
  return { yMin, yMax, ticks };
}

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

function shortDate(iso: string) {
  const [, m, d] = iso.split("-");
  return `${d} ${MONTHS[Number(m) - 1] ?? ""}`;
}

function longDate(iso: string) {
  const [yr, m, d] = iso.split("-");
  return `${d} ${MONTHS[Number(m) - 1] ?? ""} ${yr}`;
}
