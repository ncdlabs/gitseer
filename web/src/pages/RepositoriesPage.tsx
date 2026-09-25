import { useMemo, useState } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { api, ciBadgeClass, ciLabel, openOnForgeLabel, repoLabel, safeExternalHref } from "../api/client";
import { ForgeBadge } from "../components/ForgeBadge";
import { ForgeFilterChips, matchesForgeFilter, type ForgeFilterValue } from "../components/ForgeFilterChips";
import { ListControls } from "../components/ListControls";
import { useViewMode } from "../hooks/useViewMode";

export function RepositoriesPage() {
  const { mode, setMode } = useViewMode();
  const [filter, setFilter] = useState("");
  const [forgeFilter, setForgeFilter] = useState<ForgeFilterValue>("all");
  const q = useQuery({
    queryKey: ["repositories", filter],
    queryFn: () => api.repositories(filter),
    placeholderData: keepPreviousData,
  });
  const items = useMemo(() => {
    const all = q.data?.items ?? [];
    return all.filter((repo) => matchesForgeFilter(repo.forge_type, forgeFilter));
  }, [q.data?.items, forgeFilter]);

  if (q.isPending && !q.isPlaceholderData) return <div className="loading">Loading repositories…</div>;
  if (q.isError) return <div className="error">{(q.error as Error).message}</div>;
  const empty = filter || forgeFilter !== "all"
    ? "No repositories match this filter."
    : "No repositories yet. Use Sync in the header (bootstrap admin).";
  return (
    <>
      <div className="topbar">
        <div className="page-header">
          <h1>Repositories</h1>
          <p className="muted">
            {q.data?.total ?? 0} repositor{q.data?.total === 1 ? "y" : "ies"} visible to you
            {filter ? ` matching “${filter}”` : ""}
            {forgeFilter !== "all" ? ` · ${forgeFilter}` : ""}.
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
          <ForgeFilterChips value={forgeFilter} onChange={setForgeFilter} />
        </ListControls>
      </div>
      {mode === "cards" ? (
        items.length === 0 ? (
          <div className="empty">{empty}</div>
        ) : (
          <div className="item-grid">
            {items.map((repo) => {
              const href = safeExternalHref(repo.html_url);
              return href ? (
                <a key={repo.id} className="item-card" href={href} target="_blank" rel="noreferrer">
                  <div className="item-card__title">{repo.full_name}</div>
                  <div className="item-card__meta">
                    <ForgeBadge forgeType={repo.forge_type} />
                    <span className="badge">{repo.private ? "private" : "public"}</span>
                    {repo.archived && <span className="badge">archived</span>}
                  </div>
                  <div className="item-card__repo mono">{repo.default_branch || "—"}</div>
                  <span className="item-card__cta muted">{openOnForgeLabel(repo.forge_type)}</span>
                </a>
              ) : (
                <div key={repo.id} className="item-card item-card--static">
                  <div className="item-card__title">{repo.full_name}</div>
                  <div className="item-card__meta">
                    <ForgeBadge forgeType={repo.forge_type} />
                    <span className="badge">{repo.private ? "private" : "public"}</span>
                    {repo.archived && <span className="badge">archived</span>}
                  </div>
                  <div className="item-card__repo mono">{repo.default_branch || "—"}</div>
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
                <th>Forge</th>
                <th>Default branch</th>
                <th>Visibility</th>
              </tr>
            </thead>
            <tbody>
              {items.map((repo) => {
                const href = safeExternalHref(repo.html_url);
                return (
                  <tr key={repo.id}>
                    <td>
                      {href ? (
                        <a href={href} target="_blank" rel="noreferrer">
                          {repo.full_name}
                        </a>
                      ) : (
                        repo.full_name
                      )}
                    </td>
                    <td>
                      <ForgeBadge forgeType={repo.forge_type} />
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
                  <td colSpan={4} className="empty">
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
  const [filter, setFilter] = useState("");
  const [forgeFilter, setForgeFilter] = useState<ForgeFilterValue>("all");
  const q = useQuery({
    queryKey: ["prs", filter],
    queryFn: () => api.pullRequests(filter),
    placeholderData: keepPreviousData,
  });
  const items = useMemo(() => {
    const all = q.data?.items ?? [];
    return all.filter((pr) => matchesForgeFilter(pr.forge_type, forgeFilter));
  }, [q.data?.items, forgeFilter]);

  if (q.isPending && !q.isPlaceholderData) return <div className="loading">Loading pull requests…</div>;
  if (q.isError) return <div className="error">{(q.error as Error).message}</div>;
  const empty =
    filter || forgeFilter !== "all" ? "No pull requests match this filter." : "No open pull requests.";
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
          <ForgeFilterChips value={forgeFilter} onChange={setForgeFilter} />
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
                    <ForgeBadge forgeType={pr.forge_type} />
                    <span className={`badge ${ciBadgeClass(pr.ci_state)}`}>{ciLabel(pr.ci_state)}</span>
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
                <th>Forge</th>
                <th>Author</th>
                <th>CI</th>
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
                    <td>
                      <ForgeBadge forgeType={pr.forge_type} />
                    </td>
                    <td>{pr.author_login}</td>
                    <td>
                      <span className={`badge ${ciBadgeClass(pr.ci_state)}`}>{ciLabel(pr.ci_state)}</span>
                    </td>
                  </tr>
                );
              })}
              {items.length === 0 && (
                <tr>
                  <td colSpan={5} className="empty">
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
