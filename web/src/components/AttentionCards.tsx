import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { repoLabel, safeExternalHref, type AttentionItem } from "../api/client";
import type { ViewMode } from "../hooks/useViewMode";
import { relativeAge } from "../lib/relativeAge";

function typeLabel(type: string) {
  return type.replaceAll("_", " ");
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

export function AttentionCards({ items, empty = "Nothing needs attention.", limit, mode = "cards" }: Props) {
  const source = items ?? [];
  const list = typeof limit === "number" ? source.slice(0, limit) : source;
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
              <th>Age</th>
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
                  <td className="muted">{relativeAge(item.opened_at)}</td>
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
          <ItemLink key={item.id} item={item} className="item-card">
            <div className="item-card__meta">
              <span className={`badge ${item.severity}`}>{item.severity}</span>
              <span className="item-card__type mono">{typeLabel(item.type)}</span>
              {item.opened_at && <span className="item-card__age muted">{relativeAge(item.opened_at)}</span>}
            </div>
            <div className="item-card__title">{item.title}</div>
            <div className="item-card__repo mono">{repoLabel(item) || "—"}</div>
            {link && (
              <span className="item-card__cta muted">
                {link.external ? "Open in Gitea →" : "Open in Lens →"}
              </span>
            )}
          </ItemLink>
        );
      })}
    </div>
  );
}
