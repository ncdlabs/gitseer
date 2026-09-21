import { useEffect, useId, useState } from "react";
import { ActiveActionsPanel, openActionsPopout, useActiveActionsCount } from "./ActiveActionsPanel";
import { Glyph } from "./Glyph";

const OPEN_KEY = "lens-actions-flyout-open";

function readOpen(): boolean {
  try {
    return sessionStorage.getItem(OPEN_KEY) === "1";
  } catch {
    return false;
  }
}

function writeOpen(open: boolean) {
  try {
    if (open) sessionStorage.setItem(OPEN_KEY, "1");
    else sessionStorage.removeItem(OPEN_KEY);
  } catch {
    /* private mode / quota — ignore */
  }
}

export function ActionsStatusFlyout() {
  const [open, setOpen] = useState(readOpen);
  const panelId = useId();
  const total = useActiveActionsCount();

  useEffect(() => {
    writeOpen(open);
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [open]);

  function popOut() {
    openActionsPopout();
    setOpen(false);
  }

  return (
    <div className={`actions-flyout${open ? " is-open" : ""}`}>
      {open && (
        <div className="actions-flyout__panel" id={panelId} role="dialog" aria-label="Active Actions">
          <ActiveActionsPanel
            headerActions={
              <button
                type="button"
                className="actions-flyout__popout"
                title="Pop Out"
                aria-label="Pop Out"
                onClick={popOut}
              >
                <Glyph name="popOut" />
              </button>
            }
          />
        </div>
      )}
      <button
        className="sidebar-action actions-flyout__trigger"
        type="button"
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-controls={open ? panelId : undefined}
        title="Active Actions"
        onClick={() => setOpen((v) => !v)}
      >
        <span className="actions-flyout__icon-wrap">
          <Glyph name="pipelines" />
          {total > 0 && <span className="actions-flyout__badge" aria-hidden="true">{total > 99 ? "99+" : total}</span>}
        </span>
        <span>Active Actions</span>
      </button>
    </div>
  );
}
