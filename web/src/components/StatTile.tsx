interface Props {
  label: string;
  value: string;
  sub?: string;
  color?: string;
}

/**
 * A single headline number. When the story is one figure, a stat tile is the
 * right form — a one-bar chart would say the same thing with more ink.
 */
export default function StatTile({ label, value, sub, color }: Props) {
  return (
    <div className="tile">
      <div className="label">
        {color && <span className="swatch" style={{ background: color }} />}
        {label}
      </div>
      <div className="value">{value}</div>
      {sub && <div className="sub">{sub}</div>}
    </div>
  );
}
