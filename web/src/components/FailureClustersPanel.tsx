import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api, type FailureCluster } from "../api/client";
import { SortableTh } from "./SortableTh";
import { useTableSort } from "../hooks/useTableSort";
import { relativeAge } from "../lib/relativeAge";

type Props = {
  owner: string;
  repo: string;
  instanceId?: number;
  days?: number;
};

type FailureClusterSortKey = "job" | "workflow" | "failures" | "last_failed";

const failureClusterAccessors = {
  job: (c: FailureCluster) => c.job_name,
  workflow: (c: FailureCluster) => c.workflow_path,
  failures: (c: FailureCluster) => c.failure_count,
  last_failed: (c: FailureCluster) => {
    if (!c.last_failed_at) return undefined;
    const t = Date.parse(c.last_failed_at);
    return Number.isNaN(t) ? undefined : t;
  },
} as const;

export function FailureClustersPanel({ owner, repo, instanceId, days = 7 }: Props) {
  const q = useQuery({
    queryKey: ["failure-clusters", owner, repo, instanceId ?? 0, days],
    queryFn: () => api.repositoryFailureClusters(owner, repo, instanceId, days),
    enabled: Boolean(owner && repo),
  });

  const items = q.data?.items ?? [];
  const { sorted, sortKey, sortDir, toggle } = useTableSort<FailureCluster, FailureClusterSortKey>(
    items,
    failureClusterAccessors,
  );

  if (q.isPending) return <div className="muted">Loading failure clusters…</div>;
  if (q.isError) return <div className="error">{(q.error as Error).message}</div>;

  if (items.length === 0) {
    return <div className="empty">No repeated job failures in the last {days} days.</div>;
  }

  return (
    <div className="panel">
      <table>
        <thead>
          <tr>
            <SortableTh
              label="Job"
              columnKey="job"
              sortKey={sortKey}
              sortDir={sortDir}
              onToggle={toggle}
            />
            <SortableTh
              label="Workflow"
              columnKey="workflow"
              sortKey={sortKey}
              sortDir={sortDir}
              onToggle={toggle}
            />
            <SortableTh
              label="Failures"
              columnKey="failures"
              sortKey={sortKey}
              sortDir={sortDir}
              onToggle={toggle}
            />
            <SortableTh
              label="Last Failed"
              columnKey="last_failed"
              sortKey={sortKey}
              sortDir={sortDir}
              onToggle={toggle}
            />
          </tr>
        </thead>
        <tbody>
          {sorted.map((c: FailureCluster) => (
            <tr key={`${c.workflow_path}|${c.job_name}`}>
              <td>
                {c.sample_run_id ? (
                  <Link to={`/pipelines/${c.sample_run_id}`}>{c.job_name || "(unnamed)"}</Link>
                ) : (
                  c.job_name || "(unnamed)"
                )}
              </td>
              <td className="mono muted">{c.workflow_path || "—"}</td>
              <td>{c.failure_count}</td>
              <td className="muted">{c.last_failed_at ? relativeAge(c.last_failed_at) : "—"}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
