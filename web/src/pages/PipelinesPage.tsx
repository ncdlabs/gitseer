import { Fragment, useMemo, useState } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import {
  api,
  openOnForgeLabel,
  repoLabel,
  safeExternalHref,
  type Job,
  type User,
  type WorkflowNode,
  type WorkflowRun,
} from "../api/client";
import { ExpandCollapseControls } from "../components/ExpandCollapseControls";
import { ForgeBadge } from "../components/ForgeBadge";
import { ForgeFilterChips, type ForgeFilterValue } from "../components/ForgeFilterChips";
import { ListControls } from "../components/ListControls";
import { SortableTh } from "../components/SortableTh";
import { WorkflowDAG } from "../components/WorkflowDAG";
import { WorkflowWriteActions } from "../components/WorkflowWriteActions";
import { resolveInstanceName, useForgeInventory } from "../hooks/useShowForgeUI";
import { useTableSort } from "../hooks/useTableSort";
import { useViewMode } from "../hooks/useViewMode";
import { relativeAge } from "../lib/relativeAge";

type ActionGroup = {
  key: string;
  name: string;
  workflowPath: string;
  repoFull: string;
  forgeType?: string;
  instanceId?: number;
  instanceName?: string;
  latest: WorkflowRun;
  runs: WorkflowRun[];
};

type PipelineGroupSortKey = "action" | "repository" | "forge" | "latest" | "runs";
type PipelineJobSortKey = "job" | "status";

const pipelineGroupAccessors = {
  action: (g: ActionGroup) => g.name,
  repository: (g: ActionGroup) => g.repoFull,
  forge: (g: ActionGroup) =>
    [g.forgeType, g.instanceName].filter(Boolean).join(" ").toLowerCase() || undefined,
  latest: (g: ActionGroup) =>
    (g.latest.conclusion || g.latest.status || "").toLowerCase() || undefined,
  runs: (g: ActionGroup) => g.runs.length,
} as const;

const pipelineJobAccessors = {
  job: (j: Job) => j.name,
  status: (j: Job) => (j.conclusion || j.status || "").toLowerCase() || undefined,
} as const;

function runTime(run: WorkflowRun): number {
  const raw = run.started_at || run.completed_at;
  if (!raw) return run.id;
  const t = Date.parse(raw);
  return Number.isNaN(t) ? run.id : t;
}

function normalizeWorkflowPath(path?: string): string {
  const p = (path || "").trim();
  if (!p) return "";
  const at = p.indexOf("@");
  return (at >= 0 ? p.slice(0, at) : p).trim();
}

function actionKey(run: WorkflowRun): string {
  const repo = run.repo_full || repoLabel(run) || "unknown";
  const path = normalizeWorkflowPath(run.workflow_path) || (run.name || "").trim() || `run-${run.id}`;
  return `${repo}\0${path}`;
}

function groupByAction(runs: WorkflowRun[]): ActionGroup[] {
  const buckets = new Map<string, WorkflowRun[]>();
  for (const run of runs) {
    const key = actionKey(run);
    const list = buckets.get(key);
    if (list) list.push(run);
    else buckets.set(key, [run]);
  }
  const groups: ActionGroup[] = [];
  for (const [key, list] of buckets) {
    const sorted = [...list].sort((a, b) => runTime(b) - runTime(a));
    const latest = sorted[0];
    const workflowPath = normalizeWorkflowPath(latest.workflow_path) || latest.workflow_path || "";
    groups.push({
      key,
      name: latest.name || workflowPath || `Action`,
      workflowPath,
      repoFull: latest.repo_full || repoLabel(latest) || "—",
      forgeType: latest.forge_type,
      instanceId: latest.instance_id,
      instanceName: latest.instance_name,
      latest,
      runs: sorted,
    });
  }
  groups.sort((a, b) => runTime(b.latest) - runTime(a.latest));
  return groups;
}

function statusBadge(run: WorkflowRun) {
  const label = run.conclusion || run.status;
  return <span className={`badge ${label}`}>{label}</span>;
}

function RunList({ runs }: { runs: WorkflowRun[] }) {
  return (
    <ul className="action-run-list">
      {runs.map((run) => (
        <li key={run.id}>
          <Link className="action-run" to={`/pipelines/${run.id}`}>
            <span className="action-run__status">{statusBadge(run)}</span>
            <span className="action-run__branch mono">{run.branch || "—"}</span>
            <span className="action-run__event muted">{run.event || "—"}</span>
            <span className="action-run__actor muted">{run.actor_login || "—"}</span>
            <span className="action-run__age muted">{relativeAge(run.started_at || run.completed_at) || "—"}</span>
            <span className="action-run__cta muted">Open run →</span>
          </Link>
        </li>
      ))}
    </ul>
  );
}

