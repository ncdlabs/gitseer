import { useEffect, useId, useRef, useState } from "react";
import { giteaPATSettingsURL } from "../lib/gitea";
import { Glyph } from "./Glyph";

type Props = {
  /** Forge base URL (https://git.example.com). */
  baseURL: string;
  /** Raise backdrop above an already-open settings dialog. */
  nested?: boolean;
};

const GITEA_PERMS = [
  {
    code: "write:admin",
    detail: "manage system webhooks (token user must be a site admin)",
  },
  {
    code: "read:repository",
    detail: "sync repositories, PRs, statuses, and Actions",
  },
  {
    code: "read:organization",
    detail: "list organizations",
  },
  {
    code: "read:user",
    detail: "authenticate and identify the token user",
  },
] as const;

export function GiteaPATHelp({ baseURL, nested = false }: Props) {
  const [open, setOpen] = useState(false);
  const titleId = useId();
  const ctaRef = useRef<HTMLAnchorElement>(null);
  const settingsURL = giteaPATSettingsURL(baseURL);

  useEffect(() => {
    if (!open) return;
    ctaRef.current?.focus();
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") setOpen(false);
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open]);

  return (
    <>
      <div className="field-help">
        <div className="field-help-block">
          <div className="field-help-row">
            <a
              href={settingsURL}
              target="_blank"
              rel="noreferrer"
              className="field-help-row__link"
            >
              Open Gitea PAT Settings
            </a>
            <button
              type="button"
              className="info-tip"
              aria-label="How to create a Gitea PAT"
              aria-haspopup="dialog"
              aria-expanded={open}
              onClick={() => setOpen(true)}
            >
              <Glyph name="info" className="info-tip__icon" />
            </button>
          </div>
          <ul className="field-help__perms settings-form__hint">
            {GITEA_PERMS.map((p) => (
              <li key={p.code}>
                <code>{p.code}</code> — {p.detail}
              </li>
            ))}
          </ul>
        </div>
      </div>

      {open && (
        <div
          className={`modal-backdrop${nested ? " modal-backdrop--stack" : ""}`}
          role="presentation"
          onClick={(e) => {
            if (e.target === e.currentTarget) setOpen(false);
          }}
        >
          <div className="modal modal--wide" role="dialog" aria-modal="true" aria-labelledby={titleId}>
            <header className="modal__header">
              <h2 id={titleId}>Create a Gitea PAT</h2>
              <p className="muted">
                GitSeer uses a personal access token to sync data and manage system webhook delivery.
                Paste the token into the field after you create it.
              </p>
            </header>

            <div className="pat-help">
              <ol className="pat-help__steps">
                <li>
                  Open{" "}
                  <a href={settingsURL} target="_blank" rel="noreferrer">
                    Gitea PAT settings
                  </a>
                  .
                </li>
                <li>
                  Create a token named something like GitSeer and enable the permissions listed under
                  the settings link above. The token user must be a site admin so{" "}
                  <code>write:admin</code> can manage system webhooks.
                </li>
                <li>Generate the token, copy it once, and paste it into GitSeer.</li>
              </ol>
            </div>

            <div className="modal__actions">
              <button
                className="btn modal__actions-back"
                type="button"
                onClick={() => setOpen(false)}
              >
                Close
              </button>
              <a
                className="btn primary"
                href={settingsURL}
                target="_blank"
                rel="noreferrer"
                ref={ctaRef}
              >
                Open Gitea PAT Settings
              </a>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
