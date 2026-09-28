import { useEffect, useMemo, useState } from "react";
import { keepPreviousData, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api, type DashboardScope } from "../api/client";
import { AttentionCards } from "../components/AttentionCards";
import { BarList } from "../components/charts/BarList";
import { StackedAreaChart } from "../components/charts/StackedAreaChart";
import { StatCallout } from "../components/charts/StatCallout";
import { DashboardFilter } from "../components/DashboardFilter";
import type { ForgeFilterValue } from "../components/ForgeFilterChips";
import { ListControls } from "../components/ListControls";
import { RangeToggle } from "../components/RangeToggle";
import { useDashboardRange } from "../hooks/useDashboardRange";
import { useForgeInventory } from "../hooks/useShowForgeUI";
import { useViewMode } from "../hooks/useViewMode";
import { prefetchDashboardRanges } from "../lib/prefetchDashboard";
import { relativeAge } from "../lib/relativeAge";

const LAYER_STALE_MS = 60_000;

const RUN_SERIES = [
  { key: "success", label: "Success", color: "var(--ok)" },
  { key: "failure", label: "Failure", color: "var(--danger)" },
  { key: "cancelled", label: "Cancelled", color: "var(--muted)" },
  { key: "other", label: "Other", color: "var(--accent)" },
] as const;

const PR_SERIES = [
  { key: "opened", label: "Opened", color: "var(--accent)" },
  { key: "merged", label: "Merged", color: "var(--ok)" },
  { key: "closed", label: "Closed", color: "var(--muted)" },
] as const;

function bucketColor(key: string): string {
  switch ((key || "").toLowerCase()) {
    case "success":
    case "ok":
    case "pass":
    case "low":
    case "medium":
      return "var(--ok)";
    case "failure":
    case "failed":
    case "fail":
    case "critical":
    case "error":
      return "var(--danger)";
    case "cancelled":
    case "canceled":
      return "var(--muted)";
    case "high":
    case "warn":
    case "warning":
    case "pending":
      return "var(--warn)";
    default:
      return "var(--accent)";
  }
}

function scopeFromFilters(orgId: number | null, forgeFilter: ForgeFilterValue, showForge: boolean): DashboardScope | undefined {
  const scope: DashboardScope = {};
  if (orgId != null && orgId > 0) scope.org_id = orgId;
  if (showForge && forgeFilter && forgeFilter !== "all") {
    if (forgeFilter.startsWith("instance:")) {
      const id = Number(forgeFilter.slice("instance:".length));
      if (Number.isFinite(id) && id > 0) scope.instance_id = id;
    } else {
      scope.forge_type = forgeFilter;
    }
  }
  return scope.org_id || scope.forge_type || scope.instance_id ? scope : undefined;
}

