import { useEffect, useId, useRef, useState, type FormEvent } from "react";
import { Glyph } from "./Glyph";

export function HeaderSearch() {
  const inputId = useId();
  const rootRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");

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

  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    // Search routing is not wired yet; keep the flyout open for continued editing.
  };

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
        <form className="header-search__flyout" role="search" onSubmit={onSubmit}>
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
            onChange={(event) => setQuery(event.target.value)}
          />
          <button className="header-search__submit" type="submit" aria-label="Submit Search" title="Submit Search">
            <Glyph name="submit" />
          </button>
        </form>
      )}
    </div>
  );
}