export function PipelinesPage() {
  const { mode, setMode } = useViewMode();
  const { showForge, forges, chipOptions } = useForgeInventory();
  const [filter, setFilter] = useState("");
  const [forgeFilter, setForgeFilter] = useState<ForgeFilterValue>("all");
  const effectiveForge = showForge ? forgeFilter : "all";
  const q = useQuery({
    queryKey: ["runs", filter, effectiveForge],
    queryFn: () => api.workflowRuns(filter, effectiveForge),
    placeholderData: keepPreviousData,
  });
  const [openKeys, setOpenKeys] = useState<Set<string>>(() => new Set());

  const groups = useMemo(() => groupByAction(q.data?.items ?? []), [q.data?.items]);
  const {
    sorted: sortedGroups,
    sortKey,
    sortDir,
    toggle: toggleSort,
  } = useTableSort<ActionGroup, PipelineGroupSortKey>(groups, pipelineGroupAccessors);

  if (q.isPending && !q.isPlaceholderData) return <div className="loading">Loading pipelines…</div>;
  if (q.isError) return <div className="error">{(q.error as Error).message}</div>;

  const toggleOpen = (key: string) => {
    setOpenKeys((cur) => {
      const next = new Set(cur);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };
  const allOpen = sortedGroups.length > 0 && sortedGroups.every((group) => openKeys.has(group.key));
  const anyOpen = sortedGroups.some((group) => openKeys.has(group.key));
  const expandAll = () => setOpenKeys(new Set(sortedGroups.map((group) => group.key)));
  const collapseAll = () => setOpenKeys(new Set());
  const empty =
    filter || effectiveForge !== "all" ? "No pipelines match this filter." : "No workflow runs indexed yet.";

  function groupBadge(group: ActionGroup) {
    return {
      forgeType: group.forgeType,
      instanceName: resolveInstanceName(forges, {
        forgeType: group.forgeType,
        instanceId: group.instanceId,
        instanceName: group.instanceName,
      }),
    };
  }

  return (
    <>
      <div className="topbar">
        <div className="page-header">
          <h1>Pipelines</h1>
          <p className="muted">
            Workflow actions grouped across accessible repositories. Expand an action to see individual runs.
          </p>
        </div>
        <ListControls
          mode={mode}
          onMode={setMode}
          filter={filter}
          onFilter={setFilter}
          filterPlaceholder="Filter pipelines…"
          filterLabel="Filter pipelines"
        >
          {showForge && (
            <ForgeFilterChips value={forgeFilter} onChange={setForgeFilter} options={chipOptions} />
          )}
          {sortedGroups.length > 0 && (
            <ExpandCollapseControls
              onExpandAll={expandAll}
              onCollapseAll={collapseAll}
              canExpandAll={!allOpen}
              canCollapseAll={anyOpen}
            />
          )}
        </ListControls>
      </div>
      {sortedGroups.length === 0 ? (
        <div className="empty">{empty}</div>
      ) : mode === "cards" ? (
        <div className="item-grid item-grid--actions">
          {sortedGroups.map((group) => {
            const open = openKeys.has(group.key);
            return (
              <div
                key={group.key}
                className={`item-card item-card--action${open ? " is-open" : ""}`}
              >
                <button
                  type="button"
                  className="action-card__toggle"
                  aria-expanded={open}
                  onClick={() => toggleOpen(group.key)}
                >
                  <div className="item-card__meta">
                    {showForge && <ForgeBadge {...groupBadge(group)} />}
                    {statusBadge(group.latest)}
                    <span className="item-card__age muted">
                      {group.runs.length} run{group.runs.length === 1 ? "" : "s"}
                    </span>
                  </div>
                  <div className="item-card__title">{group.name}</div>
                  <div className="item-card__repo mono">{group.repoFull}</div>
                  {group.workflowPath && group.workflowPath !== group.name && (
                    <div className="mono muted">{group.workflowPath}</div>
                  )}
                  <span className="item-card__cta muted">
                    {open ? "Hide Runs" : "Show Runs"}
                  </span>
                </button>
                {open && <RunList runs={group.runs} />}
              </div>
            );
          })}
        </div>
      ) : (
        <div className="panel">
          <table className="action-table">
            <thead>
              <tr>
                <SortableTh
                  label="Action"
                  columnKey="action"
                  sortKey={sortKey}
                  sortDir={sortDir}
                  onToggle={toggleSort}
                />
                <SortableTh
                  label="Repository"
                  columnKey="repository"
                  sortKey={sortKey}
                  sortDir={sortDir}
                  onToggle={toggleSort}
                />
                {showForge && (
                  <SortableTh
                    label="Forge"
                    columnKey="forge"
                    sortKey={sortKey}
                    sortDir={sortDir}
                    onToggle={toggleSort}
                  />
                )}
                <SortableTh
                  label="Latest"
                  columnKey="latest"
                  sortKey={sortKey}
                  sortDir={sortDir}
                  onToggle={toggleSort}
                />
                <SortableTh
                  label="Runs"
                  columnKey="runs"
                  sortKey={sortKey}
                  sortDir={sortDir}
                  onToggle={toggleSort}
                />
              </tr>
            </thead>
            <tbody>
              {sortedGroups.map((group) => {
                const open = openKeys.has(group.key);
                return (
                  <Fragment key={group.key}>
                    <tr className={open ? "is-open" : undefined}>
                      <td>
                        <button
                          type="button"
                          className="action-table__toggle"
                          aria-expanded={open}
                          onClick={() => toggleOpen(group.key)}
                        >
                          <span className="action-table__chevron" aria-hidden>
                            {open ? "▾" : "▸"}
                          </span>
                          <span className="action-table__name">{group.name}</span>
                          {group.workflowPath && group.workflowPath !== group.name && (
                            <span className="mono muted">{group.workflowPath}</span>
                          )}
                        </button>
                      </td>
                      <td className="mono">{group.repoFull}</td>
                      {showForge && (
                        <td>
                          <ForgeBadge {...groupBadge(group)} />
                        </td>
                      )}
                      <td>{statusBadge(group.latest)}</td>
                      <td>{group.runs.length}</td>
                    </tr>
                    {open && (
                      <tr className="action-table__detail">
                        <td colSpan={showForge ? 5 : 4}>
                          <RunList runs={group.runs} />
                        </td>
                      </tr>
                    )}
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}

export function PipelineDetailPage() {
  const { id } = useParams();
  const { showForge, forges } = useForgeInventory();
  const runId = Number(id);
  const validId = Number.isFinite(runId) && runId > 0;
  const me = useQuery({ queryKey: ["me"], queryFn: api.me, retry: false });
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

  const jobs = q.data?.jobs ?? [];
  const jobStatus = useMemo(() => {
    const map: Record<string, string> = {};
    for (const job of jobs) {
      map[job.name] = job.conclusion || job.status;
    }
    return map;
  }, [jobs]);
  const {
    sorted: sortedJobs,
    sortKey: jobSortKey,
    sortDir: jobSortDir,
    toggle: toggleJobSort,
  } = useTableSort<Job, PipelineJobSortKey>(jobs, pipelineJobAccessors);

  if (!validId) return <div className="error">Invalid pipeline id.</div>;
  if (q.isLoading || q.isPending) return <div className="loading">Loading run…</div>;
  if (q.isError) return <div className="error">{(q.error as Error).message}</div>;
  if (!q.data) return <div className="error">Run not found.</div>;
  const { run, graph, graph_error: graphError } = q.data;
  const forgeHref = safeExternalHref(run.html_url);
  const user = (me.data as User | undefined) ?? null;
  return (
    <>
      <div className="topbar">
        <div className="page-header">
          <p className="muted">
            <Link to="/pipelines">Pipelines</Link>
            {run.workflow_path ? ` · ${normalizeWorkflowPath(run.workflow_path) || run.workflow_path}` : ""}
          </p>
          <h1>{run.name}</h1>
          <p className="muted">
            {repoLabel(run)} · {run.branch}
            {showForge && (
              <>
                {" · "}
                <ForgeBadge
                  forgeType={run.forge_type}
                  instanceName={resolveInstanceName(forges, {
                    forgeType: run.forge_type,
                    instanceId: run.instance_id,
                    instanceName: run.instance_name,
                  })}
                />
              </>
            )}{" "}
            · <span className={`badge ${run.conclusion || run.status}`}>{run.conclusion || run.status}</span>
          </p>
        </div>
        <div className="topbar__actions">
          <WorkflowWriteActions run={run} user={user} />
          {forgeHref && (
            <a className="btn" href={forgeHref} target="_blank" rel="noreferrer">
              {openOnForgeLabel(run.forge_type).replace(" →", "")}
            </a>
          )}
        </div>
      </div>
      <div className="panel panel--padded dag-panel">
        <h2>Workflow Graph</h2>
        <WorkflowDAG
          graph={(graph || []) as WorkflowNode[]}
          jobStatusByName={jobStatus}
          graphError={graphError}
        />
      </div>
      <div className="panel">
        <table>
          <thead>
            <tr>
              <SortableTh
                label="Job"
                columnKey="job"
                sortKey={jobSortKey}
                sortDir={jobSortDir}
                onToggle={toggleJobSort}
              />
              <SortableTh
                label="Status"
                columnKey="status"
                sortKey={jobSortKey}
                sortDir={jobSortDir}
                onToggle={toggleJobSort}
              />
              <th>Logs</th>
            </tr>
          </thead>
          <tbody>
            {sortedJobs.map((job) => (
              <tr key={job.id}>
                <td>{job.name}</td>
                <td><span className={`badge ${job.conclusion || job.status}`}>{job.conclusion || job.status}</span></td>
                <td>
                  <button className="btn" type="button" onClick={() => setLogJob(job.id)}>View Logs</button>
                </td>
              </tr>
            ))}
            {sortedJobs.length === 0 && (
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
