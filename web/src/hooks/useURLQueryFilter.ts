import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";

/** List filter seeded from `?q=` and kept in sync when the query string changes. */
export function useURLQueryFilter(): [string, (next: string) => void] {
  const [params] = useSearchParams();
  const urlQ = params.get("q") ?? "";
  const [filter, setFilter] = useState(urlQ);
  useEffect(() => {
    setFilter(urlQ);
  }, [urlQ]);
  return [filter, setFilter];
}
