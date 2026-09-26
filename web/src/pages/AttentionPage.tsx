import { useState } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { AttentionCards } from "../components/AttentionCards";
import { ForgeFilterChips, type ForgeFilterValue } from "../components/ForgeFilterChips";
import { ListControls } from "../components/ListControls";
import { useForgeInventory } from "../hooks/useShowForgeUI";
import { useURLQueryFilter } from "../hooks/useURLQueryFilter";
import { useViewMode } from "../hooks/useViewMode";

export function AttentionPage() {
  const { mode, setMode } = useViewMode();
  const { showForge, forges, chipOptions } = useForgeInventory();
  const [filter, setFilter] = useURLQueryFilter();
  const [forgeFilter, setForgeFilter] = useState<ForgeFilterValue>("all");
  const effectiveForge = showForge ? forgeFilter : "all";
  const q = useQuery({
    queryKey: ["attention", filter, effectiveForge],
    queryFn: () => api.attention(filter, effectiveForge),
    placeholderData: keepPreviousData,
  });
  if (q.isPending && !q.isPlaceholderData) return <div className="loading">Loading attention…</div>;
  if (q.isError) return <div className="error">{(q.error as Error).message}</div>;
  const empty =
    filter || effectiveForge !== "all" ? "No attention items match this filter." : "Nothing needs attention.";
  return (
    <>
      <div className="topbar">
        <div className="page-header">
          <h1>Attention</h1>
          <p className="muted">
            Prioritized operational items. Open an item for GitSeer detail or the forge.
          </p>
        </div>
        <ListControls
          mode={mode}
          onMode={setMode}
          filter={filter}
          onFilter={setFilter}
          filterPlaceholder="Filter attention…"
          filterLabel="Filter attention"
        >
          {showForge && (
            <ForgeFilterChips value={forgeFilter} onChange={setForgeFilter} options={chipOptions} />
          )}
        </ListControls>
      </div>
      <AttentionCards
        items={q.data?.items}
        empty={empty}
        mode={mode}
        showForge={showForge}
        forges={forges}
      />
    </>
  );
}
