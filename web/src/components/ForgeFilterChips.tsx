import type { ForgeChipOption } from "../hooks/useShowForgeUI";

/** Filter id: "all" | "gitea" | "github" | "instance:{id}" */
export type ForgeFilterValue = string;

type Props = {
  value: ForgeFilterValue;
  onChange: (value: ForgeFilterValue) => void;
  options?: ForgeChipOption[];
};

const FALLBACK: ForgeChipOption[] = [
  { id: "all", label: "All" },
  { id: "gitea", label: "Gitea", forgeType: "gitea" },
  { id: "github", label: "GitHub", forgeType: "github" },
];

/** Optional All / forge-type / per-instance filter chips for shared inventory lists. */
export function ForgeFilterChips({ value, onChange, options }: Props) {
  const opts = options && options.length > 0 ? options : FALLBACK;
  return (
    <div className="forge-filter" role="radiogroup" aria-label="Filter by forge">
      {opts.map((opt) => {
        const selected = value === opt.id;
        return (
          <button
            key={opt.id}
            type="button"
            role="radio"
            className={`forge-filter__chip${selected ? " is-active" : ""}`}
            aria-checked={selected}
            onClick={() => onChange(opt.id)}
          >
            {opt.label}
          </button>
        );
      })}
    </div>
  );
}

export function matchesForgeFilter(
  forgeType: string | undefined | null,
  filter: ForgeFilterValue,
  instanceId?: number | null,
): boolean {
  if (!filter || filter === "all") return true;
  if (filter.startsWith("instance:")) {
    const id = Number(filter.slice("instance:".length));
    if (!Number.isFinite(id) || id <= 0) return true;
    return instanceId === id;
  }
  const ft = (forgeType || "gitea").toLowerCase();
  return ft === filter.toLowerCase();
}
