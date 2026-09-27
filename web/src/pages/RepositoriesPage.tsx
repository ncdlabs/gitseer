import { useState } from "react";
import { Link } from "react-router-dom";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import {
  api,
  ciBadgeClass,
  ciLabel,
  openOnForgeLabel,
  repoLabel,
  reviewBadgeClass,
  reviewLabel,
  safeExternalHref,
} from "../api/client";
import { ForgeBadge } from "../components/ForgeBadge";
import { ForgeFilterChips, type ForgeFilterValue } from "../components/ForgeFilterChips";
import { HealthBadge } from "../components/HealthBadge";
import { ListControls } from "../components/ListControls";
import { resolveInstanceName, useForgeInventory } from "../hooks/useShowForgeUI";
import { useURLQueryFilter } from "../hooks/useURLQueryFilter";
import { useViewMode } from "../hooks/useViewMode";

function repoDetailPath(repo: { owner?: string; name?: string; full_name?: string; instance_id?: number }) {
  const owner = repo.owner || (repo.full_name || "").split("/")[0] || "";
  const name = repo.name || (repo.full_name || "").split("/")[1] || "";
  if (!owner || !name) return "/repositories";
  const q = repo.instance_id && repo.instance_id > 0 ? `?instance_id=${repo.instance_id}` : "";
  return `/repositories/${encodeURIComponent(owner)}/${encodeURIComponent(name)}${q}`;
}

