import { useQuery } from "@tanstack/react-query";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { api, openOnForgeLabel, safeExternalHref } from "../api/client";
import { ForgeBadge } from "../components/ForgeBadge";
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
      <div className="panel panel--padded">
        <p className="muted">
          Open pull requests and workflow runs for this repository appear on the inventory pages.
          Use Sync if this forge was just connected.
        </p>
        <Link className="btn" to="/repositories">
          Back To Repositories
        </Link>
      </div>
    </>
  );
}
