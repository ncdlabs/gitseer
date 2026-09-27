import { useState, type ReactNode } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  api,
  openOnForgeLabel,
  repoLabel,
  safeExternalHref,
  type AttentionItem,
  type AttentionMuteUntil,
  type ForgeStatusRow,
} from "../api/client";
import { resolveInstanceName } from "../hooks/useShowForgeUI";
import type { ViewMode } from "../hooks/useViewMode";
import { relativeAge } from "../lib/relativeAge";
import { ForgeBadge } from "./ForgeBadge";

function typeLabel(type: string) {
  return type.replaceAll("_", " ");
}

function canFetchLogSnippet(item: AttentionItem) {
  if (item.entity_type === "job" || item.entity_type === "workflow_run") return true;
  try {
    const meta = JSON.parse(item.metadata_json || "{}") as { job_id?: number; run_id?: number };
    return Boolean(meta.job_id || meta.run_id);
  } catch {
    return false;
  }
}

function fallbackHref(item: AttentionItem): string | null {
  if (item.entity_type === "workflow_run" && item.entity_id) {
    return `/pipelines/${item.entity_id}`;
  }
  if (item.entity_type === "pull_request") {
    return "/pull-requests";
  }
  if (item.entity_type === "job") {
    try {
      const meta = JSON.parse(item.metadata_json || "{}") as { run_id?: number };
      if (meta.run_id) return `/pipelines/${meta.run_id}`;
    } catch {
      /* ignore */
    }
    return "/pipelines";
  }
  return null;
}

function itemHref(item: AttentionItem): { href: string; external: boolean } | null {
  const external = safeExternalHref(item.html_url);
  if (external) {
    return { href: external, external: true };
  }
  const internal = fallbackHref(item);
  if (internal) return { href: internal, external: false };
  return null;
}

type Props = {
  items?: AttentionItem[] | null;
  empty?: string;
  limit?: number;
  mode?: ViewMode;
  /** When true (2+ forges configured), show forge badges on rows. */
  showForge?: boolean;
  /** Configured forges — used to show instance name when a type has multiple. */
  forges?: ForgeStatusRow[];
};

function ItemLink({
  item,
  className,
  children,
}: {
  item: AttentionItem;
  className: string;
  children: ReactNode;
}) {
  const link = itemHref(item);
  if (!link) {
    return <div className={`${className} ${className}--static`}>{children}</div>;
  }
  if (link.external) {
    return (
      <a className={className} href={link.href} target="_blank" rel="noreferrer">
        {children}
      </a>
    );
  }
  return (
    <Link className={className} to={link.href}>
      {children}
    </Link>
  );
}

function itemBadge(item: AttentionItem, forges: ForgeStatusRow[]) {
  return {
    forgeType: item.forge_type,
    instanceName: resolveInstanceName(forges, {
      forgeType: item.forge_type,
      instanceId: item.instance_id,
      instanceName: item.instance_name,
    }),
  };
}

function MuteMenu({
  itemId,
  onDone,
}: {
  itemId: number;
  onDone: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");
  const mute = useMutation({
    mutationFn: (until: AttentionMuteUntil) => api.muteAttention(itemId, until),
    onSuccess: () => {
      setError("");
      setOpen(false);
      onDone();
    },
    onError: (err: Error) => setError(err.message || "Mute failed"),
  });

  function choose(until: AttentionMuteUntil) {
    if (mute.isPending) return;
    mute.mutate(until);
  }

  return (
    <div className="attention-mute">
      <button
        type="button"
        className="btn btn--small"
        aria-expanded={open}
        aria-haspopup="menu"
        disabled={mute.isPending}
        onClick={(e) => {
          e.preventDefault();
          e.stopPropagation();
          setOpen((v) => !v);
        }}
      >
        {mute.isPending ? "Muting…" : "Mute"}
      </button>
      {open && (
        <div
          className="attention-mute__menu"
          role="menu"
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
          }}
        >
          <button type="button" role="menuitem" className="attention-mute__option" onClick={() => choose("24h")}>
            Snooze 24h
          </button>
          <button type="button" role="menuitem" className="attention-mute__option" onClick={() => choose("7d")}>
            Snooze 7d
          </button>
          <button type="button" role="menuitem" className="attention-mute__option" onClick={() => choose("resolved")}>
            Mute Until Resolved
          </button>
        </div>
      )}
      {error && (
        <span className="error attention-mute__error" role="alert">
          {error}
        </span>
      )}
    </div>
  );
}

