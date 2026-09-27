import { useMemo, useState } from "react";

export type SortDir = "asc" | "desc";

export type TableSortAccessors<T, K extends string> = Record<
  K,
  (item: T) => string | number | null | undefined
>;

export type UseTableSortResult<T, K extends string> = {
  sorted: T[];
  sortKey: K | null;
  sortDir: SortDir;
  toggle: (key: K) => void;
};

function isMissing(value: string | number | null | undefined): boolean {
  return value === null || value === undefined;
}

function compareValues(
  a: string | number | null | undefined,
  b: string | number | null | undefined,
  dir: SortDir,
): number {
  const aMissing = isMissing(a);
  const bMissing = isMissing(b);
  if (aMissing && bMissing) return 0;
  if (aMissing) return 1;
  if (bMissing) return -1;

  let cmp: number;
  if (typeof a === "number" && typeof b === "number") {
    cmp = a - b;
  } else {
    cmp = String(a).localeCompare(String(b), undefined, { numeric: true });
  }
  return dir === "asc" ? cmp : -cmp;
}

/**
 * Client-side table sort. Starts unsorted (input order) until the first header toggle.
 * First click on a column sorts ascending; second click descends; another column resets to ascending.
 * Missing values (`null` / `undefined`) always sort last.
 */
export function useTableSort<T, K extends string>(
  items: T[],
  accessors: TableSortAccessors<T, K>,
): UseTableSortResult<T, K> {
  const [sortKey, setSortKey] = useState<K | null>(null);
  const [sortDir, setSortDir] = useState<SortDir>("asc");

  const sorted = useMemo(() => {
    if (sortKey === null) return items;
    const accessor = accessors[sortKey];
    return [...items].sort((left, right) =>
      compareValues(accessor(left), accessor(right), sortDir),
    );
  }, [items, accessors, sortKey, sortDir]);

  function toggle(key: K) {
    if (sortKey === key) {
      setSortDir((prev) => (prev === "asc" ? "desc" : "asc"));
      return;
    }
    setSortKey(key);
    setSortDir("asc");
  }

  return { sorted, sortKey, sortDir, toggle };
}
