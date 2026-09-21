import type { DashboardRangeDays } from "../hooks/useDashboardRange";
import { DASHBOARD_RANGE_OPTIONS } from "../hooks/useDashboardRange";

type Props = {
  days: DashboardRangeDays;
  onDays: (days: DashboardRangeDays) => void;
};

function rangeLabel(opt: DashboardRangeDays): string {
  return opt === 0 ? "Now" : `${opt}d`;
}

function rangeAriaLabel(opt: DashboardRangeDays): string {
  return opt === 0 ? "Now" : `Last ${opt} day${opt === 1 ? "" : "s"}`;
}

export function RangeToggle({ days, onDays }: Props) {
  return (
    <div className="range-toggle" role="radiogroup" aria-label="Dashboard time range">
      {DASHBOARD_RANGE_OPTIONS.map((opt) => {
        const selected = days === opt;
        const label = rangeLabel(opt);
        const aria = rangeAriaLabel(opt);
        return (
          <button
            key={opt}
            type="button"
            className={`range-toggle__btn${selected ? " is-active" : ""}`}
            role="radio"
            aria-checked={selected}
            aria-label={aria}
            title={aria}
            onClick={() => onDays(opt)}
          >
            {label}
          </button>
        );
      })}
    </div>
  );
}
