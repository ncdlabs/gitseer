export type ForgeFilterValue = "all" | "gitea" | "github";

type Props = {
  value: ForgeFilterValue;
  onChange: (value: ForgeFilterValue) => void;
};

const OPTIONS: { id: ForgeFilterValue; label: string }[] = [
  { id: "all", label: "All" },
  { id: "gitea", label: "Gitea" },
  { id: "github", label: "GitHub" },
];

/** Optional All / Gitea / GitHub filter chips for shared inventory lists. */
export function ForgeFilterChips({ value, onChange }: Props) {
  return (
    <div className="forge-filter" role="radiogroup" aria-label="Filter by forge">
      {OPTIONS.map((opt) => {
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
): boolean {
  if (filter === "all") return true;
  const ft = (forgeType || "gitea").toLowerCase();
  return ft === filter;
}
