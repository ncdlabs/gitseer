import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api, type FlakyJob } from "../api/client";
import { SortableTh } from "./SortableTh";
import { useTableSort } from "../hooks/useTableSort";
import { relativeAge } from "../lib/relativeAge";

type Props = {
  owner: string;
  repo: string;
  instanceId?: number;
  days?: number;
};

type SortKey = "job" | "workflow" | "flips" | "failures" | "successes";

const accessors = {
  job: (c: FlakyJob) => c.job_name,
  workflow: (c: FlakyJob) => c.workflow_path,
  flips: (c: FlakyJob) => c.flip_count,
  failures: (c: FlakyJob) => c.failure_count,
  successes: (c: FlakyJob) => c.success_count,
} as const;

/** Heuristic flaky jobs (failed and succeeded in the window) for one repository. */
export function FlakyJobsPanel({ owner, repo, instanceId, days = 14 }: Props) {
  const q = useQuery({
    queryKey: ["flaky-jobs", owner, repo, instanceId ?? 0, days],
    queryFn: () => api.repositoryFlakyJobs(owner, repo, instanceId, days),
    enabled: Boolean(owner && repo),
  });

  const items = q.data?.items ?? [];
  const { sorted, sortKey, sortDir, toggle } = useTableSort<FlakyJob, SortKey>(items, accessors);

  if (q.isPending) return <div className="muted">Loading flaky jobs…</div>;
  if (q.isError) return <div className="error">{(q.error as Error).message}</div>;

  if (items.length === 0) {
    return <div className="empty">No flaky jobs detected in the last {days} days.</div>;
  }

  return (
    <div className="panel">
      <table>
        <thead>
          <tr>
            <SortableTh label="Job" columnKey="job" sortKey={sortKey} sortDir={sortDir} onToggle={toggle} />
            <SortableTh
              label="Workflow"
              columnKey="workflow"
              sortKey={sortKey}
              sortDir={sortDir}
              onToggle={toggle}
            />
            <SortableTh label="Flips" columnKey="flips" sortKey={sortKey} sortDir={sortDir} onToggle={toggle} />
            <SortableTh
              label="Failures"
              columnKey="failures"
              sortKey={sortKey}
              sortDir={sortDir}
              onToggle={toggle}
            />
            <SortableTh
              label="Successes"
              columnKey="successes"
              sortKey={sortKey}
              sortDir={sortDir}
              onToggle={toggle}
            />
            <th>Last Failed</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {sorted.map((c) => (
            <tr key={`${c.workflow_path}:${c.job_name}`}>
              <td>{c.job_name || "—"}</td>
              <td className="muted">{c.workflow_path || "—"}</td>
              <td>{c.flip_count}</td>
              <td>{c.failure_count}</td>
              <td>{c.success_count}</td>
              <td className="muted">{c.last_failed_at ? relativeAge(c.last_failed_at) : "—"}</td>
              <td>
                {c.sample_run_id ? (
                  <Link className="btn btn--small" to={`/pipelines/${c.sample_run_id}`}>
                    Open Run
                  </Link>
                ) : null}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
