import { useEffect, useId, useRef, type ReactNode } from "react";

export type ConfirmDialogProps = {
  open: boolean;
  title: string;
  message: ReactNode;
  /** Confirm / primary action label. Defaults to "Confirm". */
  confirmLabel?: string;
  /**
   * Cancel label. Pass `null` for an alert-style dialog with only the confirm button
   * (defaults to "OK" when cancel is omitted via null).
   */
  cancelLabel?: string | null;
  /** When true, the confirm button uses the danger style. */
  danger?: boolean;
  busy?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
};

export function ConfirmDialog({
  open,
  title,
  message,
  confirmLabel,
  cancelLabel = "Cancel",
  danger = false,
  busy = false,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  const titleId = useId();
  const descId = useId();
  const cancelRef = useRef<HTMLButtonElement>(null);
  const confirmRef = useRef<HTMLButtonElement>(null);
  const isAlert = cancelLabel === null;
  const resolvedConfirm = confirmLabel ?? (isAlert ? "OK" : "Confirm");

  useEffect(() => {
    if (!open) return;
    const focusTarget = isAlert ? confirmRef.current : cancelRef.current;
    focusTarget?.focus();
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape" && !busy) onCancel();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, busy, isAlert, onCancel]);

  if (!open) return null;

  return (
    <div className="modal-backdrop modal-backdrop--dialog" role="presentation">
      <div
        className="modal modal--confirm"
        role="alertdialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={descId}
      >
        <header className="modal__header">
          <h2 id={titleId}>{title}</h2>
          <p id={descId} className="muted">
            {message}
          </p>
        </header>
        <div className="modal__actions">
          {!isAlert && (
            <button
              ref={cancelRef}
              className="btn"
              type="button"
              onClick={onCancel}
              disabled={busy}
            >
              {cancelLabel}
            </button>
          )}
          <button
            ref={confirmRef}
            className={`btn ${danger ? "danger" : "primary"}`}
            type="button"
            onClick={onConfirm}
            disabled={busy}
          >
            {busy ? "Working…" : resolvedConfirm}
          </button>
        </div>
      </div>
    </div>
  );
}
