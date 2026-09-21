import type { Theme } from "../hooks/useTheme";
import { THEME_OPTIONS } from "../hooks/useTheme";

type Props = {
  theme: Theme;
  onTheme: (t: Theme) => void;
};

function ThemeIcon({ id }: { id: Theme }) {
  const common = {
    width: 18,
    height: 18,
    viewBox: "0 0 24 24",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: 1.75,
    strokeLinecap: "round" as const,
    strokeLinejoin: "round" as const,
    "aria-hidden": true as const,
  };

  switch (id) {
    case "system":
      return (
        <svg {...common}>
          <rect x="3" y="4" width="18" height="12" rx="2" />
          <path d="M8 20h8M12 16v4" />
        </svg>
      );
    case "light":
      return (
        <svg {...common}>
          <circle cx="12" cy="12" r="4" />
          <path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
        </svg>
      );
    case "dark":
      return (
        <svg {...common}>
          <path d="M21 14.5A8.5 8.5 0 1 1 9.5 3 7 7 0 0 0 21 14.5z" />
        </svg>
      );
    case "gruvbox":
      return (
        <svg {...common}>
          <circle cx="12" cy="12" r="8" />
          <circle cx="9" cy="10" r="1.5" fill="#fe8019" stroke="none" />
          <circle cx="14.5" cy="9.5" r="1.5" fill="#fabd2f" stroke="none" />
          <circle cx="11" cy="14.5" r="1.5" fill="#b8bb26" stroke="none" />
          <circle cx="15.5" cy="14" r="1.5" fill="#8ec07c" stroke="none" />
        </svg>
      );
  }
}

export function ThemePicker({ theme, onTheme }: Props) {
  return (
    <div className="theme-picker" role="radiogroup" aria-label="Color theme">
      {THEME_OPTIONS.map((opt) => {
        const selected = theme === opt.id;
        return (
          <button
            key={opt.id}
            type="button"
            className={`theme-picker__btn${selected ? " is-active" : ""}`}
            role="radio"
            aria-checked={selected}
            aria-label={opt.label}
            title={opt.label}
            onClick={() => onTheme(opt.id)}
          >
            <ThemeIcon id={opt.id} />
          </button>
        );
      })}
    </div>
  );
}
