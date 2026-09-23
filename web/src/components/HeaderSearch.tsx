import { useEffect, useId, useRef, useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { api, safeExternalHref, type SearchResult } from "../api/client";
import { Glyph } from "./Glyph";

export function HeaderSearch() {
  const inputId = useId();
  const rootRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<SearchResult | null>(null);
  const [searching, setSearching] = useState(false);
  const [error, setError] = useState<string | null>(null);

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

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault();
    const q = query.trim();
    if (!q || searching) return;
    setSearching(true);
    setError(null);
    try {
      const res = await api.search(q);
      setResults(res);
    } catch (err) {
      setResults(null);
      setError(err instanceof Error ? err.message : "Search failed.");
    } finally {
      setSearching(false);
    }
  };

  const close = () => setOpen(false);

  const hasResults =
    results &&
    (results.repositories.length > 0 ||
      results.pull_requests.length > 0 ||
      results.workflow_runs.length > 0);
  const showPanel = results !== null || error !== null || searching;

  return (
    <div className={`header-search${open ? " is-open" : ""}`} ref={rootRef}>
      {!open ? (
        <button
          className="header-search__trigger"
          type="button"
          aria-label="Search"
          title="Search"
          aria-expanded={false}
          onClick={() => setOpen(true)}
        >
          <Glyph name="search" />
        </button>
      ) : (
        <div className="header-search__panel">
          <form className="header-search__flyout" role="search" onSubmit={(e) => void onSubmit(e)}>
            <span className="header-search__glyph" aria-hidden="true">
              <Glyph name="search" />
            </span>
            <label className="visually-hidden" htmlFor={inputId}>
              Search repositories and pull requests
            </label>
            <input
              ref={inputRef}
              id={inputId}
              className="header-search__input"
              type="search"
              value={query}
              placeholder="Search repositories, pull requests…"
              aria-label="Search repositories and pull requests"
              aria-controls={showPanel ? `${inputId}-results` : undefined}
              aria-expanded={showPanel}
              onChange={(event) => {
                setQuery(event.target.value);
                setResults(null);
                setError(null);
              }}
            />
            <button
              className="header-search__submit"
              type="submit"
              aria-label="Submit Search"
              title="Submit Search"
              disabled={searching || !query.trim()}
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
              {!searching && !error && results && !hasResults && (
                <p className="header-search__empty muted">No matches.</p>
              )}
              {!searching && !error && hasResults && results && (
                <>
                  {results.repositories.length > 0 && (
                    <div className="header-search__group">
                      <div className="header-search__group-label">Repositories</div>
                      <ul className="header-search__list">
                        {results.repositories.map((repo) => {
                          const external = safeExternalHref(repo.html_url);
                          const label = repo.full_name || `${repo.owner}/${repo.name}`;
                          return (
                            <li key={`repo-${repo.id}`}>
                              {external ? (
                                <a
                                  className="header-search__item"
                                  href={external}
                                  target="_blank"
                                  rel="noreferrer"
                                  onClick={close}
                                >
                                  {label}
                                </a>
                              ) : (
                                <Link className="header-search__item" to="/repositories" onClick={close}>
                                  {label}
                                </Link>
                              )}
                            </li>
                          );
                        })}
                      </ul>
                    </div>
                  )}
                  {results.pull_requests.length > 0 && (
                    <div className="header-search__group">
                      <div className="header-search__group-label">Pull Requests</div>
                      <ul className="header-search__list">
                        {results.pull_requests.map((pr) => (
                          <li key={`pr-${pr.id}`}>
                            <Link className="header-search__item" to="/pull-requests" onClick={close}>
                              {pr.repo_full ? `${pr.repo_full}#${pr.number}` : `#${pr.number}`}{" "}
                              {pr.title}
                            </Link>
                          </li>
                        ))}
                      </ul>
                    </div>
                  )}
                  {results.workflow_runs.length > 0 && (
                    <div className="header-search__group">
                      <div className="header-search__group-label">Pipelines</div>
                      <ul className="header-search__list">
                        {results.workflow_runs.map((run) => (
                          <li key={`run-${run.id}`}>
                            <Link
                              className="header-search__item"
                              to={`/pipelines/${run.id}`}
                              onClick={close}
                            >
                              {run.repo_full ? `${run.repo_full} · ` : ""}
                              {run.name || `Run ${run.id}`}
                            </Link>
                          </li>
                        ))}
                      </ul>
                    </div>
                  )}
                </>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
