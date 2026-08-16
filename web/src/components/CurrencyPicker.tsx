import { useRef } from "react";

interface Props {
  currencies: string[];
  value: string;
  onChange: (code: string) => void;
}

/**
 * A radio group of currency chips.
 *
 * A dropdown hides its options until clicked, which is the wrong trade for a
 * short, stable list: with ten currencies the whole set fits on screen, so
 * switching is one click instead of two and the reader can see what else is
 * available without opening anything.
 *
 * Implemented as a proper radiogroup with roving tabindex — one tab stop for
 * the group, arrow keys to move within it — rather than a row of buttons that
 * would put ten stops in the tab order.
 */
export default function CurrencyPicker({ currencies, value, onChange }: Props) {
  const ref = useRef<HTMLDivElement>(null);

  const onKeyDown = (e: React.KeyboardEvent) => {
    const keys = ["ArrowRight", "ArrowLeft", "Home", "End"];
    if (!keys.includes(e.key)) return;
    e.preventDefault();

    const i = currencies.indexOf(value);
    let next = i;
    if (e.key === "ArrowRight") next = (i + 1) % currencies.length;
    if (e.key === "ArrowLeft") next = (i - 1 + currencies.length) % currencies.length;
    if (e.key === "Home") next = 0;
    if (e.key === "End") next = currencies.length - 1;

    const code = currencies[next];
    if (code === undefined) return;
    onChange(code);
    // Move focus with the selection so the two never drift apart.
    ref.current?.querySelector<HTMLButtonElement>(`[data-code="${code}"]`)?.focus();
  };

  return (
    <div
      className="chips"
      role="radiogroup"
      aria-label="Currency"
      ref={ref}
      onKeyDown={onKeyDown}
    >
      {currencies.map((c) => {
        const selected = c === value;
        return (
          <button
            key={c}
            type="button"
            role="radio"
            data-code={c}
            aria-checked={selected}
            // Roving tabindex: only the selected chip is a tab stop.
            tabIndex={selected ? 0 : -1}
            className="chip"
            onClick={() => onChange(c)}
          >
            {c}
          </button>
        );
      })}
    </div>
  );
}
