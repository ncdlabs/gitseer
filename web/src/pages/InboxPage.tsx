import { useMemo, useState } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import {
  api,
  openOnForgeLabel,
  repoLabel,
  safeExternalHref,
  type InboxItem,
  type InboxReason,
  type PullRequest,
} from "../api/client";
import { AttentionCards } from "../components/AttentionCards";
import { ForgeBadge } from "../components/ForgeBadge";
import { ForgeFilterChips, type ForgeFilterValue } from "../components/ForgeFilterChips";
import { ListControls } from "../components/ListControls";
import { SavedFiltersBar } from "../components/SavedFiltersBar";
import { resolveInstanceName, useForgeInventory } from "../hooks/useShowForgeUI";
import { useURLQueryFilter } from "../hooks/useURLQueryFilter";
import { useViewMode } from "../hooks/useViewMode";

const REASON_OPTIONS: Array<{ id: "" | InboxReason; label: string }> = [
  { id: "", label: "All" },
  { id: "author", label: "Author" },
  { id: "requested_reviewer", label: "Review Requests" },
  { id: "failing_ci", label: "Failing CI" },
  { id: "blocked_on_me", label: "Blocked On Me" },
];

function reasonLabel(r: string) {
  return r.replaceAll("_", " ");
}

function ciBadgeClass(state?: string) {
  switch ((state || "").toLowerCase()) {
    case "success":
      return "badge--ok";
    case "failure":
    case "error":
    case "timed_out":
      return "badge--fail";
    case "pending":
      return "badge--pending";
    default:
      return "";
  }
}

function InboxPRCard({
  pr,
  reasons,
  showForge,
  forges,
}: {
  pr: PullRequest;
  reasons: string[];
  showForge: boolean;
  forges: ReturnType<typeof useForgeInventory>["forges"];
}) {
  const href = safeExternalHref(pr.html_url);
  const body = (
    <>
      <div className="item-card__meta">
        <span className="mono">#{pr.number}</span>
        {showForge && (
          <ForgeBadge
            forgeType={pr.forge_type}
            instanceName={resolveInstanceName(forges, {
              forgeType: pr.forge_type,
              instanceId: pr.instance_id,
              instanceName: pr.instance_name,
            })}
          />
        )}
        {reasons.map((r) => (
          <span key={r} className="badge">
            {reasonLabel(r)}
          </span>
        ))}
        {pr.ci_state && <span className={`badge ${ciBadgeClass(pr.ci_state)}`}>{pr.ci_state}</span>}
      </div>
      <div className="item-card__title">{pr.title}</div>
      <div className="item-card__repo mono">{repoLabel(pr)}</div>
      <div className="muted mono">{pr.author_login}</div>
      {href && <span className="item-card__cta muted">{openOnForgeLabel(pr.forge_type)}</span>}
    </>
  );
  if (href) {
    return (
      <a className="item-card" href={href} target="_blank" rel="noreferrer">
        {body}
      </a>
    );
  }
  return (
    <Link className="item-card" to="/pull-requests">
      {body}
    </Link>
  );
}

