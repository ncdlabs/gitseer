import { useQuery } from "@tanstack/react-query";
import { api, ciBadgeClass, ciLabel, repoLabel, safeExternalHref } from "../api/client";
import { ViewModeToggle } from "../components/ViewModeToggle";
import { useViewMode } from "../hooks/useViewMode";

export function RepositoriesPage() {
  const { mode, setMode } = useViewMode();
  const q = useQuery({ queryKey: ["repositories"], queryFn: () => api.repositories() });
  if (q.isLoading) return <div className="loading">Loading repositories…</div>;
  if (q.isError) return <div className="error">{(q.error as Error).message}</div>;
  const items = q.data?.items ?? [];
  return (
    <>
      <div className="topbar">
        <div className="page-header">
          <h1>Repositories</h1>
          <p className="muted">{q.data?.total ?? 0} repositories visible to you.</p>
        </div>
        <ViewModeToggle mode={mode} onMode={setMode} />
      </div>
      {mode === "cards" ? (
        items.length === 0 ? (
          <div className="empty">No repositories yet. Use Sync in the sidebar (bootstrap admin).</div>
        ) : (
          <div className="item-grid">
            {items.map((repo) => {
              const href = safeExternalHref(repo.html_url);
              return href ? (
                <a key={repo.id} className="item-card" href={href} target="_blank" rel="noreferrer">
                  <div className="item-card__title">{repo.full_name}</div>
                  <div className="item-card__meta">
                    <span className="badge">{repo.private ? "private" : "public"}</span>
                    {repo.archived && <span className="badge">archived</span>}
                  </div>
                  <div className="item-card__repo mono">{repo.default_branch || "—"}</div>
                  <span className="item-card__cta muted">Open in Gitea →</span>
                </a>
              ) : (
                <div key={repo.id} className="item-card item-card--static">
                  <div className="item-card__title">{repo.full_name}</div>
                  <div className="item-card__meta">
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
                  <td colSpan={3} className="empty">
                    No repositories yet. Use Sync in the sidebar (bootstrap admin).
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
  const q = useQuery({ queryKey: ["prs"], queryFn: api.pullRequests });
  if (q.isLoading) return <div className="loading">Loading pull requests…</div>;
  if (q.isError) return <div className="error">{(q.error as Error).message}</div>;
  const items = q.data?.items ?? [];
  return (
    <>
      <div className="topbar">
        <div className="page-header">
          <h1>Pull Requests</h1>
          <p className="muted">Open PRs across accessible repositories.</p>
        </div>
        <ViewModeToggle mode={mode} onMode={setMode} />
      </div>
      {mode === "cards" ? (
        items.length === 0 ? (
          <div className="empty">No open pull requests.</div>
        ) : (
          <div className="item-grid">
            {items.map((pr) => {
              const href = safeExternalHref(pr.html_url);
              const body = (
                <>
                  <div className="item-card__meta">
                    <span className="mono">#{pr.number}</span>
                    <span className={`badge ${ciBadgeClass(pr.ci_state)}`}>{ciLabel(pr.ci_state)}</span>
                    {pr.draft && <span className="badge">draft</span>}
                  </div>
                  <div className="item-card__title">
                    {pr.title}
                    {pr.draft ? " (draft)" : ""}
                  </div>
                  <div className="item-card__repo mono">{repoLabel(pr)}</div>
                  <div className="muted mono">{pr.author_login}</div>
                  {href && <span className="item-card__cta muted">Open in Gitea →</span>}
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
                    <td>{pr.author_login}</td>
                    <td>
                      <span className={`badge ${ciBadgeClass(pr.ci_state)}`}>{ciLabel(pr.ci_state)}</span>
                    </td>
                  </tr>
                );
              })}
              {items.length === 0 && (
                <tr>
                  <td colSpan={4} className="empty">
                    No open pull requests.
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
