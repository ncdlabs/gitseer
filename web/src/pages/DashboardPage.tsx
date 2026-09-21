import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import { AttentionCards } from "../components/AttentionCards";
import { BarList } from "../components/charts/BarList";
import { StackedAreaChart } from "../components/charts/StackedAreaChart";
import { StatCallout } from "../components/charts/StatCallout";
import { RangeToggle } from "../components/RangeToggle";
import { ViewModeToggle } from "../components/ViewModeToggle";
import { useDashboardRange } from "../hooks/useDashboardRange";
import { useViewMode } from "../hooks/useViewMode";

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

export function DashboardPage() {
  const { mode, setMode } = useViewMode();
  const { days, setDays } = useDashboardRange();
  const summary = useQuery({
    queryKey: ["summary", days],
    queryFn: () => api.summary(days),
  });
  const stats = useQuery({
    queryKey: ["stats", days],
    queryFn: () => api.stats(days),
  });
  const attention = useQuery({ queryKey: ["attention"], queryFn: api.attention });

  if (summary.isLoading) return <div className="loading">Loading dashboard…</div>;
  if (summary.isError) return <div className="error">{(summary.error as Error).message}</div>;

  const s = summary.data!;
  const items = attention.data?.items || [];
  const rangeLabel = days === 1 ? "last day" : `last ${days} days`;
  const report = stats.data;
  const duration = report?.run_duration ?? null;

  return (
    <>
      <div className="topbar">
        <div className="page-header">
          <h1>Dashboard</h1>
          <p className="muted">
            Operational report for the {rangeLabel} across repositories you can access.
          </p>
        </div>
        <RangeToggle days={days} onDays={setDays} />
      </div>
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

      <section className="report-section">
        <div className="panel__header report-section__header">
          <h2>Trends</h2>
        </div>
        {stats.isLoading ? (
          <div className="loading">Loading trends…</div>
        ) : stats.isError ? (
          <div className="error">{(stats.error as Error).message}</div>
        ) : (
          <div className="charts-grid charts-grid--trends">
            <div className="charts-grid__cell">
              <StackedAreaChart
                title="Workflow runs"
                description={`Daily workflow run outcomes for the ${rangeLabel}.`}
                data={report?.runs_by_day || []}
                xKey="day"
                series={[...RUN_SERIES]}
                empty="No workflow runs in this range."
              />
              <StatCallout
                title="Run duration"
                p50Seconds={duration?.p50_seconds}
                p95Seconds={duration?.p95_seconds}
                sampleCount={duration?.sample_count ?? 0}
              />
            </div>
            <div className="charts-grid__cell">
              <StackedAreaChart
                title="Pull request activity"
                description={`Daily pull request opened, merged, and closed counts for the ${rangeLabel}.`}
                data={report?.prs_by_day || []}
                xKey="day"
                series={[...PR_SERIES]}
                empty="No pull request activity in this range."
              />
            </div>
          </div>
        )}
      </section>

      <section className="report-section">
        <div className="panel__header report-section__header">
          <h2>Breakdowns</h2>
        </div>
        {stats.isLoading ? (
          <div className="loading">Loading breakdowns…</div>
        ) : stats.isError ? (
          <div className="error">{(stats.error as Error).message}</div>
        ) : (
          <div className="charts-grid charts-grid--breakdowns">
            <BarList
              title="Run conclusions"
              items={report?.run_conclusions || []}
              colorForKey={bucketColor}
              empty="No run conclusions in this range."
            />
            <BarList
              title="Open PR CI state"
              items={report?.pr_ci_states || []}
              colorForKey={bucketColor}
              empty="No open pull requests."
            />
            <BarList
              title="Attention by severity"
              items={report?.attention_by_severity || []}
              colorForKey={bucketColor}
              empty="No open attention items."
            />
            <BarList
              title="Attention by type"
              items={report?.attention_by_type || []}
              colorForKey={bucketColor}
              empty="No open attention items."
            />
          </div>
        )}
      </section>

      <section className="report-section">
        <div className="panel__header report-section__header">
          <h2>Needs attention now</h2>
          <div className="report-section__actions">
            {items.length > 0 && (
              <Link className="muted" to="/attention">
                View all ({attention.data?.total ?? items.length})
              </Link>
            )}
            <ViewModeToggle mode={mode} onMode={setMode} />
          </div>
        </div>
        {attention.isLoading ? (
          <div className="loading">Loading attention…</div>
        ) : attention.isError ? (
          <div className="error">{(attention.error as Error).message}</div>
        ) : (
          <AttentionCards items={items} limit={12} empty="No open attention items." mode={mode} />
        )}
      </section>
    </>
  );
}