function LogSnippetButton({ itemId }: { itemId: number }) {
  const [open, setOpen] = useState(false);
  const [snippet, setSnippet] = useState("");
  const [meta, setMeta] = useState("");
  const [error, setError] = useState("");
  const fetchSnippet = useMutation({
    mutationFn: () => api.attentionLogSnippet(itemId),
    onSuccess: (data) => {
      setError("");
      setSnippet(data.snippet || "");
      setMeta(
        [
          data.job_name ? `Job ${data.job_name}` : null,
          data.truncated ? "truncated" : null,
          `${data.bytes} bytes`,
        ]
          .filter(Boolean)
          .join(" · "),
      );
      setOpen(true);
    },
    onError: (err: Error) => {
      setSnippet("");
      setMeta("");
      setError(err.message || "Could not fetch log snippet");
      setOpen(true);
    },
  });

  return (
    <div className="attention-log-snippet">
      <button
        type="button"
        className="btn btn--small"
        disabled={fetchSnippet.isPending}
        onClick={(e) => {
          e.preventDefault();
          e.stopPropagation();
          if (open) {
            setOpen(false);
            return;
          }
          fetchSnippet.mutate();
        }}
      >
        {fetchSnippet.isPending ? "Loading…" : open ? "Hide Log Tail" : "Log Tail"}
      </button>
      {open && (
        <div
          className="attention-log-snippet__panel"
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
          }}
        >
          {error ? (
            <p className="error" role="alert">
              {error}
            </p>
          ) : (
            <>
              {meta && <p className="muted mono attention-log-snippet__meta">{meta}</p>}
              <pre className="attention-log-snippet__pre">{snippet || "(empty)"}</pre>
            </>
          )}
        </div>
      )}
    </div>
  );
}

export function AttentionCards({
  items,
  empty = "Nothing needs attention.",
  limit,
  mode = "cards",
  showForge = false,
  forges = [],
}: Props) {
  const qc = useQueryClient();
  const source = items ?? [];
  const list = typeof limit === "number" ? source.slice(0, limit) : source;

  function invalidate() {
    void qc.invalidateQueries({ queryKey: ["attention"] });
    void qc.invalidateQueries({ queryKey: ["summary"] });
  }

  if (list.length === 0) {
    return <div className="empty">{empty}</div>;
  }

  if (mode === "table") {
    return (
      <div className="panel">
        <table>
          <thead>
            <tr>
              <th>Severity</th>
              <th>Type</th>
              <th>Title</th>
              <th>Repository</th>
              {showForge && <th>Forge</th>}
              <th>Age</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {list.map((item) => {
              const link = itemHref(item);
              const title = link?.external ? (
                <a href={link.href} target="_blank" rel="noreferrer">
                  {item.title}
                </a>
              ) : link ? (
                <Link to={link.href}>{item.title}</Link>
              ) : (
                item.title
              );
              return (
                <tr key={item.id}>
                  <td>
                    <span className={`badge ${item.severity}`}>{item.severity}</span>
                  </td>
                  <td className="mono">{typeLabel(item.type)}</td>
                  <td>{title}</td>
                  <td className="mono">{repoLabel(item) || "—"}</td>
                  {showForge && (
                    <td>
                      <ForgeBadge {...itemBadge(item, forges)} />
                    </td>
                  )}
                  <td className="muted">{relativeAge(item.opened_at)}</td>
                  <td>
                    <div className="attention-actions-row">
                      {canFetchLogSnippet(item) && <LogSnippetButton itemId={item.id} />}
                      <MuteMenu itemId={item.id} onDone={invalidate} />
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    );
  }

  return (
    <div className="item-grid">
      {list.map((item) => {
        const link = itemHref(item);
        return (
          <div key={item.id} className="item-card item-card--attention">
            <ItemLink item={item} className="item-card__body">
              <div className="item-card__meta">
                <span className={`badge ${item.severity}`}>{item.severity}</span>
                {showForge && <ForgeBadge {...itemBadge(item, forges)} />}
                <span className="item-card__type mono">{typeLabel(item.type)}</span>
                {item.opened_at && <span className="item-card__age muted">{relativeAge(item.opened_at)}</span>}
              </div>
              <div className="item-card__title">{item.title}</div>
              <div className="item-card__repo mono">{repoLabel(item) || "—"}</div>
              {link && (
                <span className="item-card__cta muted">
                  {link.external ? openOnForgeLabel(item.forge_type) : "Open in GitSeer →"}
                </span>
              )}
            </ItemLink>
            <div className="item-card__actions">
              {canFetchLogSnippet(item) && <LogSnippetButton itemId={item.id} />}
              <MuteMenu itemId={item.id} onDone={invalidate} />
            </div>
          </div>
        );
      })}
    </div>
  );
}