export function DashboardPage() {
  const queryClient = useQueryClient();
  const { mode, setMode } = useViewMode();
  const { showForge, forges, chipOptions } = useForgeInventory();
  const { days, setDays } = useDashboardRange();
  const [filter, setFilter] = useState("");
  const [orgId, setOrgId] = useState<number | null>(null);
  const [orgLabel, setOrgLabel] = useState("");
  const [forgeFilter, setForgeFilter] = useState<ForgeFilterValue>("all");
  const isNow = days === 0;
  const effectiveForge = showForge ? forgeFilter : "all";
  const scope = useMemo(
    () => scopeFromFilters(orgId, effectiveForge, showForge),
    [orgId, effectiveForge, showForge],
  );

  const summary = useQuery({
    queryKey: ["summary", days, scope],
    queryFn: () => api.summary(days, scope),
    staleTime: LAYER_STALE_MS,
    placeholderData: keepPreviousData,
  });
  const statsCore = useQuery({
    queryKey: ["stats", days, "core", scope],
    queryFn: () => api.stats(days, "core", scope),
    staleTime: LAYER_STALE_MS,
    placeholderData: keepPreviousData,
  });
  const statsTrends = useQuery({
    queryKey: ["stats", days, "trends", scope],
    queryFn: () => api.stats(days, "trends", scope),
    enabled: !isNow,
    staleTime: LAYER_STALE_MS,
    placeholderData: keepPreviousData,
  });
  const statsDuration = useQuery({
    queryKey: ["stats", days, "duration", scope],
    queryFn: () => api.stats(days, "duration", scope),
    enabled: !isNow,
    staleTime: LAYER_STALE_MS,
    placeholderData: keepPreviousData,
  });
  const attention = useQuery({
    queryKey: ["attention", filter],
    queryFn: () => api.attention(filter),
    placeholderData: keepPreviousData,
  });
  const runners = useQuery({
    queryKey: ["runners-utilization", 7],
    queryFn: () => api.runnerUtilization(7),
    staleTime: LAYER_STALE_MS,
  });
  const releases = useQuery({
    queryKey: ["releases", 30],
    queryFn: () => api.releases(30),
    staleTime: LAYER_STALE_MS,
  });

  useEffect(() => {
    if (!summary.isSuccess || !statsCore.isSuccess) return;
    void prefetchDashboardRanges(queryClient, { preferDays: days, scope });
  }, [summary.isSuccess, statsCore.isSuccess, days, scope, queryClient]);

  const s = summary.data;
  const items = attention.data?.items || [];
  const rangeLabel = days === 1 ? "last day" : `last ${days} days`;
  const core = statsCore.data;
  const trends = statsTrends.data;
  const duration = statsDuration.data?.run_duration ?? null;
  const scopeParts: string[] = [];
  if (orgId != null && orgLabel) scopeParts.push(orgLabel);
  if (showForge && effectiveForge !== "all") {
    const forgeOpt = chipOptions.find((o) => o.id === effectiveForge);
    if (forgeOpt) scopeParts.push(forgeOpt.label);
  }
  const scopeHint = scopeParts.length > 0 ? ` scoped to ${scopeParts.join(" · ")}` : "";

  return (
    <>
      <div className="topbar">
        <div className="page-header">
          <h1>Dashboard</h1>
          <p className="muted">
            {isNow
              ? `Current snapshot across repositories you can access${scopeHint}.`
              : `Operational report for the ${rangeLabel} across repositories you can access${scopeHint}.`}
          </p>
        </div>
      </div>

      {summary.isError && !s ? (
        <div className="error">{(summary.error as Error).message}</div>
      ) : summary.isLoading && !s ? (
        <div className="metrics">
          <div className="loading">Loading metrics…</div>
        </div>
      ) : s ? (
        <div className="metrics">
          <Link className="metric" to="/repositories">
            <div className="metric__label">Repositories</div>
            <div className="metric__value">{s.repositories}</div>
          </Link>
          <Link className="metric" to="/pull-requests">
            <div className="metric__label">Open PRs</div>
            <div className="metric__value">{s.open_pull_requests}</div>
          </Link>
          <Link className="metric" to="/attention">
            <div className="metric__label">Attention</div>
            <div className="metric__value">{s.attention_open}</div>
          </Link>
          <Link className="metric" to="/pipelines">
            <div className="metric__label">Failed runs</div>
            <div className="metric__value">{s.failed_runs}</div>
          </Link>
          <Link className="metric" to="/pipelines">
            <div className="metric__label">Running</div>
            <div className="metric__value">{s.running_runs}</div>
          </Link>
        </div>
      ) : null}

      <section className="report-section">
        <div className="panel__header report-section__header">
          <h2>Runner Utilization</h2>
          <div className="report-section__actions">
            <DashboardFilter
              orgId={orgId}
              onOrgId={(next, label) => {
                setOrgId(next);
                setOrgLabel(next == null ? "" : label || "");
              }}
              forgeFilter={effectiveForge}
              onForgeFilter={setForgeFilter}
              showForge={showForge}
              forgeOptions={chipOptions}
            />
            <RangeToggle days={days} onDays={setDays} />
          </div>
        </div>
        {runners.isLoading ? (
          <div className="loading">Loading runners…</div>
        ) : runners.isError ? (
          <div className="error">{(runners.error as Error).message}</div>
        ) : (
          <>
            {runners.data?.degraded ? (
              <p className="settings-form__hint" role="status">
                {runners.data.degraded_reason || "Runner utilization is degraded."}
              </p>
            ) : null}
            {(runners.data?.items?.length ?? 0) === 0 ? (
              <div className="empty">No runner activity in the last 7 days.</div>
            ) : (
              <div className="panel">
                <table>
                  <thead>
                    <tr>
                      <th>Runner</th>
                      <th>Busy</th>
                      <th>Queued</th>
                      <th>Completed</th>
                      <th>Failed</th>
                    </tr>
                  </thead>
                  <tbody>
                    {(runners.data?.items ?? []).map((row) => (
                      <tr key={row.runner_name}>
                        <td>{row.runner_name}</td>
                        <td>{row.busy_jobs}</td>
                        <td>{row.queued_jobs}</td>
                        <td>{row.completed_jobs}</td>
                        <td>{row.failed_jobs}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </>
        )}
      </section>

      <section className="report-section">
        <div className="panel__header report-section__header">
          <h2>Releases & Deployments</h2>
        </div>
        {releases.isLoading ? (
          <div className="loading">Loading releases…</div>
        ) : releases.isError ? (
          <div className="error">{(releases.error as Error).message}</div>
        ) : (releases.data?.items?.length ?? 0) === 0 ? (
          <div className="empty">No release or deploy-like runs in the last 30 days.</div>
        ) : (
          <div className="panel">
            <table>
              <thead>
                <tr>
                  <th>Repo</th>
                  <th>Workflow</th>
                  <th>Status</th>
                  <th>When</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {(releases.data?.items ?? []).slice(0, 12).map((rr) => (
                  <tr key={rr.id}>
                    <td>{rr.repo_full}</td>
                    <td>
                      {rr.name || rr.workflow_path}
                      {rr.attention_open ? <span className="badge badge--danger"> Attention</span> : null}
                    </td>
                    <td>
                      {rr.status}
                      {rr.conclusion ? ` / ${rr.conclusion}` : ""}
                    </td>
                    <td className="muted">
                      {rr.completed_at || rr.started_at
                        ? relativeAge(rr.completed_at || rr.started_at || "")
                        : "—"}
                    </td>
                    <td>
                      <Link className="btn btn--small" to={`/pipelines/${rr.id}`}>
                        Open
                      </Link>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {!isNow && (
        <section className="report-section">
          <div className="panel__header report-section__header">
            <h2>Trends</h2>
          </div>
          {statsTrends.isLoading && !trends ? (
            <div className="loading">Loading trends…</div>
          ) : statsTrends.isError && !trends ? (
            <div className="error">{(statsTrends.error as Error).message}</div>
          ) : (
            <div className="charts-grid charts-grid--trends">
              <div className="charts-grid__cell">
                <StackedAreaChart
                  title="Workflow runs"
                  description={`Daily workflow run outcomes for the ${rangeLabel}.`}
                  data={trends?.runs_by_day || []}
                  xKey="day"
                  series={[...RUN_SERIES]}
                  empty="No workflow runs in this range."
                />
                {statsDuration.isLoading && !duration ? (
                  <div className="loading">Loading run duration…</div>
                ) : statsDuration.isError && !duration ? (
                  <div className="error">{(statsDuration.error as Error).message}</div>
                ) : (
                  <StatCallout
                    title="Run duration"
                    p50Seconds={duration?.p50_seconds}
                    p95Seconds={duration?.p95_seconds}
                    sampleCount={duration?.sample_count ?? 0}
                  />
                )}
              </div>
              <div className="charts-grid__cell">
                <StackedAreaChart
                  title="Pull request activity"
                  description={`Daily pull request opened, merged, and closed counts for the ${rangeLabel}.`}
                  data={trends?.prs_by_day || []}
                  xKey="day"
                  series={[...PR_SERIES]}
                  empty="No pull request activity in this range."
                />
              </div>
            </div>
          )}
        </section>
      )}

      <section className="report-section">
        <div className="panel__header report-section__header">
          <h2>{isNow ? "Current State" : "Breakdowns"}</h2>
        </div>
        {statsCore.isLoading && !core ? (
          <div className="loading">Loading breakdowns…</div>
        ) : statsCore.isError && !core ? (
          <div className="error">{(statsCore.error as Error).message}</div>
        ) : (
          <div className="charts-grid charts-grid--breakdowns">
            {!isNow &&
              (statsTrends.isLoading && !trends ? (
                <div className="loading">Loading conclusions…</div>
              ) : (
                <BarList
                  title="Run conclusions"
                  items={trends?.run_conclusions || []}
                  colorForKey={bucketColor}
                  empty="No run conclusions in this range."
                />
              ))}
            <BarList
              title="Open PR CI state"
              items={core?.pr_ci_states || []}
              colorForKey={bucketColor}
              empty="No open pull requests."
            />
            <BarList
              title="Attention by severity"
              items={core?.attention_by_severity || []}
              colorForKey={bucketColor}
              empty="No open attention items."
            />
            <BarList
              title="Attention by type"
              items={core?.attention_by_type || []}
              colorForKey={bucketColor}
              empty="No open attention items."
            />
          </div>
        )}
      </section>

      <section className="report-section">
        <div className="panel__header report-section__header">
          <h2>Needs Attention</h2>
          <div className="report-section__actions">
            {items.length > 0 && (
              <Link className="muted" to="/attention">
                View all ({attention.data?.total ?? items.length})
              </Link>
            )}
            <ListControls
              mode={mode}
              onMode={setMode}
              filter={filter}
              onFilter={setFilter}
              filterPlaceholder="Filter attention…"
              filterLabel="Filter attention"
            />
          </div>
        </div>
        {attention.isLoading ? (
          <div className="loading">Loading attention…</div>
        ) : attention.isError ? (
          <div className="error">{(attention.error as Error).message}</div>
        ) : (
          <AttentionCards
            items={items}
            limit={12}
            empty={filter ? "No attention items match this filter." : "No open attention items."}
            mode={mode}
            showForge={showForge}
            forges={forges}
          />
        )}
      </section>
    </>
  );
}
