import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { api, repoLabel, safeExternalHref, type WorkflowNode } from "../api/client";
import { useMemo, useState } from "react";
import { ViewModeToggle } from "../components/ViewModeToggle";
import { WorkflowDAG } from "../components/WorkflowDAG";
import { useViewMode } from "../hooks/useViewMode";

export function PipelinesPage() {
  const { mode, setMode } = useViewMode();
  const q = useQuery({ queryKey: ["runs"], queryFn: api.workflowRuns });
  if (q.isLoading) return <div className="loading">Loading pipelines…</div>;
  if (q.isError) return <div className="error">{(q.error as Error).message}</div>;
  const items = q.data?.items ?? [];
  return (
    <>
      <div className="topbar">
        <div className="page-header">
          <h1>Pipelines</h1>
          <p className="muted">Recent workflow runs across accessible repositories.</p>
        </div>
        <ViewModeToggle mode={mode} onMode={setMode} />
      </div>
      {mode === "cards" ? (
        items.length === 0 ? (
          <div className="empty">No workflow runs indexed yet.</div>
        ) : (
          <div className="item-grid">
            {items.map((run) => (
              <Link key={run.id} className="item-card" to={`/pipelines/${run.id}`}>
                <div className="item-card__meta">
                  <span className={`badge ${run.conclusion || run.status}`}>
                    {run.conclusion || run.status}
                  </span>
                  <span className="mono muted">{run.branch}</span>
                </div>
                <div className="item-card__title">{run.name || `Run #${run.id}`}</div>
                <div className="item-card__repo mono">{repoLabel(run)}</div>
                <span className="item-card__cta muted">Open run →</span>
              </Link>
            ))}
          </div>
        )
      ) : (
        <div className="panel">
          <table>
            <thead>
              <tr>
                <th>Run</th>
                <th>Repository</th>
                <th>Status</th>
                <th>Branch</th>
              </tr>
            </thead>
            <tbody>
              {items.map((run) => (
                <tr key={run.id}>
                  <td>
                    <Link to={`/pipelines/${run.id}`}>{run.name || `Run #${run.id}`}</Link>
                  </td>
                  <td className="mono">{repoLabel(run)}</td>
                  <td>
                    <span className={`badge ${run.conclusion || run.status}`}>
                      {run.conclusion || run.status}
                    </span>
                  </td>
                  <td className="mono">{run.branch}</td>
                </tr>
              ))}
              {items.length === 0 && (
                <tr>
                  <td colSpan={4} className="empty">
                    No workflow runs indexed yet.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}

export function PipelineDetailPage() {
  const { id } = useParams();
  const runId = Number(id);
  const validId = Number.isFinite(runId) && runId > 0;
  const q = useQuery({
    queryKey: ["run", runId],
    queryFn: () => api.workflowRun(runId),
    enabled: validId,
  });
  const [logJob, setLogJob] = useState<number | null>(null);
  const logs = useQuery({
    queryKey: ["logs", logJob],
    queryFn: () => api.jobLogs(logJob!),
    enabled: logJob != null,
  });

  const jobStatus = useMemo(() => {
    const map: Record<string, string> = {};
    for (const job of (q.data?.jobs as Array<{ name: string; status: string; conclusion: string }> | undefined) || []) {
      map[job.name] = job.conclusion || job.status;
    }
    return map;
  }, [q.data?.jobs]);

  if (!validId) return <div className="error">Invalid pipeline id.</div>;
  if (q.isLoading || q.isPending) return <div className="loading">Loading run…</div>;
  if (q.isError) return <div className="error">{(q.error as Error).message}</div>;
  if (!q.data) return <div className="error">Run not found.</div>;
  const { run, jobs, graph } = q.data;
  const giteaHref = safeExternalHref(run.html_url);
  return (
    <>
      <div className="topbar">
        <div className="page-header">
          <h1>{run.name}</h1>
          <p className="muted">
            {repoLabel(run)} · {run.branch} ·{" "}
            <span className={`badge ${run.conclusion || run.status}`}>{run.conclusion || run.status}</span>
          </p>
        </div>
        {giteaHref && (
          <a className="btn" href={giteaHref} target="_blank" rel="noreferrer">
            Open in Gitea
          </a>
        )}
      </div>
      <div className="panel panel--padded dag-panel">
        <h2>Workflow graph</h2>
        <WorkflowDAG graph={(graph || []) as WorkflowNode[]} jobStatusByName={jobStatus} />
      </div>
      <div className="panel">
        <table>
          <thead>
            <tr><th>Job</th><th>Status</th><th>Logs</th></tr>
          </thead>
          <tbody>
            {(jobs as Array<{ id: number; name: string; status: string; conclusion: string }>).map((job) => (
              <tr key={job.id}>
                <td>{job.name}</td>
                <td><span className={`badge ${job.conclusion || job.status}`}>{job.conclusion || job.status}</span></td>
                <td>
                  <button className="btn" type="button" onClick={() => setLogJob(job.id)}>View logs</button>
                </td>
              </tr>
            ))}
            {(jobs as unknown[]).length === 0 && (
              <tr><td colSpan={3} className="empty">No jobs for this run.</td></tr>
            )}
          </tbody>
        </table>
      </div>
      {logJob != null && (
        <div className="panel">
          <div className="panel__header">
            <strong>Job logs</strong>
            <button className="btn" type="button" onClick={() => setLogJob(null)}>Close</button>
          </div>
          {logs.isLoading && <div className="loading">Fetching logs…</div>}
          {logs.isError && <div className="error">{(logs.error as Error).message}</div>}
          {logs.data && <pre className="log-viewer">{logs.data}</pre>}
        </div>
      )}
    </>
  );
}
