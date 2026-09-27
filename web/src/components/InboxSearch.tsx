import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type FormEvent,
  type KeyboardEvent as ReactKeyboardEvent,
} from "react";
import { Link, useNavigate } from "react-router-dom";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import {
  api,
  repoLabel,
  safeExternalHref,
  type AttentionItem,
  type InboxItem,
  type InboxReason,
  type PullRequest,
} from "../api/client";
import { resolveInstanceName, useForgeInventory } from "../hooks/useShowForgeUI";
import { ForgeBadge } from "./ForgeBadge";
import { Glyph } from "./Glyph";
import { inboxReasonLabel } from "./InboxReasonFilter";

type Props = {
  reason?: "" | InboxReason;
  forgeType?: string;
};

type FlatEntry =
  | { key: string; group: "Attention"; kind: "attention"; item: AttentionItem }
  | { key: string; group: "Pull Requests"; kind: "pr"; item: PullRequest };

function normalize(s: string) {
  return s.trim().toLowerCase();
}

function reasonHaystack(reasons: string[]) {
  return reasons.map((r) => `${r} ${inboxReasonLabel(r)}`).join(" ");
}

function authorFromMetadata(metadataJSON?: string): string {
  if (!metadataJSON?.trim()) return "";
  try {
    const meta = JSON.parse(metadataJSON) as { author_login?: unknown };
    return typeof meta.author_login === "string" ? meta.author_login : "";
  } catch {
    return "";
  }
}

function matchesQuery(item: InboxItem, q: string): boolean {
  if (!q) return true;
  const parts: string[] = [reasonHaystack(item.reasons)];
  if (item.attention) {
    const a = item.attention;
    parts.push(
      a.title,
      a.type,
      a.severity,
      a.repo_full || "",
      a.entity_type || "",
      authorFromMetadata(a.metadata_json),
    );
  }
  if (item.pull_request) {
    const pr = item.pull_request;
    parts.push(
      pr.title,
      String(pr.number),
      `#${pr.number}`,
      pr.author_login,
      pr.repo_full || "",
      repoLabel(pr),
      pr.ci_state || "",
      pr.review_state || "",
    );
  }
  return normalize(parts.filter(Boolean).join(" ")).includes(q);
}

function attentionHref(item: AttentionItem): { href: string; external: boolean } | null {
  const external = safeExternalHref(item.html_url);
  if (external) return { href: external, external: true };
  if (item.entity_type === "workflow_run" && item.entity_id) {
    return { href: `/pipelines/${item.entity_id}`, external: false };
  }
  if (item.entity_type === "pull_request") {
    return { href: "/pull-requests", external: false };
  }
  if (item.entity_type === "job") {
    try {
      const meta = JSON.parse(item.metadata_json || "{}") as { run_id?: number };
      if (meta.run_id) return { href: `/pipelines/${meta.run_id}`, external: false };
    } catch {
      /* ignore */
    }
    return { href: "/pipelines", external: false };
  }
  return item.title
    ? { href: `/attention?q=${encodeURIComponent(item.title)}`, external: false }
    : { href: "/attention", external: false };
}

function toFlat(items: InboxItem[], query: string, limit = 12): FlatEntry[] {
  const q = normalize(query);
  const out: FlatEntry[] = [];
  for (const it of items) {
    if (!matchesQuery(it, q)) continue;
    if (it.attention && it.kind === "attention") {
      out.push({
        key: `att-${it.attention.id}`,
        group: "Attention",
        kind: "attention",
        item: it.attention,
      });
    } else if (it.pull_request) {
      out.push({
        key: `pr-${it.pull_request.id}`,
        group: "Pull Requests",
        kind: "pr",
        item: it.pull_request,
      });
    }
    if (out.length >= limit) break;
  }
  return out;
}