export function RepositoriesPage() {
  const { mode, setMode } = useViewMode();
  const { showForge, forges, chipOptions } = useForgeInventory();
  const [filter, setFilter] = useURLQueryFilter();
  const [forgeFilter, setForgeFilter] = useState<ForgeFilterValue>("all");
  const effectiveForge = showForge ? forgeFilter : "all";
  const q = useQuery({
    queryKey: ["repositories", filter, effectiveForge],
    queryFn: () => api.repositories(filter, effectiveForge),
    placeholderData: keepPreviousData,
  });
  const items = q.data?.items ?? [];

  if (q.isPending && !q.isPlaceholderData) return <div className="loading">Loading repositories…</div>;
  if (q.isError) return <div className="error">{(q.error as Error).message}</div>;
  const empty = filter || effectiveForge !== "all"
    ? "No repositories match this filter."
    : "No repositories yet. Use Sync in the header (bootstrap admin).";

  function badgeProps(repo: (typeof items)[number]) {
    return {
      forgeType: repo.forge_type,
      instanceName: resolveInstanceName(forges, {
        forgeType: repo.forge_type,
        instanceId: repo.instance_id,
        instanceName: repo.instance_name,
      }),
    };
  }

  return (
    <>
      <div className="topbar">
        <div className="page-header">
          <h1>Repositories</h1>
          <p className="muted">
            {q.data?.total ?? 0} repositor{q.data?.total === 1 ? "y" : "ies"} visible to you
            {filter ? ` matching “${filter}”` : ""}
            {effectiveForge !== "all" ? ` · ${effectiveForge}` : ""}.
          </p>
        </div>
        <ListControls
          mode={mode}
          onMode={setMode}
          filter={filter}
          onFilter={setFilter}
          filterPlaceholder="Filter repositories…"
          filterLabel="Filter repositories"
        >
          {showForge && (
            <ForgeFilterChips value={forgeFilter} onChange={setForgeFilter} options={chipOptions} />
          )}
        </ListControls>
      </div>
      {mode === "cards" ? (
        items.length === 0 ? (
          <div className="empty">{empty}</div>
        ) : (
          <div className="item-grid">
            {items.map((repo) => {
              const forgeHref = safeExternalHref(repo.html_url);
              const detail = repoDetailPath(repo);
              const badge = badgeProps(repo);
              return (
                <div key={repo.id} className="item-card item-card--static">
                  <Link className="item-card__title" to={detail}>
                    {repo.full_name}
                  </Link>
                  <div className="item-card__meta">
                    {showForge && <ForgeBadge {...badge} />}
                    <HealthBadge health={repo.health} />
                    <span className="badge">{repo.private ? "private" : "public"}</span>
                    {repo.archived && <span className="badge">archived</span>}
                  </div>
                  <div className="item-card__repo mono">{repo.default_branch || "—"}</div>
                  {forgeHref && (
                    <a className="item-card__cta muted" href={forgeHref} target="_blank" rel="noreferrer">
                      {openOnForgeLabel(repo.forge_type)}
                    </a>
                  )}
                </div>
              );
            })}
          </div>
        )
      ) : (
        <div className="panel">
          <table>
            <thead>
              <tr>
                <th>Repository</th>
                {showForge && <th>Forge</th>}
                <th>Health</th>
                <th>Default branch</th>
                <th>Visibility</th>
              </tr>
            </thead>
            <tbody>
              {items.map((repo) => {
                const forgeHref = safeExternalHref(repo.html_url);
                const detail = repoDetailPath(repo);
                return (
                  <tr key={repo.id}>
                    <td>
                      <Link to={detail}>{repo.full_name}</Link>
                      {forgeHref && (
                        <>
                          {" "}
                          <a className="muted" href={forgeHref} target="_blank" rel="noreferrer">
                            {openOnForgeLabel(repo.forge_type)}
                          </a>
                        </>
                      )}
                    </td>
                    {showForge && (
                      <td>
                        <ForgeBadge {...badgeProps(repo)} />
                      </td>
                    )}
                    <td>
                      <HealthBadge health={repo.health} />
                    </td>
                    <td className="mono">{repo.default_branch || "—"}</td>
                    <td>
                      {repo.private ? "Private" : "Public"}
                      {repo.archived ? " · Archived" : ""}
                    </td>
                  </tr>
                );
              })}
              {items.length === 0 && (
                <tr>
                  <td colSpan={showForge ? 5 : 4} className="empty">
                    {empty}
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

export function PullRequestsPage() {
  const { mode, setMode } = useViewMode();
  const { showForge, forges, chipOptions } = useForgeInventory();
  const [filter, setFilter] = useURLQueryFilter();
  const [forgeFilter, setForgeFilter] = useState<ForgeFilterValue>("all");
  const effectiveForge = showForge ? forgeFilter : "all";
  const q = useQuery({
    queryKey: ["prs", filter, effectiveForge],
    queryFn: () => api.pullRequests(filter, effectiveForge),
    placeholderData: keepPreviousData,
  });
  const items = q.data?.items ?? [];

  if (q.isPending && !q.isPlaceholderData) return <div className="loading">Loading pull requests…</div>;
  if (q.isError) return <div className="error">{(q.error as Error).message}</div>;
  const empty =
    filter || effectiveForge !== "all" ? "No pull requests match this filter." : "No open pull requests.";

  function badgeProps(pr: (typeof items)[number]) {
    return {
      forgeType: pr.forge_type,
      instanceName: resolveInstanceName(forges, {
        forgeType: pr.forge_type,
        instanceId: pr.instance_id,
        instanceName: pr.instance_name,
      }),
    };
  }

  return (
    <>
      <div className="topbar">
        <div className="page-header">
          <h1>Pull Requests</h1>
          <p className="muted">Open PRs across accessible repositories.</p>
        </div>
        <ListControls
          mode={mode}
          onMode={setMode}
          filter={filter}
          onFilter={setFilter}
          filterPlaceholder="Filter pull requests…"
          filterLabel="Filter pull requests"
        >
          {showForge && (
            <ForgeFilterChips value={forgeFilter} onChange={setForgeFilter} options={chipOptions} />
          )}
        </ListControls>
      </div>
      {mode === "cards" ? (
        items.length === 0 ? (
          <div className="empty">{empty}</div>
        ) : (
          <div className="item-grid">
            {items.map((pr) => {
              const href = safeExternalHref(pr.html_url);
              const body = (
                <>
                  <div className="item-card__meta">
                    <span className="mono">#{pr.number}</span>
                    {showForge && <ForgeBadge {...badgeProps(pr)} />}
                    <span className={`badge ${ciBadgeClass(pr.ci_state)}`}>{ciLabel(pr.ci_state)}</span>
                    {reviewLabel(pr.review_state) && (
                      <span className={`badge ${reviewBadgeClass(pr.review_state)}`}>
                        {reviewLabel(pr.review_state)}
                      </span>
                    )}
                    {pr.draft && <span className="badge">draft</span>}
                  </div>
                  <div className="item-card__title">
                    {pr.title}
                    {pr.draft ? " (draft)" : ""}
                  </div>
                  <div className="item-card__repo mono">{repoLabel(pr)}</div>
                  <div className="muted mono">{pr.author_login}</div>
                  {href && <span className="item-card__cta muted">{openOnForgeLabel(pr.forge_type)}</span>}
                </>
              );
              return href ? (
                <a key={pr.id} className="item-card" href={href} target="_blank" rel="noreferrer">
                  {body}
                </a>
              ) : (
                <div key={pr.id} className="item-card item-card--static">
                  {body}
                </div>
              );
            })}
          </div>
        )
      ) : (
        <div className="panel">
          <table>
            <thead>
              <tr>
                <th>PR</th>
                <th>Repository</th>
                {showForge && <th>Forge</th>}
                <th>Author</th>
                <th>CI</th>
                <th>Review</th>
              </tr>
            </thead>
            <tbody>
              {items.map((pr) => {
                const href = safeExternalHref(pr.html_url);
                return (
                  <tr key={pr.id}>
                    <td>
                      {href ? (
                        <a href={href} target="_blank" rel="noreferrer">
                          #{pr.number} {pr.title}
                          {pr.draft ? " (draft)" : ""}
                        </a>
                      ) : (
                        <>
                          #{pr.number} {pr.title}
                          {pr.draft ? " (draft)" : ""}
                        </>
                      )}
                    </td>
                    <td className="mono">{repoLabel(pr)}</td>
                    {showForge && (
                      <td>
                        <ForgeBadge {...badgeProps(pr)} />
                      </td>
                    )}
                    <td>{pr.author_login}</td>
                    <td>
                      <span className={`badge ${ciBadgeClass(pr.ci_state)}`}>{ciLabel(pr.ci_state)}</span>
                    </td>
                    <td>
                      {reviewLabel(pr.review_state) ? (
                        <span className={`badge ${reviewBadgeClass(pr.review_state)}`}>
                          {reviewLabel(pr.review_state)}
                        </span>
                      ) : (
                        "—"
                      )}
                    </td>
                  </tr>
                );
              })}
              {items.length === 0 && (
                <tr>
                  <td colSpan={showForge ? 6 : 5} className="empty">
                    {empty}
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
