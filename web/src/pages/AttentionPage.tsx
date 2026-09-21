import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { AttentionCards } from "../components/AttentionCards";
import { ViewModeToggle } from "../components/ViewModeToggle";
import { useViewMode } from "../hooks/useViewMode";

export function AttentionPage() {
  const { mode, setMode } = useViewMode();
  const q = useQuery({ queryKey: ["attention"], queryFn: api.attention });
  if (q.isLoading) return <div className="loading">Loading attention…</div>;
  if (q.isError) return <div className="error">{(q.error as Error).message}</div>;
  return (
    <>
      <div className="topbar">
        <div className="page-header">
          <h1>Attention</h1>
          <p className="muted">
            Prioritized operational items. Open an item for Lens detail or Gitea.
          </p>
        </div>
        <ViewModeToggle mode={mode} onMode={setMode} />
      </div>
      <AttentionCards items={q.data?.items} empty="Nothing needs attention." mode={mode} />
    </>
  );
}
