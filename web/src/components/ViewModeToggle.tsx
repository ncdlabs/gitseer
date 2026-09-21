import type { ViewMode } from "../hooks/useViewMode";
import { Glyph } from "./Glyph";

type Props = {
  mode: ViewMode;
  onMode: (mode: ViewMode) => void;
};

const OPTIONS: { id: ViewMode; label: string; icon: "table" | "cards" }[] = [
  { id: "table", label: "Table view", icon: "table" },
  { id: "cards", label: "Card view", icon: "cards" },
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
            <Glyph name={opt.icon} />
          </button>
        );
      })}
    </div>
  );
}
