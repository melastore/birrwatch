import Sparkline from "./Sparkline";

interface Props {
  label: string;
  value: string;
  sub?: string;
  color?: string;
  /** Percent change across the visible range, if it can be computed. */
  delta?: number;
  spark?: number[];
}

/**
 * A single headline number. When the story is one figure a stat tile is the
 * right form — a one-bar chart would say the same thing with more ink.
 */
export default function StatTile({ label, value, sub, color, delta, spark }: Props) {
  return (
    <div className="tile">
      <div className="label">
        {color && <span className="swatch" style={{ background: color }} />}
        {label}
      </div>

      <div className="value">
        {value}
        {delta !== undefined && Number.isFinite(delta) && (
          /* Direction is carried by the arrow glyph and the sign, never by
             color alone — and a rate moving up is not inherently good or bad,
             so this deliberately does not wear a status color. */
          <span className="delta">
            {delta > 0 ? "▲" : delta < 0 ? "▼" : "•"}
            {Math.abs(delta).toFixed(1)}%
          </span>
        )}
      </div>

      {sub && <div className="sub">{sub}</div>}
      {spark && spark.length > 1 && color && <Sparkline values={spark} color={color} />}
    </div>
  );
}
