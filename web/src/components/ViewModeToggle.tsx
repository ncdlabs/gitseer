import type { ReactNode } from "react";
import type { ViewMode } from "../hooks/useViewMode";

type Props = {
  mode: ViewMode;
  onMode: (mode: ViewMode) => void;
};

function TableIcon() {
  return (
    <svg
      width={16}
      height={16}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.75}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <rect x="3" y="4" width="18" height="16" rx="2" />
      <path d="M3 10h18M3 16h18M9 4v16" />
    </svg>
  );
}

function CardsIcon() {
  return (
    <svg
      width={16}
      height={16}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.75}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <rect x="3" y="3" width="8" height="8" rx="1.5" />
      <rect x="13" y="3" width="8" height="8" rx="1.5" />
      <rect x="3" y="13" width="8" height="8" rx="1.5" />
      <rect x="13" y="13" width="8" height="8" rx="1.5" />
    </svg>
  );
}

const OPTIONS: { id: ViewMode; label: string; icon: ReactNode }[] = [
  { id: "table", label: "Table view", icon: <TableIcon /> },
  { id: "cards", label: "Card view", icon: <CardsIcon /> },
];

export function ViewModeToggle({ mode, onMode }: Props) {
  return (
    <div className="view-mode-toggle" role="radiogroup" aria-label="View layout">
      {OPTIONS.map((opt) => {
        const selected = mode === opt.id;
        return (
          <button
            key={opt.id}
            type="button"
            className={`view-mode-toggle__btn${selected ? " is-active" : ""}`}
            role="radio"
            aria-checked={selected}
            aria-label={opt.label}
            title={opt.label}
            onClick={() => onMode(opt.id)}
          >
            {opt.icon}
          </button>
        );
      })}
    </div>
  );
}
