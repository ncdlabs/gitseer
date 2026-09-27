import type { SortDir } from "../hooks/useTableSort";

export type SortableThProps<K extends string> = {
  /** Title Case column label shown in the header button. */
  label: string;
  columnKey: K;
  sortKey: K | null;
  sortDir: SortDir;
  onToggle: (key: K) => void;
};

export function SortableTh<K extends string>({
  label,
  columnKey,
  sortKey,
  sortDir,
  onToggle,
}: SortableThProps<K>) {
  const active = sortKey === columnKey;
  const ariaSort = active ? (sortDir === "asc" ? "ascending" : "descending") : "none";

  return (
    <th className="sortable-th" aria-sort={ariaSort} scope="col">
      <button
        type="button"
        className="sortable-th__btn"
        onClick={() => onToggle(columnKey)}
      >
        <span className="sortable-th__label">{label}</span>
        {active ? (
          <span className="sortable-th__marker" aria-hidden="true">
            {sortDir === "asc" ? "▴" : "▾"}
          </span>
        ) : null}
      </button>
    </th>
  );
}