export function InboxPage() {
  const { mode, setMode } = useViewMode();
  const { showForge, forges, chipOptions } = useForgeInventory();
  const [filter, setFilter] = useURLQueryFilter();
  const [params, setParams] = useSearchParams();
  const [forgeFilter, setForgeFilter] = useState<ForgeFilterValue>("all");
  const reason = (params.get("reason") || "") as "" | InboxReason;
  const effectiveForge = showForge ? forgeFilter : "all";

  const q = useQuery({
    queryKey: ["inbox", filter, reason, effectiveForge],
    queryFn: () => api.inbox({ q: filter, reason: reason || undefined, forgeType: effectiveForge }),
    placeholderData: keepPreviousData,
  });

  const attentionItems = useMemo(
    () =>
      (q.data?.items ?? [])
        .filter((it): it is InboxItem & { attention: NonNullable<InboxItem["attention"]> } => !!it.attention)
        .map((it) => it.attention),
    [q.data?.items],
  );
  const prItems = useMemo(
    () =>
      (q.data?.items ?? []).filter(
        (it): it is InboxItem & { pull_request: NonNullable<InboxItem["pull_request"]> } =>
          it.kind === "pull_request" && !!it.pull_request,
      ),
    [q.data?.items],
  );

  if (q.isPending && !q.isPlaceholderData) return <div className="loading">Loading inbox…</div>;
  if (q.isError) return <div className="error">{(q.error as Error).message}</div>;

  const empty =
    filter || reason || effectiveForge !== "all"
      ? "No inbox items match this filter."
      : "Your inbox is empty — nothing authored by you, assigned for review, or blocked on you.";

  function setReason(next: "" | InboxReason) {
    const nextParams = new URLSearchParams(params);
    if (next) nextParams.set("reason", next);
    else nextParams.delete("reason");
    setParams(nextParams, { replace: true });
  }

  return (
    <>
      <div className="topbar">
        <div className="page-header">
          <h1>Inbox</h1>
          <p className="muted">
            Personal queue: your PRs, review requests (when known), failing CI, and blocked-on-me items.
          </p>
        </div>
        <ListControls
          mode={mode}
          onMode={setMode}
          filter={filter}
          onFilter={setFilter}
          filterPlaceholder="Filter inbox…"
          filterLabel="Filter inbox"
        >
          <div className="forge-filter" role="radiogroup" aria-label="Filter by reason">
            {REASON_OPTIONS.map((opt) => (
              <button
                key={opt.id || "all"}
                type="button"
                role="radio"
                className={`forge-filter__chip${reason === opt.id ? " is-active" : ""}`}
                aria-checked={reason === opt.id}
                onClick={() => setReason(opt.id)}
              >
                {opt.label}
              </button>
            ))}
          </div>
          {showForge && (
            <ForgeFilterChips value={forgeFilter} onChange={setForgeFilter} options={chipOptions} />
          )}
        </ListControls>
      </div>
      <SavedFiltersBar
        page="inbox"
        currentQuery={{
          q: filter || undefined,
          reason: reason || undefined,
          forge_type: effectiveForge !== "all" && !effectiveForge.startsWith("instance:") ? effectiveForge : undefined,
          page: "inbox",
        }}
        onApply={(query) => {
          if (typeof query.q === "string") setFilter(query.q);
          if (typeof query.reason === "string") setReason((query.reason as InboxReason) || "");
        }}
      />
      {(q.data?.total ?? 0) === 0 ? (
        <div className="empty">{empty}</div>
      ) : (
        <>
          {attentionItems.length > 0 && (
            <section className="inbox-section">
              <h2 className="inbox-section__title">Attention</h2>
              <AttentionCards
                items={attentionItems}
                empty="No attention items."
                mode={mode}
                showForge={showForge}
                forges={forges}
              />
            </section>
          )}
          {prItems.length > 0 && (
            <section className="inbox-section">
              <h2 className="inbox-section__title">Pull Requests</h2>
              {mode === "cards" ? (
                <div className="item-grid">
                  {prItems.map((it) => (
                    <InboxPRCard
                      key={it.pull_request.id}
                      pr={it.pull_request}
                      reasons={it.reasons}
                      showForge={showForge}
                      forges={forges}
                    />
                  ))}
                </div>
              ) : (
                <div className="panel">
                  <table>
                    <thead>
                      <tr>
                        <th>PR</th>
                        <th>Repository</th>
                        <th>Reasons</th>
                        <th>CI</th>
                      </tr>
                    </thead>
                    <tbody>
                      {prItems.map((it) => {
                        const pr = it.pull_request;
                        const href = safeExternalHref(pr.html_url);
                        return (
                          <tr key={pr.id}>
                            <td>
                              {href ? (
                                <a href={href} target="_blank" rel="noreferrer">
                                  #{pr.number} {pr.title}
                                </a>
                              ) : (
                                <>
                                  #{pr.number} {pr.title}
                                </>
                              )}
                            </td>
                            <td className="mono">{repoLabel(pr)}</td>
                            <td>{it.reasons.map(reasonLabel).join(", ")}</td>
                            <td>
                              <span className={`badge ${ciBadgeClass(pr.ci_state)}`}>{pr.ci_state || "—"}</span>
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              )}
            </section>
          )}
        </>
      )}
    </>
  );
}
