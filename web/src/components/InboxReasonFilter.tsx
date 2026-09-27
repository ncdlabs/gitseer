import { useEffect, useId, useRef, useState } from "react";
import type { InboxReason } from "../api/client";
import { Glyph } from "./Glyph";

export const INBOX_REASON_OPTIONS: Array<{ id: "" | InboxReason; label: string }> = [
  { id: "", label: "All" },
  { id: "author", label: "My PRs" },
  { id: "requested_reviewer", label: "Review Requests" },
  { id: "failing_ci", label: "Failing CI" },
  { id: "blocked_on_me", label: "Blocked On Me" },
];

/** User-facing label for an inbox reason tag. */
export function inboxReasonLabel(reason: string): string {
  const hit = INBOX_REASON_OPTIONS.find((o) => o.id && o.id === reason);
  if (hit) return hit.label;
  return reason.replaceAll("_", " ");
}

type Props = {
  value: "" | InboxReason;
  onChange: (next: "" | InboxReason) => void;
};

/** Filter-icon control that opens inbox reason presets (All / My PRs / …). */
export function InboxReasonFilter({ value, onChange }: Props) {
  const menuId = useId();
  const rootRef = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const active = value !== "";

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

  return (
    <div
      className={`inbox-reason-filter${open ? " is-open" : ""}${active ? " is-active" : ""}`}
      ref={rootRef}
    >
      <button
        type="button"
        className="inbox-reason-filter__trigger"
        aria-label="Filter Inbox"
        title="Filter Inbox"
        aria-expanded={open}
        aria-haspopup="listbox"
        aria-controls={open ? menuId : undefined}
        onClick={() => setOpen((v) => !v)}
      >
        <Glyph name="filter" />
      </button>
      {open && (
        <div
          id={menuId}
          className="inbox-reason-filter__menu"
          role="listbox"
          aria-label="Filter by reason"
        >
          {INBOX_REASON_OPTIONS.map((opt) => {
            const selected = value === opt.id;
            return (
              <button
                key={opt.id || "all"}
                type="button"
                role="option"
                className={`inbox-reason-filter__option${selected ? " is-active" : ""}`}
                aria-selected={selected}
                onClick={() => {
                  onChange(opt.id);
                  setOpen(false);
                }}
              >
                {opt.label}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
