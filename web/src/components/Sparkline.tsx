interface Props {
  values: number[];
  color: string;
  /** Rendered at a fixed viewBox and stretched, so it needs no measurement. */
  width?: number;
  height?: number;
}

/**
 * The shape of a series, not its values. No axes, no labels, no tooltip — the
 * numbers are already on the tile above it and in the chart below.
 */
export default function Sparkline({ values, color, width = 200, height = 30 }: Props) {
  if (values.length < 2) return <div className="spark" />;

  const lo = Math.min(...values);
  const hi = Math.max(...values);
  const span = hi - lo || 1;
  const pad = 3;

  const pts = values.map((v, i) => {
    const x = (i / (values.length - 1)) * width;
    const y = height - pad - ((v - lo) / span) * (height - pad * 2);
    return [x, y] as const;
  });

  const line = pts.map(([x, y], i) => `${i === 0 ? "M" : "L"}${x.toFixed(1)},${y.toFixed(1)}`).join("");
  const area = `${line}L${width},${height}L0,${height}Z`;
  const id = `spark-${color.replace(/[^a-z0-9]/gi, "")}`;
  const last = pts[pts.length - 1]!;

  return (
    <svg
      className="spark"
      viewBox={`0 0 ${width} ${height}`}
      preserveAspectRatio="none"
      width="100%"
      aria-hidden="true"
      style={{ display: "block", overflow: "visible" }}
    >
      <defs>
        <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={color} stopOpacity="0.22" />
          <stop offset="100%" stopColor={color} stopOpacity="0" />
        </linearGradient>
      </defs>
      <path d={area} fill={`url(#${id})`} />
      <path
        d={line}
        fill="none"
        stroke={color}
        strokeWidth={1.6}
        strokeLinecap="round"
        strokeLinejoin="round"
        vectorEffect="non-scaling-stroke"
      />
      <circle cx={last[0]} cy={last[1]} r={2.2} fill={color} vectorEffect="non-scaling-stroke" />
    </svg>
  );
}
