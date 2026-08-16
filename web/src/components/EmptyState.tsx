interface Props {
  title: string;
  children: React.ReactNode;
}

/** An empty dataset is a state worth designing, not a blank rectangle. */
export default function EmptyState({ title, children }: Props) {
  return (
    <div className="empty">
      <div className="glyph" aria-hidden="true">
        <svg viewBox="0 0 24 24" width="22" height="22">
          <path
            fill="none"
            stroke="currentColor"
            strokeWidth="1.7"
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M3 20h18M5.5 20v-6M11 20V8.5M16.5 20v-3.5M22 20V4.5"
          />
        </svg>
      </div>
      <h3>{title}</h3>
      <p>{children}</p>
    </div>
  );
}
