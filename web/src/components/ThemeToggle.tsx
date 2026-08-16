import { useEffect, useState, type ReactElement } from "react";

type Theme = "light" | "dark" | "system";

const KEY = "birrwatch-theme";

function apply(theme: Theme) {
  const root = document.documentElement;
  if (theme === "system") root.removeAttribute("data-theme");
  else root.setAttribute("data-theme", theme);
}

/**
 * Three states, not two. "System" is a real choice and the default — stamping
 * an explicit attribute on first paint would override the preference the
 * visitor already expressed to their OS.
 */
export default function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>(() => {
    const stored = localStorage.getItem(KEY);
    return stored === "light" || stored === "dark" ? stored : "system";
  });

  useEffect(() => {
    apply(theme);
    if (theme === "system") localStorage.removeItem(KEY);
    else localStorage.setItem(KEY, theme);
  }, [theme]);

  const options: Array<{ value: Theme; label: string; icon: ReactElement }> = [
    { value: "light", label: "Light theme", icon: <SunIcon /> },
    { value: "system", label: "Match system theme", icon: <SystemIcon /> },
    { value: "dark", label: "Dark theme", icon: <MoonIcon /> },
  ];

  return (
    <div className="theme-toggle" role="group" aria-label="Color theme">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          title={o.label}
          aria-label={o.label}
          aria-pressed={theme === o.value}
          onClick={() => setTheme(o.value)}
        >
          {o.icon}
        </button>
      ))}
    </div>
  );
}

const stroke = {
  fill: "none",
  stroke: "currentColor",
  strokeWidth: 1.7,
  strokeLinecap: "round" as const,
  strokeLinejoin: "round" as const,
};

function SunIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <circle cx="12" cy="12" r="4.2" {...stroke} />
      <path
        {...stroke}
        d="M12 2.6v2.2M12 19.2v2.2M21.4 12h-2.2M4.8 12H2.6M18.6 5.4l-1.6 1.6M7 17l-1.6 1.6M18.6 18.6L17 17M7 7 5.4 5.4"
      />
    </svg>
  );
}

function MoonIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path {...stroke} d="M20.5 14.6A8.6 8.6 0 1 1 9.4 3.5a6.9 6.9 0 0 0 11.1 11.1Z" />
    </svg>
  );
}

function SystemIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <rect x="2.8" y="4.2" width="18.4" height="12.6" rx="2" {...stroke} />
      <path {...stroke} d="M8.5 20.2h7" />
    </svg>
  );
}