/** Magnifying-glass search for the Inbox page — same look/behavior as header search, inbox-only. */
export function InboxSearch({ reason = "", forgeType = "all" }: Props) {
  const inputId = useId();
  const rootRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const navigate = useNavigate();
  const { showForge, forges } = useForgeInventory();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");

  const indexQ = useQuery({
    queryKey: ["inbox-search-index", reason, forgeType],
    queryFn: () => api.inbox({ reason: reason || undefined, forgeType }),
    placeholderData: keepPreviousData,
    enabled: open,
  });

  const items = indexQ.data?.items ?? [];
  const flat = useMemo(() => toFlat(items, query), [items, query]);
  const [activeIndex, setActiveIndex] = useState(0);

  useEffect(() => {
    setActiveIndex(0);
  }, [flat.length, query]);

  useEffect(() => {
    if (!open) return;
    const id = window.requestAnimationFrame(() => inputRef.current?.focus());
    return () => window.cancelAnimationFrame(id);
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: MouseEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  const close = () => setOpen(false);

  function activate(entry: FlatEntry) {
    close();
    if (entry.kind === "pr") {
      const href = safeExternalHref(entry.item.html_url);
      if (href) {
        window.open(href, "_blank", "noopener,noreferrer");
        return;
      }
      navigate(`/pull-requests?q=${encodeURIComponent(String(entry.item.number || entry.item.title || ""))}`);
      return;
    }
    const link = attentionHref(entry.item);
    if (!link) {
      navigate("/attention");
      return;
    }
    if (link.external) {
      window.open(link.href, "_blank", "noopener,noreferrer");
      return;
    }
    navigate(link.href);
  }

  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (flat[activeIndex]) {
      activate(flat[activeIndex]);
    }
  };

  const onInputKeyDown = (event: ReactKeyboardEvent<HTMLInputElement>) => {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      if (flat.length === 0) return;
      setActiveIndex((i) => (i + 1) % flat.length);
      return;
    }
    if (event.key === "ArrowUp") {
      event.preventDefault();
      if (flat.length === 0) return;
      setActiveIndex((i) => (i - 1 + flat.length) % flat.length);
      return;
    }
    if (event.key === "Enter" && flat[activeIndex]) {
      event.preventDefault();
      activate(flat[activeIndex]);
    }
  };

  const searching = indexQ.isFetching && !indexQ.isPlaceholderData;
  const error = indexQ.isError ? ((indexQ.error as Error).message || "Search failed.") : null;
  const showPanel = open && (query.trim() !== "" || flat.length > 0 || error !== null || searching || items.length === 0);
  const noMatches = !searching && !error && query.trim() !== "" && flat.length === 0 && indexQ.isSuccess;

  function forgeProps(opts: {
    forgeType?: string | null;
    instanceId?: number | null;
    instanceName?: string | null;
  }) {
    if (!showForge) return null;
    return (
      <ForgeBadge
        forgeType={opts.forgeType}
        instanceName={resolveInstanceName(forges, {
          forgeType: opts.forgeType,
          instanceId: opts.instanceId,
          instanceName: opts.instanceName,
        })}
      />
    );
  }

  function renderGroups() {
    const groups: Array<FlatEntry["group"]> = ["Attention", "Pull Requests"];
    return groups.map((group) => {
      const entries = flat.filter((e) => e.group === group);
      if (entries.length === 0) return null;
      return (
        <div className="header-search__group" key={group}>
          <div className="header-search__group-label">{group}</div>
          <ul className="header-search__list" role="group" aria-label={group}>
            {entries.map((entry) => {
              const index = flat.indexOf(entry);
              const active = index === activeIndex;
              const className = `header-search__item${active ? " is-active" : ""}`;
              if (entry.kind === "pr") {
                const pr = entry.item;
                const href = safeExternalHref(pr.html_url);
                const label = (
                  <span>
                    {pr.repo_full ? `${pr.repo_full}#${pr.number}` : `#${pr.number}`} {pr.title}
                  </span>
                );
                const badges = forgeProps({
                  forgeType: pr.forge_type,
                  instanceId: pr.instance_id,
                  instanceName: pr.instance_name,
                });
                return (
                  <li key={entry.key} role="option" aria-selected={active} id={`${inputId}-opt-${index}`}>
                    {href ? (
                      <a
                        className={className}
                        href={href}
                        target="_blank"
                        rel="noreferrer"
                        onMouseEnter={() => setActiveIndex(index)}
                        onClick={close}
                      >
                        {badges}
                        {label}
                      </a>
                    ) : (
                      <Link
                        className={className}
                        to={`/pull-requests?q=${encodeURIComponent(String(pr.number || ""))}`}
                        onMouseEnter={() => setActiveIndex(index)}
                        onClick={close}
                      >
                        {badges}
                        {label}
                      </Link>
                    )}
                  </li>
                );
              }
              const att = entry.item;
              const link = attentionHref(att);
              const label = (
                <span>
                  {att.repo_full ? `${att.repo_full} · ` : ""}
                  {att.title}
                </span>
              );
              const badges = forgeProps({
                forgeType: att.forge_type,
                instanceId: att.instance_id,
                instanceName: att.instance_name,
              });
              return (
                <li key={entry.key} role="option" aria-selected={active} id={`${inputId}-opt-${index}`}>
                  {link?.external ? (
                    <a
                      className={className}
                      href={link.href}
                      target="_blank"
                      rel="noreferrer"
                      onMouseEnter={() => setActiveIndex(index)}
                      onClick={close}
                    >
                      {badges}
                      {label}
                    </a>
                  ) : (
                    <Link
                      className={className}
                      to={link?.href || "/attention"}
                      onMouseEnter={() => setActiveIndex(index)}
                      onClick={close}
                    >
                      {badges}
                      {label}
                    </Link>
                  )}
                </li>
              );
            })}
          </ul>
        </div>
      );
    });
  }

  return (
    <div className={`header-search${open ? " is-open" : ""}`} ref={rootRef}>
      {!open ? (
        <button
          className="header-search__trigger"
          type="button"
          aria-label="Search Inbox"
          title="Search Inbox"
          aria-expanded={false}
          onClick={() => setOpen(true)}
        >
          <Glyph name="search" />
        </button>
      ) : (
        <div className="header-search__panel">
          <form className="header-search__flyout" role="search" onSubmit={onSubmit}>
            <span className="header-search__glyph" aria-hidden="true">
              <Glyph name="search" />
            </span>
            <label className="visually-hidden" htmlFor={inputId}>
              Search inbox items
            </label>
            <input
              ref={inputRef}
              id={inputId}
              className="header-search__input"
              type="search"
              value={query}
              placeholder="Search inbox items…"
              aria-label="Search inbox items"
              aria-controls={showPanel ? `${inputId}-results` : undefined}
              aria-expanded={showPanel}
              aria-activedescendant={flat[activeIndex] ? `${inputId}-opt-${activeIndex}` : undefined}
              autoComplete="off"
              onChange={(event) => setQuery(event.target.value)}
              onKeyDown={onInputKeyDown}
            />
            <button
              className="header-search__submit"
              type="submit"
              aria-label="Submit Search"
              title="Submit Search"
              disabled={searching && flat.length === 0}
            >
              <Glyph name="submit" />
            </button>
          </form>
          {showPanel && (
            <div
              id={`${inputId}-results`}
              className="header-search__results"
              role="listbox"
              aria-label="Inbox search results"
            >
              {searching && <p className="header-search__empty muted">Searching…</p>}
              {error && <p className="header-search__empty error">{error}</p>}
              {noMatches && <p className="header-search__empty muted">No matches.</p>}
              {!searching && !error && !query.trim() && items.length === 0 && (
                <p className="header-search__empty muted">Inbox is empty.</p>
              )}
              {!error && flat.length > 0 && renderGroups()}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
