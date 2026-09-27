import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api, type FailureCluster } from "../api/client";
import { relativeAge } from "../lib/relativeAge";

type Props = {
  owner: string;
  repo: string;
  instanceId?: number;
  days?: number;
};

export function FailureClustersPanel({ owner, repo, instanceId, days = 7 }: Props) {
  const q = useQuery({
    queryKey: ["failure-clusters", owner, repo, instanceId ?? 0, days],
    queryFn: () => api.repositoryFailureClusters(owner, repo, instanceId, days),
    enabled: Boolean(owner && repo),
  });

  if (q.isPending) return <div className="muted">Loading failure clusters…</div>;
  if (q.isError) return <div className="error">{(q.error as Error).message}</div>;

  const items = q.data?.items ?? [];
  if (items.length === 0) {
    return <div className="empty">No repeated job failures in the last {days} days.</div>;
  }

  return (
    <div className="panel">
      <table>
        <thead>
          <tr>
            <th>Job</th>
            <th>Workflow</th>
            <th>Failures</th>
            <th>Last Failed</th>
          </tr>
        </thead>
        <tbody>
          {items.map((c: FailureCluster) => (
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
