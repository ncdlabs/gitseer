import { useState } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { AttentionCards } from "../components/AttentionCards";
import { ListControls } from "../components/ListControls";
import { useViewMode } from "../hooks/useViewMode";

export function AttentionPage() {
  const { mode, setMode } = useViewMode();
  const [filter, setFilter] = useState("");
  const q = useQuery({
    queryKey: ["attention", filter],
    queryFn: () => api.attention(filter),
    placeholderData: keepPreviousData,
  });
  if (q.isPending && !q.isPlaceholderData) return <div className="loading">Loading attention…</div>;
  if (q.isError) return <div className="error">{(q.error as Error).message}</div>;
  const empty = filter ? "No attention items match this filter." : "Nothing needs attention.";
  return (
    <>
      <div className="topbar">
        <div className="page-header">
          <h1>Attention</h1>
          <p className="muted">
            Prioritized operational items. Open an item for Lens detail or Gitea.
          </p>
        </div>
        <ListControls
          mode={mode}
          onMode={setMode}
          filter={filter}
          onFilter={setFilter}
          filterPlaceholder="Filter attention…"
          filterLabel="Filter attention"
        />
      </div>
      <AttentionCards items={q.data?.items} empty={empty} mode={mode} />
    </>
  );
}
