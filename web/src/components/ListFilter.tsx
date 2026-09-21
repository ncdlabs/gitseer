import { useEffect, useId, useRef, useState, type FormEvent } from "react";
import { Glyph } from "./Glyph";

type Props = {
  value: string;
  onApply: (query: string) => void;
  placeholder?: string;
  label?: string;
};

export function ListFilter({
  value,
  onApply,
  placeholder = "Filter…",
  label = "Filter list",
}: Props) {
  const inputId = useId();
  const inputRef = useRef<HTMLInputElement>(null);
  const [open, setOpen] = useState(value.trim() !== "");
  const [draft, setDraft] = useState(value);

  useEffect(() => {
    setDraft(value);
    if (value.trim() !== "") setOpen(true);
  }, [value]);

  useEffect(() => {
    if (!open) return;
    const id = window.requestAnimationFrame(() => inputRef.current?.focus());
    return () => window.cancelAnimationFrame(id);
  }, [open]);

  const apply = (raw: string) => {
    const next = raw.trim();
    onApply(next);
    setDraft(next);
    if (next === "") setOpen(false);
  };

  const onButtonClick = () => {
    if (!open) {
      setOpen(true);
      return;
    }
    apply(draft);
  };

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    apply(draft);
  };

  const active = value.trim() !== "";

  return (
    <form
      className={`list-filter${open ? " is-open" : ""}${active ? " is-active" : ""}`}
      onSubmit={onSubmit}
      role="search"
    >
      <label className="visually-hidden" htmlFor={inputId}>
        {label}
      </label>
      <input
        ref={inputRef}
        id={inputId}
        className="list-filter__input"
        type="search"
        value={draft}
        placeholder={placeholder}
        aria-label={label}
        tabIndex={open ? 0 : -1}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            e.preventDefault();
            setDraft(value);
            if (value.trim() === "") setOpen(false);
            else inputRef.current?.blur();
          }
        }}
      />
      <button
        type="button"
        className="list-filter__btn"
        aria-label={open ? "Apply Filter" : "Open Filter"}
        title={open ? "Apply Filter" : "Filter"}
        aria-expanded={open}
        onClick={onButtonClick}
      >
        <Glyph name={open ? "search" : "filter"} />
      </button>
    </form>
  );
}
