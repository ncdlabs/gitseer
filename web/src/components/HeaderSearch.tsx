import { useEffect, useId, useMemo, useRef, useState, type FormEvent, type KeyboardEvent as ReactKeyboardEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import {
  api,
  type AttentionItem,
  type Organization,
  type PullRequest,
  type Repository,
  type SearchResult,
  type WorkflowRun,
} from "../api/client";
import type { Theme } from "../hooks/useTheme";
import { resolveInstanceName, useForgeInventory } from "../hooks/useShowForgeUI";
import { filterSearchCatalog, type SearchCatalogItem } from "../lib/searchCatalog";
import { openActionsPopout } from "./ActiveActionsPanel";
import { ForgeBadge } from "./ForgeBadge";
import { Glyph } from "./Glyph";

type Props = {
  canSync?: boolean;
  onSync?: () => void;
  onTheme: (t: Theme) => void;
  onLogout: () => void;
  onOpenActions: () => void;
};

type FlatEntry =
  | { key: string; group: "Commands" | "Pages"; kind: "catalog"; item: SearchCatalogItem }
  | { key: string; group: "Repositories"; kind: "repo"; item: Repository }
  | { key: string; group: "Organizations"; kind: "org"; item: Organization }
  | { key: string; group: "Pull Requests"; kind: "pr"; item: PullRequest }
  | { key: string; group: "Pipelines"; kind: "run"; item: WorkflowRun }
  | { key: string; group: "Attention"; kind: "attention"; item: AttentionItem };

function repoDetailPath(repo: { owner?: string; name?: string; full_name?: string; instance_id?: number }) {
  const owner = repo.owner || (repo.full_name || "").split("/")[0] || "";
  const name = repo.name || (repo.full_name || "").split("/")[1] || "";
  if (!owner || !name) return "/repositories";
  const q = repo.instance_id && repo.instance_id > 0 ? `?instance_id=${repo.instance_id}` : "";
  return `/repositories/${encodeURIComponent(owner)}/${encodeURIComponent(name)}${q}`;
}

export function HeaderSearch({ canSync, onSync, onTheme, onLogout, onOpenActions }: Props) {
  const inputId = useId();
  const rootRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const navigate = useNavigate();
  const { showForge, forges } = useForgeInventory();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<SearchResult | null>(null);
  const [searching, setSearching] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [activeIndex, setActiveIndex] = useState(0);
  const debounceRef = useRef<number | null>(null);
  const reqSeq = useRef(0);

  const catalog = useMemo(
    () => filterSearchCatalog(query, { includeSync: !!canSync }),
    [query, canSync],
  );

  const flat: FlatEntry[] = useMemo(() => {
    const out: FlatEntry[] = [];
    const commands = catalog.filter((c) => c.kind === "command");
    const pages = catalog.filter((c) => c.kind === "page");
    for (const item of commands) {
      out.push({ key: item.id, group: "Commands", kind: "catalog", item });
    }
    for (const item of pages) {
      out.push({ key: item.id, group: "Pages", kind: "catalog", item });
    }
    if (results) {
      for (const item of results.repositories) {
        out.push({ key: `repo-${item.id}`, group: "Repositories", kind: "repo", item });
      }
      for (const item of results.organizations) {
        out.push({ key: `org-${item.id}`, group: "Organizations", kind: "org", item });
      }
      for (const item of results.pull_requests) {
        out.push({ key: `pr-${item.id}`, group: "Pull Requests", kind: "pr", item });
      }
      for (const item of results.workflow_runs) {
        out.push({ key: `run-${item.id}`, group: "Pipelines", kind: "run", item });
      }
      for (const item of results.attention) {
        out.push({ key: `att-${item.id}`, group: "Attention", kind: "attention", item });
      }
    }
    return out;
  }, [catalog, results]);

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

  useEffect(() => {
    const onGlobal = (event: KeyboardEvent) => {
      if (event.key === "k" && (event.metaKey || event.ctrlKey)) {
        event.preventDefault();
        setOpen(true);
      }
    };
    document.addEventListener("keydown", onGlobal);
    return () => document.removeEventListener("keydown", onGlobal);
  }, []);

  useEffect(() => {
    if (!open) return;
    const q = query.trim();
    if (!q) {
      setResults(null);
      setError(null);
      setSearching(false);
      return;
    }
    if (debounceRef.current != null) window.clearTimeout(debounceRef.current);
    debounceRef.current = window.setTimeout(() => {
      const seq = ++reqSeq.current;
      setSearching(true);
      setError(null);
      void api
        .search(q)
        .then((res) => {
          if (seq !== reqSeq.current) return;
          setResults({
            repositories: res.repositories ?? [],
            organizations: res.organizations ?? [],
            pull_requests: res.pull_requests ?? [],
            workflow_runs: res.workflow_runs ?? [],
            attention: res.attention ?? [],
          });
        })
        .catch((err) => {
          if (seq !== reqSeq.current) return;
          setResults(null);
          setError(err instanceof Error ? err.message : "Search failed.");
        })
        .finally(() => {
          if (seq !== reqSeq.current) return;
          setSearching(false);
        });
    }, 200);
    return () => {
      if (debounceRef.current != null) window.clearTimeout(debounceRef.current);
    };
  }, [query, open]);

  const close = () => setOpen(false);

  function runCatalogItem(item: SearchCatalogItem) {
    close();
    if (item.to) {
      navigate(item.to);
      return;
    }
    const action = item.action;
    if (!action) return;
    if (action === "sync") {
      onSync?.();
      return;
    }
    if (action === "open-actions") {
      onOpenActions();
      return;
    }
    if (action === "popout-actions") {
      openActionsPopout();
      return;
    }
    if (action === "logout") {
      onLogout();
      return;
    }
    if (typeof action === "object" && action.type === "theme") {
      onTheme(action.theme);
    }
  }

  function activate(entry: FlatEntry) {
    if (entry.kind === "catalog") {
      runCatalogItem(entry.item);
      return;
    }
    close();
    if (entry.kind === "repo") {
      navigate(repoDetailPath(entry.item));
      return;
    }
    if (entry.kind === "org") {
      navigate(`/repositories?q=${encodeURIComponent(entry.item.name || entry.item.full_name || "")}`);
      return;
    }
    if (entry.kind === "pr") {
      navigate(`/pull-requests?q=${encodeURIComponent(String(entry.item.number || entry.item.title || ""))}`);
      return;
    }
    if (entry.kind === "run") {
      navigate(`/pipelines/${entry.item.id}`);
      return;
    }
    if (entry.kind === "attention") {
      const q = entry.item.title || entry.item.type || "";
      navigate(q ? `/attention?q=${encodeURIComponent(q)}` : "/attention");
    }
  }

  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (flat[activeIndex]) {
      activate(flat[activeIndex]);
      return;
    }
    const q = query.trim();
    if (!q || searching) return;
    setSearching(true);
    setError(null);
    void api
      .search(q)
      .then((res) => setResults(res))
      .catch((err) => {
        setResults(null);
        setError(err instanceof Error ? err.message : "Search failed.");
      })
      .finally(() => setSearching(false));
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

  const hasCatalog = catalog.length > 0;
  const hasObjects =
    results &&
    (results.repositories.length > 0 ||
      results.organizations.length > 0 ||
      results.pull_requests.length > 0 ||
      results.workflow_runs.length > 0 ||
      results.attention.length > 0);
  const showPanel = open && (query.trim() !== "" || hasCatalog || results !== null || error !== null || searching);
  const noMatches =
    !searching && !error && query.trim() !== "" && !hasCatalog && results !== null && !hasObjects;

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
    const groups: Array<FlatEntry["group"]> = [
      "Commands",
      "Pages",
      "Repositories",
      "Organizations",
      "Pull Requests",
      "Pipelines",
      "Attention",
    ];
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
              if (entry.kind === "catalog") {
                return (
                  <li key={entry.key} role="option" aria-selected={active}>
                    <button
                      type="button"
                      className={className}
                      onMouseEnter={() => setActiveIndex(index)}
                      onClick={() => activate(entry)}
                    >
                      <span>{entry.item.label}</span>
                    </button>
                  </li>
                );
              }
              if (entry.kind === "repo") {
                const label = entry.item.full_name || `${entry.item.owner}/${entry.item.name}`;
                return (
                  <li key={entry.key} role="option" aria-selected={active}>
                    <Link
                      className={className}
                      to={repoDetailPath(entry.item)}
                      onMouseEnter={() => setActiveIndex(index)}
                      onClick={close}
                    >
                      {forgeProps({
                        forgeType: entry.item.forge_type,
                        instanceId: entry.item.instance_id,
                        instanceName: entry.item.instance_name,
                      })}
                      <span>{label}</span>
                    </Link>
                  </li>
                );
              }
              if (entry.kind === "org") {
                const label = entry.item.full_name || entry.item.name;
                return (
                  <li key={entry.key} role="option" aria-selected={active}>
                    <Link
                      className={className}
                      to={`/repositories?q=${encodeURIComponent(entry.item.name || "")}`}
                      onMouseEnter={() => setActiveIndex(index)}
                      onClick={close}
                    >
                      {forgeProps({
                        forgeType: entry.item.forge_type,
                        instanceId: entry.item.instance_id,
                        instanceName: entry.item.instance_name,
                      })}
                      <span>{label}</span>
                    </Link>
                  </li>
                );
              }
              if (entry.kind === "pr") {
                const pr = entry.item;
                return (
                  <li key={entry.key} role="option" aria-selected={active}>
                    <Link
                      className={className}
                      to={`/pull-requests?q=${encodeURIComponent(String(pr.number || ""))}`}
                      onMouseEnter={() => setActiveIndex(index)}
                      onClick={close}
                    >
                      {forgeProps({
                        forgeType: pr.forge_type,
                        instanceId: pr.instance_id,
                        instanceName: pr.instance_name,
                      })}
                      <span>
                        {pr.repo_full ? `${pr.repo_full}#${pr.number}` : `#${pr.number}`} {pr.title}
                      </span>
                    </Link>
                  </li>
                );
              }
              if (entry.kind === "run") {
                const run = entry.item;
                return (
                  <li key={entry.key} role="option" aria-selected={active}>
                    <Link
                      className={className}
                      to={`/pipelines/${run.id}`}
                      onMouseEnter={() => setActiveIndex(index)}
                      onClick={close}
                    >
                      {forgeProps({
                        forgeType: run.forge_type,
                        instanceId: run.instance_id,
                        instanceName: run.instance_name,
                      })}
                      <span>
                        {run.repo_full ? `${run.repo_full} · ` : ""}
                        {run.name || `Run ${run.id}`}
                      </span>
                    </Link>
                  </li>
                );
              }
              const att = entry.item;
              return (
                <li key={entry.key} role="option" aria-selected={active}>
                  <Link
                    className={className}
                    to={
                      att.title
                        ? `/attention?q=${encodeURIComponent(att.title)}`
                        : "/attention"
                    }
                    onMouseEnter={() => setActiveIndex(index)}
                    onClick={close}
                  >
                    {forgeProps({
                      forgeType: att.forge_type,
                      instanceId: att.instance_id,
                      instanceName: att.instance_name,
                    })}
                    <span>
                      {att.repo_full ? `${att.repo_full} · ` : ""}
                      {att.title}
                    </span>
                  </Link>
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
          aria-label="Search"
          title="Search (⌘K)"
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
              Search pages, commands, and inventory
            </label>
            <input
              ref={inputRef}
              id={inputId}
              className="header-search__input"
              type="search"
              value={query}
              placeholder="Search pages, commands, repositories…"
              aria-label="Search pages, commands, repositories"
              aria-controls={showPanel ? `${inputId}-results` : undefined}
              aria-expanded={showPanel}
              aria-activedescendant={flat[activeIndex] ? `${inputId}-opt-${activeIndex}` : undefined}
              autoComplete="off"
              onChange={(event) => {
                setQuery(event.target.value);
                if (!event.target.value.trim()) {
                  setResults(null);
                  setError(null);
                }
              }}
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
              aria-label="Search results"
            >
              {searching && <p className="header-search__empty muted">Searching…</p>}
              {error && <p className="header-search__empty error">{error}</p>}
              {noMatches && <p className="header-search__empty muted">No matches.</p>}
              {!error && flat.length > 0 && renderGroups()}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
