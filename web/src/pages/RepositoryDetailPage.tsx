import { useQuery } from "@tanstack/react-query";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { api, openOnForgeLabel, safeExternalHref } from "../api/client";
import { FailureClustersPanel } from "../components/FailureClustersPanel";
import { ForgeBadge } from "../components/ForgeBadge";
import { HealthBadge } from "../components/HealthBadge";
import { resolveInstanceName, useForgeInventory } from "../hooks/useShowForgeUI";

/** Deep-link target for Gitea install-ui and bookmarks: /repositories/:owner/:repo?instance_id= */
export function RepositoryDetailPage() {
  const { owner = "", repo = "" } = useParams();
  const [params] = useSearchParams();
  const instanceIdRaw = params.get("instance_id");
  const instanceId = instanceIdRaw ? Number(instanceIdRaw) : undefined;
  const { showForge, forges } = useForgeInventory();

  const q = useQuery({
    queryKey: ["repository", owner, repo, instanceId ?? 0],
    queryFn: () => api.repository(owner, repo, instanceId && instanceId > 0 ? instanceId : undefined),
    enabled: Boolean(owner && repo),
    retry: false,
  });

  if (!owner || !repo) {
    return <div className="error">Missing repository path.</div>;
  }
  if (q.isPending) return <div className="loading">Loading repository…</div>;
  if (q.isError) {
    const msg = (q.error as Error).message || "not found";
    return (
      <div className="panel panel--padded">
        <h1>Repository</h1>
        <p className="error" role="alert">
          {msg}
        </p>
        <p className="muted">
          {msg.toLowerCase().includes("ambiguous") || msg.toLowerCase().includes("instance_id")
            ? "Pass instance_id when the same owner/name exists on more than one forge."
            : "This repository is not in your GitSeer inventory, or you do not have access."}
        </p>
        <Link className="btn" to={`/repositories?q=${encodeURIComponent(`${owner}/${repo}`)}`}>
          Search Repositories
        </Link>
      </div>
    );
  }

  const r = q.data;
  const href = safeExternalHref(r.html_url);
  const health = r.health;
  const instanceName = resolveInstanceName(forges, {
    forgeType: r.forge_type,
    instanceId: r.instance_id,
    instanceName: r.instance_name,
  });

  return (
    <>
      <div className="topbar">
        <div className="page-header">
          <h1>{r.full_name || `${owner}/${repo}`}</h1>
          <p className="muted">
            {showForge && (
              <>
                <ForgeBadge forgeType={r.forge_type} instanceName={instanceName} />{" "}
              </>
            )}
            <HealthBadge health={health} />{" "}
            Default branch {r.default_branch || "—"}
            {r.private ? " · private" : " · public"}
            {r.archived ? " · archived" : ""}
          </p>
        </div>
        <div className="topbar__actions">
          {href && (
            <a className="btn" href={href} target="_blank" rel="noreferrer">
              {openOnForgeLabel(r.forge_type)}
            </a>
          )}
          <Link className="btn primary" to={`/pull-requests?q=${encodeURIComponent(r.full_name || `${owner}/${repo}`)}`}>
            View Pull Requests
          </Link>
        </div>
      </div>

      {health && (
        <div className="panel panel--padded repo-health-summary">
          <h2>Health Summary</h2>
          <p className="muted">
            Rollup of open critical attention, default-branch CI, stale open PRs ({health.stale_pr_days}d), and CI fail
            rate over {health.window_days} days.
          </p>
          <dl className="repo-health-summary__grid">
            <div>
              <dt>Score</dt>
              <dd>
                <HealthBadge health={health} />
              </dd>
            </div>
            <div>
              <dt>Critical Attention</dt>
              <dd>{health.open_critical_attention}</dd>
            </div>
            <div>
              <dt>Default Branch</dt>
              <dd>
                {health.failing_default_branch ? (
                  health.failing_default_branch_run_id ? (
                    <Link to={`/pipelines/${health.failing_default_branch_run_id}`}>Failing</Link>
                  ) : (
                    "Failing"
                  )
                ) : (
                  "Passing / unknown"
                )}
              </dd>
            </div>
            <div>
              <dt>Stale Open PRs</dt>
              <dd>{health.stale_open_prs}</dd>
            </div>
            <div>
              <dt>CI Fail Rate</dt>
              <dd>
                {health.ci_runs_in_window > 0
                  ? `${(health.ci_fail_rate * 100).toFixed(0)}% (${health.ci_failures_in_window}/${health.ci_runs_in_window})`
                  : "No completed runs"}
              </dd>
            </div>
          </dl>
        </div>
      )}

      <div className="panel panel--padded">
        <h2>Failure Clusters</h2>
        <p className="muted">Repeated failed jobs grouped by workflow path and job name (last 7 days).</p>
        <FailureClustersPanel
          owner={owner}
          repo={repo}
          instanceId={instanceId && instanceId > 0 ? instanceId : undefined}
        />
      </div>

      <div className="panel panel--padded">
        <Link className="btn" to="/repositories">
          Back To Repositories
        </Link>
      </div>
    </>
  );
}
